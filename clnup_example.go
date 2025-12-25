
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// --- Rule definitions ---

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
	rel = strings.ReplaceAll(rel, "\\", "/")
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

// --- Generalized handler ---

type HandlerFunc func(path string, isDir bool) error

func walk(dir string, inherited []Rule, handler HandlerFunc) error {
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
		isDir := e.IsDir()

		if Evaluate(rel, isDir, rules) == Delete {
			if err := handler(full, isDir); err != nil {
				return err
			}
			continue
		}

		if isDir {
			if err := walk(full, rules, handler); err != nil {
				return err
			}
		}
	}
	return nil
}

// --- Example handlers ---

func printHandler(path string, isDir bool) error {
	fmt.Println(path)
	return nil
}

func deleteHandler(path string, isDir bool) error {
	fmt.Println("[delete]", path)
	return os.RemoveAll(path)
}

// --- main ---

func main() {
	root := "."
	action := "print" // default

	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	if len(os.Args) > 2 {
		action = os.Args[2]
	}

	var handler HandlerFunc
	switch action {
	case "print":
		handler = printHandler
	case "delete":
		handler = deleteHandler
	default:
		fmt.Println("Unknown action:", action)
		os.Exit(1)
	}

	if err := walk(root, nil, handler); err != nil {
		fmt.Fprintln(os.Stderr, "qcmd_example:", err)
		os.Exit(1)
	}
}
