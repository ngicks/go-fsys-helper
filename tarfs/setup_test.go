package tarfs

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

var (
	testTars = sync.OnceValues(func() ([]string, error) {
		testdataDir, err := stdTarTestDir()
		if err != nil {
			return nil, err
		}
		dirents, err := os.ReadDir(testdataDir)
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(dirents))
		for _, dirent := range dirents {
			if !dirent.Type().IsRegular() || !strings.HasSuffix(dirent.Name(), ".tar") {
				continue
			}
			names = append(names, filepath.Join(testdataDir, dirent.Name()))
		}
		return names, nil
	})
)

func stdTarTestDir() (string, error) {
	// Use go env GOROOT to get the Go installation path
	ctx := context.Background()
	gorootBytes, err := exec.CommandContext(ctx, "go", "env", "GOROOT").Output()
	if err != nil {
		return "", err
	}
	goroot := strings.TrimSpace(string(gorootBytes))
	return filepath.Join(goroot, "src", "archive", "tar", "testdata"), nil
}

// Verify that testdata directory exists at init time
func init() {
	testdataDir, err := stdTarTestDir()
	if err != nil {
		panic(err)
	}
	_, err = os.Stat(testdataDir)
	if err != nil {
		panic(err)
	}
}
