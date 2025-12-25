package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	err := walk(root, []Rule{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "clnup:", err)
		os.Exit(1)
	}
}

func walk(dir string, inherited []Rule) error {
	rules := append([]Rule{}, inherited...)

	clnupPath := filepath.Join(dir, ".clnup")
	if data, err := os.ReadFile(clnupPath); err == nil {
		parsed, err := ParseRules(string(data))
		if err != nil {
			return err
		}
		rules = append(rules, parsed...)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, e := range entries {
		name := e.Name()
		if name == ".clnup" {
			continue
		}

		full := filepath.Join(dir, name)
		rel := name

		decision := Evaluate(rel, e.IsDir(), rules)

		if decision == Delete {
			if err := os.RemoveAll(full); err != nil {
				return err
			}
			continue
		}

		if e.IsDir() {
			if err := walk(full, rules); err != nil {
				return err
			}
		}
	}

	return nil
}

