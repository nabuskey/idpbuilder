package util

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/cnoe-io/idpbuilder/api/v1alpha1"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
)

func CopyDirectory(scrDir, dest string) error {
	entries, err := os.ReadDir(scrDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		sourcePath := filepath.Join(scrDir, entry.Name())
		destPath := filepath.Join(dest, entry.Name())

		fileInfo, err := os.Stat(sourcePath)
		if err != nil {
			return err
		}

		switch fileInfo.Mode() & os.ModeType {
		case os.ModeDir:
			if err := CreateIfNotExists(destPath, 0755); err != nil {
				return err
			}
			if err := CopyDirectory(sourcePath, destPath); err != nil {
				return err
			}
		case os.ModeSymlink:
			continue
		default:
			if err := Copy(sourcePath, destPath); err != nil {
				return err
			}
		}

		fInfo, err := entry.Info()
		if err != nil {
			return err
		}
		if err := os.Chmod(destPath, fInfo.Mode()); err != nil {
			return err
		}
	}
	return nil
}

func Copy(srcFile, dstFile string) error {
	out, err := os.Create(dstFile)
	if err != nil {
		return err
	}

	defer out.Close()

	in, err := os.Open(srcFile)
	if err != nil {
		return err
	}

	defer in.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}

	return nil
}

func Exists(filePath string) bool {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false
	}

	return true
}

func CreateIfNotExists(dir string, perm os.FileMode) error {
	if Exists(dir) {
		return nil
	}

	if err := os.MkdirAll(dir, perm); err != nil {
		return fmt.Errorf("failed to create directory: '%s', error: '%s'", dir, err.Error())
	}

	return nil
}

func ApplyTemplate(in []byte, templateData any) ([]byte, error) {
	t, err := template.New("template").Parse(string(in))
	if err != nil {
		return nil, err
	}

	// Execute the template with the file content and write the output to the destination file
	ret := bytes.Buffer{}
	err = t.Execute(&ret, templateData)
	if err != nil {
		return nil, err
	}

	return ret.Bytes(), nil
}

// returns all files with yaml or yml suffix from a worktree
func GetWorktreeYamlFiles(parent string, wt billy.Filesystem, recurse bool) ([]string, error) {
	if strings.HasSuffix(parent, "/") {
		parent = strings.TrimSuffix(parent, "/")
	}
	paths := make([]string, 0, 10)
	ents, err := wt.ReadDir(parent)
	if err != nil {
		return nil, err
	}
	for i := range ents {
		ent := ents[i]
		if ent.IsDir() && recurse {
			dir := fmt.Sprintf("%s/%s", parent, ent.Name())
			rPaths, dErr := GetWorktreeYamlFiles(dir, wt, recurse)
			if dErr != nil {
				return nil, fmt.Errorf("reading %s : %w", ent.Name(), dErr)
			}
			paths = append(paths, rPaths...)
		}
		if ent.Mode().IsRegular() && (strings.HasSuffix(ent.Name(), "yaml") || strings.HasSuffix(ent.Name(), "yml")) {
			paths = append(paths, fmt.Sprintf("%s/%s", parent, ent.Name()))
		}
	}
	return paths, nil
}

func ReadWorktreeFile(wt billy.Filesystem, path string) ([]byte, error) {
	f, fErr := wt.Open(path)
	if fErr != nil {
		return nil, fmt.Errorf("opening %s", path)
	}
	defer f.Close()

	b := new(bytes.Buffer)
	_, fErr = b.ReadFrom(f)
	if fErr != nil {
		return nil, fmt.Errorf("reading %s", path)
	}

	return b.Bytes(), nil
}

func CloneRemoteRepoToMemory(ctx context.Context, remote v1alpha1.RemoteRepositorySpec, depth int) (billy.Filesystem, *git.Repository, error) {
	cloneOptions := &git.CloneOptions{
		URL:               remote.Url,
		Depth:             depth,
		ShallowSubmodules: true,
		ReferenceName:     plumbing.ReferenceName(remote.Ref),
		SingleBranch:      true,
		Tags:              git.NoTags,
		InsecureSkipTLS:   true,
	}
	if remote.CloneSubmodules {
		cloneOptions.RecurseSubmodules = git.DefaultSubmoduleRecursionDepth
	}

	wt := memfs.New()
	cloned, err := git.CloneContext(ctx, memory.NewStorage(), wt, cloneOptions)
	if err != nil {
		return nil, nil, fmt.Errorf("cloning repo, %s: %w", remote.Url, err)
	}

	return wt, cloned, nil
}

func CopyTreeToTree(srcWT, dstWT billy.Filesystem, srcPath, dstPath string) error {
	files, err := srcWT.ReadDir(srcPath)
	if err != nil {
		return err
	}

	for i := range files {
		srcFile := files[i]
		fullSrcPath := filepath.Join(srcPath, srcFile.Name())
		fullDstPath := filepath.Join(dstPath, srcFile.Name())
		if srcFile.Mode().IsRegular() {
			cErr := CopyWTFile(srcWT, dstWT, fullSrcPath, fullDstPath)
			if cErr != nil {
				return cErr
			}
			continue
		}

		if srcFile.IsDir() {
			dErr := CopyTreeToTree(srcWT, dstWT, fullSrcPath, fullDstPath)
			if dErr != nil {
				return dErr
			}
		}
	}
	return nil
}

func CopyWTFile(srcWT, dstWT billy.Filesystem, srcFile, dstFile string) error {
	newFile, err := dstWT.Create(dstFile)
	if err != nil {
		return fmt.Errorf("creating file %s: %w", dstFile, err)
	}
	defer newFile.Close()

	srcF, err := srcWT.Open(srcFile)
	if err != nil {
		return fmt.Errorf("reading file %s: %w", srcFile, err)
	}
	defer srcF.Close()

	_, err = io.Copy(newFile, srcF)
	if err != nil {
		return fmt.Errorf("copying file %s: %w", srcFile, err)
	}
	return nil
}
