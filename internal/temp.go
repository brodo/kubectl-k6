package internal

import (
	cp "github.com/otiai10/copy"
	"os"
	"path/filepath"
)

func CreateTempFolder(srcPath string) (error, string) {
	tmpDir, err := os.MkdirTemp(os.TempDir(), "kubectl-k6-")
	if err != nil {
		return err, ""
	}
	dest := filepath.Join(tmpDir, "test")
	err = cp.Copy(srcPath, dest)
	return err, tmpDir
}
