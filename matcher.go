package main

import (
	"path"
	"strings"
)

type Action int

const (
	Keep Action = iota
	Delete
)

type Rule struct {
	Pattern   string
	Negated   bool
	DirOnly   bool
	Anchored  bool
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
	rel = path.Clean(filepathToSlash(rel))

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
	pp := strings.Split(pattern, "/")
	np := strings.Split(name, "/")
	return matchParts(pp, np)
}

func matchParts(pat, name []string) bool {
	if len(pat) == 0 {
		return len(name) == 0
	}

	if pat[0] == "**" {
		for i := 0; i <= len(name); i++ {
			if matchParts(pat[1:], name[i:]) {
				return true
			}
		}
		return false
	}

	if len(name) == 0 {
		return false
	}

	if !segmentMatch(pat[0], name[0]) {
		return false
	}

	return matchParts(pat[1:], name[1:])
}

func segmentMatch(pat, s string) bool {
	ok, _ := path.Match(pat, s)
	return ok
}

func filepathToSlash(p string) string {
	return strings.ReplaceAll(p, "\\", "/")
}


