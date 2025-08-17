package util

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func FileExists(filePath string) (bool, error) {
	_, err := os.Stat(filePath)
	if err == nil {
		return true, nil // File exists
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil // File does not exist
	}
	// Other error occurred (e.g., permissions issue)
	return false, err
}

func IsDirectory(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return fileInfo.IsDir(), err
}

func RemoveExt(path string) string {
	return strings.TrimSuffix(path, filepath.Ext(path))
}
