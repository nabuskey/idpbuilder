package util

import (
	"context"
	"io"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/cnoe-io/idpbuilder/api/v1alpha1"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/stretchr/testify/assert"
)

var specialCharMap = make(map[string]struct{})

func TestGeneratePassword(t *testing.T) {
	for i := range specialChars {
		specialCharMap[string(specialChars[i])] = struct{}{}
	}

	for i := 0; i < 1000; i++ {
		p, err := GeneratePassword()
		if err != nil {
			t.Fatalf("error generating password: %v", err)
		}
		counts := make([]int, 3)
		for j := range p {
			counts[0] += 1
			c := string(p[j])
			_, ok := specialCharMap[c]
			if ok {
				counts[1] += 1
				continue
			}
			_, err := strconv.Atoi(c)
			if err == nil {
				counts[2] += 1
			}
		}
		if counts[0] != passwordLength {
			t.Fatalf("password legnth incorrect")
		}
		if counts[1] < numSpecialChars {
			t.Fatalf("min number of special chars not generated")
		}
		if counts[2] < numDigits {
			t.Fatalf("min number of digits not generated")
		}
	}
}

func TestCopyTreeToTree(t *testing.T) {
	spec := v1alpha1.RemoteRepositorySpec{
		CloneSubmodules: false,
		Path:            "examples/basic",
		Url:             "../..",
		Ref:             "",
	}

	dst := memfs.New()
	src, _, err := CloneRemoteRepoToMemory(context.Background(), spec, 1)
	assert.Nil(t, err)

	err = CopyTreeToTree(src, dst, spec.Path, ".")
	assert.Nil(t, err)
	testCopiedFiles(t, src, dst, spec.Path, ".")
}

func testCopiedFiles(t *testing.T, src, dst billy.Filesystem, srcStartPath, dstStartPath string) {
	files, err := src.ReadDir(srcStartPath)
	assert.Nil(t, err)

	for i := range files {
		file := files[i]
		if file.Mode().IsRegular() {
			srcFile, err := src.Open(filepath.Join(srcStartPath, file.Name()))
			assert.Nil(t, err)
			srcB, err := io.ReadAll(srcFile)
			assert.Nil(t, err)

			dstFile, err := dst.Open(filepath.Join(dstStartPath, file.Name()))
			assert.Nil(t, err)
			dstB, err := io.ReadAll(dstFile)
			assert.Nil(t, err)
			assert.Equal(t, srcB, dstB)
		}
		if file.IsDir() {
			testCopiedFiles(t, src, dst, filepath.Join(srcStartPath, file.Name()), filepath.Join(dstStartPath, file.Name()))
		}
	}
}
