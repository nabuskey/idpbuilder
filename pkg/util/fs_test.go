package util

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/cnoe-io/idpbuilder/globals"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func TestWriteFS(t *testing.T) {
	cases := []struct {
		name        string
		srcFS       fs.FS
		expectErr   error
		expectFiles map[string][]byte
	}{{
		name: "single file",
		srcFS: fstest.MapFS{
			"testfile": &fstest.MapFile{
				Data: []byte("testdata"),
				Mode: 0666,
			},
		},
		expectFiles: map[string][]byte{
			"testfile": []byte("testdata"),
		},
	}, {
		name: "file in subdir",
		srcFS: fstest.MapFS{
			"somedir": &fstest.MapFile{
				Mode: fs.ModeDir,
			},
			"somedir/testfile": &fstest.MapFile{
				Data: []byte("testdata"),
				Mode: 0666,
			},
		},
		expectFiles: map[string][]byte{
			"somedir/testfile": []byte("testdata"),
		},
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workDir, err := os.MkdirTemp("", fmt.Sprintf("%s-fs_test.go-%s", globals.ProjectName, tc.name))
			if err != nil {
				t.Fatalf("creating tempdir: %v", err)
			}
			defer os.RemoveAll(workDir)

			err = WriteFS(tc.srcFS, workDir)
			if err != tc.expectErr {
				t.Errorf("unexpected error writing fs: %v", err)
			}

			for expectPath, expectData := range tc.expectFiles {
				fullExpectPath := filepath.Join(workDir, expectPath)
				gotData, err := os.ReadFile(fullExpectPath)
				if err != nil {
					t.Errorf("Opening expected file: %v", err)
				}

				if diff := cmp.Diff(string(expectData), string(gotData)); diff != "" {
					t.Errorf("Expected data in %q mismatch (-want +got):\n%s", expectPath, diff)
				}
			}
		})
	}
}

func TestGetWorktreeYamlFiles(t *testing.T) {
	cloneOptions := &git.CloneOptions{
		URL:               "../..", // avoid remote clone
		Depth:             1,
		ShallowSubmodules: true,
	}

	wt := memfs.New()
	_, err := git.CloneContext(context.Background(), memory.NewStorage(), wt, cloneOptions)
	if err != nil {
		t.Fatalf(err.Error())
	}

	paths, err := GetWorktreeYamlFiles("./pkg", wt, true)

	assert.Equal(t, nil, err)
	assert.NotEqual(t, 0, len(paths))
	for _, s := range paths {
		assert.Equal(t, true, strings.HasSuffix(s, "yaml") || strings.HasSuffix(s, "yml"))
	}

	paths, err = GetWorktreeYamlFiles("./pkg", wt, false)
	assert.Equal(t, nil, err)
	assert.Equal(t, 0, len(paths))
}
