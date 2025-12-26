
package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Helper to create temporary files and directories
func createTestFiles(t *testing.T, base string, paths []string) {
	t.Helper()
	for _, p := range paths {
		full := filepath.Join(base, p)
		dir := filepath.Dir(full)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if p[len(p)-1] != '/' {
			f, err := os.Create(full)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
		}
	}
}

func TestEvaluateRules(t *testing.T) {
	tmp := t.TempDir()

	// Files and dirs
	files := []string{
		"build/output.txt",
		"dist/app.bin",
		"keep.txt",
		"temp/tmpfile.tmp",
		"logs/error.log",
	}

	createTestFiles(t, tmp, files)

	// Example .clnup rules
	clnupData := `
build/
dist/
*.tmp
*.log
!keep.txt
`
	rules, err := ParseRules(clnupData)
	if err != nil {
		t.Fatal(err)
	}

	// Collect deleted files in a slice
	deleted := []string{}
	handler := func(path string, isDir bool) error {
		deleted = append(deleted, path)
		return nil
	}

	if err := walk(tmp, rules, handler); err != nil {
		t.Fatal(err)
	}

	// Check expected deleted files
	expected := []string{
		filepath.Join(tmp, "build", "output.txt"),
		filepath.Join(tmp, "dist", "app.bin"),
		filepath.Join(tmp, "temp", "tmpfile.tmp"),
		filepath.Join(tmp, "logs", "error.log"),
	}

	for _, exp := range expected {
		found := false
		for _, d := range deleted {
			if d == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected to delete %s but did not", exp)
		}
	}
}
