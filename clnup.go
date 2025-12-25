package main

import (
	"flag"
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

func walk(root string, rules []Rule, handler HandlerFunc) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}

	for _, e := range entries {
		name := e.Name()
		full := filepath.Join(root, name)
		isDir := e.IsDir()

		if Evaluate(name, isDir, rules) == Delete {
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

func touchHandler(path string, isDir bool) error {
	if isDir {
		return nil
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	return f.Close()
}

// --- main ---

func main() {
	clnupPath := flag.String("file", "", "Path to .clnup file (cleanup rules)")
	action := flag.String("action", "print", "Handler action: print | delete | touch")
	flag.Parse()

	if *clnupPath == "" {
		fmt.Println("Usage: clnup --file <.clnup> [--action=print|delete|touch]")
		os.Exit(1)
	}

	data, err := os.ReadFile(*clnupPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to read .clnup file:", err)
		os.Exit(1)
	}

	rules, err := ParseRules(string(data))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to parse .clnup:", err)
		os.Exit(1)
	}

	var handler HandlerFunc
	switch *action {
	case "print":
		handler = printHandler
	case "delete":
		handler = deleteHandler
	case "touch":
		handler = touchHandler
	default:
		fmt.Fprintln(os.Stderr, "Unknown action:", *action)
		os.Exit(1)
	}

	root := filepath.Dir(*clnupPath)
	if err := walk(root, rules, handler); err != nil {
		fmt.Fprintln(os.Stderr, "Error walking directory:", err)
		os.Exit(1)
	}
}
