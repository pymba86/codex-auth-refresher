package storage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func WriteFileAtomic(path string, data []byte, perm fs.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	file, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := file.Name()
	defer os.Remove(tmpPath)

	if _, err := file.Write(data); err != nil {
		file.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := file.Chmod(perm); err != nil {
		file.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	dirHandle, err := os.Open(dir)
	if err == nil {
		_ = dirHandle.Sync()
		_ = dirHandle.Close()
	}
	return nil
}
