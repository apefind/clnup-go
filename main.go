package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Action int

const (
	Keep Action = iota
	Delete
)

type Rule struct {
	Pattern  string
	Negated  bool
	DirOnly  bool
	Anchored bool
}

func ParseRules(input string) ([]Rule, error) {
	var rules []Rule
	for _, line := range strings.Split(input, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		r := Rule{}
		if strings.HasPrefix(line, "!") {
			r.Negated = true
			line = line[1:]
		}
		if strings.HasPrefix(line, "/") {
			r.Anchored = true
			line = line[1:]
		}
		if strings.HasSuffix(line, "/") {
			r.DirOnly = true
			line = strings.TrimSuffix(line, "/")
		}
		r.Pattern = line
		rules = append(rules, r)
	}
	return rules, nil
}

func Evaluate(rel string, isDir bool, rules []Rule) Action {
	result := Keep
	for _, r := range rules {
		if r.DirOnly && !isDir {
			continue
		}
		if match(r, rel) {
			if r.Negated {
				result = Keep
			} else {
				result = Delete
			}
		}
	}
	return result
}

func match(r Rule, rel string) bool {
	rel = strings.ReplaceAll(rel, "\\", "/") // normalize

	if r.Anchored {
		return globMatch(r.Pattern, rel)
	}

	parts := strings.Split(rel, "/")
	for i := 0; i < len(parts); i++ {
		sub := strings.Join(parts[i:], "/")
		if globMatch(r.Pattern, sub) {
			return true
		}
	}
	return false
}

func globMatch(pattern, name string) bool {
	ok, _ := filepath.Match(pattern, name)
	return ok
}

// --- removeAll function ---

func removeAll(path string, dryRun bool) error {
	if dryRun {
		fmt.Println("[dry-run] would remove:", path)
		return nil
	}
	return os.RemoveAll(path)
}

// --- tree walk ---

func walk(dir string, inherited []Rule, dryRun bool) error {
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
			if err := removeAll(full, dryRun); err != nil {
				return err
			}
			continue
		}

		if e.IsDir() {
			if err := walk(full, rules, dryRun); err != nil {
				return err
			}
		}
	}

	return nil
}

// --- main ---

func main() {
	root := "."
	dryRun := true // set false to actually delete

	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	if err := walk(root, nil, dryRun); err != nil {
		fmt.Fprintln(os.Stderr, "clnup:", err)
		os.Exit(1)
	}
}
