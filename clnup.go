package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ------------------------------------------------------------
// Types
// ------------------------------------------------------------

type actionResult int

const (
	keep actionResult = iota
	delete_
)

type statField int

const (
	fieldSize statField = iota
	fieldMtime
	fieldAtime
	fieldCtime
	fieldUID
	fieldGID
	fieldMode
)

type op int

const (
	opLt op = iota
	opLe
	opGt
	opGe
	opEq
	opNe
)

// predicate is a node in the expression AST.
type predicate interface {
	eval(s statInfo) bool
}

type cmpNode struct {
	field statField
	op    op
	value int64
}

type andNode struct{ lhs, rhs predicate }
type orNode struct{ lhs, rhs predicate }

func (c *cmpNode) eval(s statInfo) bool {
	var lhs int64
	switch c.field {
	case fieldSize:
		lhs = s.size
	case fieldMtime:
		lhs = s.mtime
	case fieldAtime:
		lhs = s.atime
	case fieldCtime:
		lhs = s.ctime
	case fieldUID:
		lhs = s.uid
	case fieldGID:
		lhs = s.gid
	case fieldMode:
		lhs = s.mode
	}
	switch c.op {
	case opLt:
		return lhs < c.value
	case opLe:
		return lhs <= c.value
	case opGt:
		return lhs > c.value
	case opGe:
		return lhs >= c.value
	case opEq:
		return lhs == c.value
	case opNe:
		return lhs != c.value
	}
	return false
}

func (n *andNode) eval(s statInfo) bool { return n.lhs.eval(s) && n.rhs.eval(s) }
func (n *orNode) eval(s statInfo) bool  { return n.lhs.eval(s) || n.rhs.eval(s) }

// statInfo is a platform-neutral wrapper around os.FileInfo stat values.
// All times are in nanoseconds since the Unix epoch.
type statInfo struct {
	size  int64
	mtime int64
	atime int64 // not available on all platforms via os.FileInfo; set to mtime
	ctime int64 // not available on all platforms via os.FileInfo; set to mtime
	uid   int64 // Unix only; 0 on Windows
	gid   int64 // Unix only; 0 on Windows
	mode  int64
}

type rule struct {
	pattern   string
	negated   bool
	dirOnly   bool
	anchored  bool
	predicate predicate // nil means glob-only rule
}

type handlerFunc func(path string, isDir bool, quiet, verbose bool) error

// ------------------------------------------------------------
// Predicate parser
//
// Grammar (|| binds looser than &&):
//   expr     := and_expr ( '||' and_expr )*
//   and_expr := cmp ( '&&' cmp )*
//   cmp      := 'stat.' FIELD OP VALUE
//   OP       := '<' | '<=' | '>' | '>=' | '==' | '!='
//   VALUE    := integer with optional kb/mb/gb suffix
//              | 'now-' N ('d' | 'h' | 'm')
//              | plain integer (epoch ns or raw number)
// ------------------------------------------------------------

type parser struct {
	src string
	pos int
}

func (p *parser) skipWs() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t') {
		p.pos++
	}
}

func (p *parser) peek(s string) bool {
	return strings.HasPrefix(p.src[p.pos:], s)
}

func (p *parser) consume(s string) bool {
	if p.peek(s) {
		p.pos += len(s)
		return true
	}
	return false
}

func (p *parser) parseExpr() (predicate, error) {
	lhs, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	p.skipWs()
	for p.consume("||") {
		p.skipWs()
		rhs, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		lhs = &orNode{lhs, rhs}
		p.skipWs()
	}
	return lhs, nil
}

func (p *parser) parseAnd() (predicate, error) {
	lhs, err := p.parseCmp()
	if err != nil {
		return nil, err
	}
	p.skipWs()
	for p.consume("&&") {
		p.skipWs()
		rhs, err := p.parseCmp()
		if err != nil {
			return nil, err
		}
		lhs = &andNode{lhs, rhs}
		p.skipWs()
	}
	return lhs, nil
}

func (p *parser) parseCmp() (predicate, error) {
	p.skipWs()
	if !p.consume("stat.") {
		return nil, fmt.Errorf("expected 'stat.' at position %d", p.pos)
	}
	f, err := p.parseField()
	if err != nil {
		return nil, err
	}
	p.skipWs()
	o, err := p.parseOp()
	if err != nil {
		return nil, err
	}
	p.skipWs()
	v, err := p.parseValue(f)
	if err != nil {
		return nil, err
	}
	return &cmpNode{field: f, op: o, value: v}, nil
}

func (p *parser) parseField() (statField, error) {
	fields := []struct {
		name string
		f    statField
	}{
		{"size", fieldSize},
		{"mtime", fieldMtime},
		{"atime", fieldAtime},
		{"ctime", fieldCtime},
		{"uid", fieldUID},
		{"gid", fieldGID},
		{"mode", fieldMode},
	}
	for _, entry := range fields {
		if p.consume(entry.name) {
			return entry.f, nil
		}
	}
	return 0, fmt.Errorf("unknown stat field at position %d", p.pos)
}

func (p *parser) parseOp() (op, error) {
	switch {
	case p.consume("<="):
		return opLe, nil
	case p.consume(">="):
		return opGe, nil
	case p.consume("!="):
		return opNe, nil
	case p.consume("=="):
		return opEq, nil
	case p.consume("<"):
		return opLt, nil
	case p.consume(">"):
		return opGt, nil
	}
	return 0, fmt.Errorf("expected operator at position %d", p.pos)
}

func (p *parser) parseValue(f statField) (int64, error) {
	switch f {
	case fieldSize:
		return p.parseSizeValue()
	case fieldMtime, fieldAtime, fieldCtime:
		return p.parseTimeValue()
	default:
		return p.parseIntValue()
	}
}

func (p *parser) parseSizeValue() (int64, error) {
	n, err := p.parseIntValue()
	if err != nil {
		return 0, err
	}
	switch {
	case p.consume("gb") || p.consume("GB"):
		return n * 1024 * 1024 * 1024, nil
	case p.consume("mb") || p.consume("MB"):
		return n * 1024 * 1024, nil
	case p.consume("kb") || p.consume("KB"):
		return n * 1024, nil
	}
	return n, nil
}

func (p *parser) parseTimeValue() (int64, error) {
	if p.consume("now-") {
		n, err := p.parseIntValue()
		if err != nil {
			return 0, err
		}
		now := time.Now().UnixNano()
		const nsPerSec = int64(time.Second)
		switch {
		case p.consume("d"):
			return now - n*86400*nsPerSec, nil
		case p.consume("h"):
			return now - n*3600*nsPerSec, nil
		case p.consume("m"):
			return now - n*60*nsPerSec, nil
		}
		return 0, fmt.Errorf("expected 'd', 'h', or 'm' after now-N at position %d", p.pos)
	}
	return p.parseIntValue()
}

func (p *parser) parseIntValue() (int64, error) {
	start := p.pos
	for p.pos < len(p.src) && p.src[p.pos] >= '0' && p.src[p.pos] <= '9' {
		p.pos++
	}
	if p.pos == start {
		return 0, fmt.Errorf("expected integer at position %d", p.pos)
	}
	return strconv.ParseInt(p.src[start:p.pos], 10, 64)
}

func parsePredicate(src string) (predicate, error) {
	p := &parser{src: src}
	pred, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	p.skipWs()
	if p.pos != len(p.src) {
		return nil, fmt.Errorf("unexpected input at position %d: %q", p.pos, p.src[p.pos:])
	}
	return pred, nil
}

// ------------------------------------------------------------
// Rules parsing
// ------------------------------------------------------------

func parseRules(input string) ([]rule, error) {
	var rules []rule
	for _, lineRaw := range strings.Split(input, "\n") {
		line := strings.Trim(lineRaw, " \t\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Split on the first '|' — everything before is the glob, after is the predicate.
		var globPart, predPart string
		if idx := strings.IndexByte(line, '|'); idx >= 0 {
			globPart = strings.TrimRight(line[:idx], " \t")
			predPart = strings.TrimSpace(line[idx+1:])
		} else {
			globPart = line
		}

		r := rule{}
		p := globPart

		if strings.HasPrefix(p, "!") {
			r.negated = true
			p = p[1:]
		}
		if strings.HasPrefix(p, "/") {
			r.anchored = true
			p = p[1:]
		}
		if strings.HasSuffix(p, "/") {
			r.dirOnly = true
			p = strings.TrimSuffix(p, "/")
		}
		r.pattern = p

		if predPart != "" {
			pred, err := parsePredicate(predPart)
			if err != nil {
				return nil, fmt.Errorf("rule %q: %w", line, err)
			}
			r.predicate = pred
		}

		rules = append(rules, r)
	}
	return rules, nil
}

// ------------------------------------------------------------
// Stat
// ------------------------------------------------------------

// statPath returns a statInfo for the given path, or nil on error.
// Platform-specific fields (uid, gid, atime, ctime) are filled in
// by the platform-specific statExtra function defined in stat_unix.go
// / stat_windows.go. For portability, atime and ctime fall back to
// mtime when not available.
func statPath(path string) *statInfo {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil
	}
	s := &statInfo{
		size:  fi.Size(),
		mtime: fi.ModTime().UnixNano(),
		atime: fi.ModTime().UnixNano(), // fallback; overridden by statExtra
		ctime: fi.ModTime().UnixNano(), // fallback; overridden by statExtra
		mode:  int64(fi.Mode()),
	}
	statExtra(s, fi) // platform-specific: fills uid, gid, atime, ctime
	return s
}

// ------------------------------------------------------------
// Rule evaluation and glob matching
// ------------------------------------------------------------

func evaluate(rel string, isDir bool, rules []rule, s *statInfo) actionResult {
	result := keep
	for _, r := range rules {
		if r.dirOnly && !isDir {
			continue
		}
		if !matchRule(r, rel) {
			continue
		}
		if r.predicate != nil {
			if s == nil {
				continue // can't stat → skip predicate rule
			}
			if !r.predicate.eval(*s) {
				continue
			}
		}
		if r.negated {
			result = keep
		} else {
			result = delete_
		}
	}
	return result
}

func matchRule(r rule, rel string) bool {
	rel = strings.ReplaceAll(rel, "\\", "/")
	if r.anchored {
		return globMatch(r.pattern, rel)
	}
	// Try every suffix starting at a component boundary.
	offset := 0
	for offset <= len(rel) {
		if globMatch(r.pattern, rel[offset:]) {
			return true
		}
		idx := strings.IndexByte(rel[offset:], '/')
		if idx < 0 {
			break
		}
		offset += idx + 1
	}
	return false
}

func globMatch(pattern, name string) bool {
	// filepath.Match handles *, ?, and character classes.
	// We only use * and ? but filepath.Match is correct for those.
	ok, _ := filepath.Match(pattern, name)
	return ok
}

// ------------------------------------------------------------
// Directory traversal
// ------------------------------------------------------------

func processDir(root string, rules []rule, handler handlerFunc, quiet, verbose bool) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		full := filepath.Join(root, name)
		isDir := e.IsDir() || e.Type()&os.ModeSymlink != 0
		s := statPath(full)
		if evaluate(name, isDir, rules, s) == delete_ {
			if err := handler(full, isDir, quiet, verbose); err != nil {
				return err
			}
		}
	}
	return nil
}

// walk tracks relPrefix — the path relative to the user-supplied root —
// so that anchored and multi-segment rules evaluate correctly at any depth.
func walk(root, relPrefix string, rules []rule, handler handlerFunc, quiet, verbose bool) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		full := filepath.Join(root, name)

		var rel string
		if relPrefix == "" {
			rel = name
		} else {
			rel = relPrefix + "/" + name
		}

		isDir := e.IsDir() || e.Type()&os.ModeSymlink != 0
		s := statPath(full)

		if evaluate(rel, isDir, rules, s) == delete_ {
			if err := handler(full, isDir, quiet, verbose); err != nil {
				return err
			}
			continue
		}
		if isDir {
			if err := walk(full, rel, rules, handler, quiet, verbose); err != nil {
				return err
			}
		}
	}
	return nil
}

// ------------------------------------------------------------
// Handlers
// ------------------------------------------------------------

func printHandler(path string, _ bool, quiet, verbose bool) error {
	if quiet {
		return nil
	}
	if verbose {
		fmt.Println("[dry-run]", path)
	} else {
		fmt.Println(path)
	}
	return nil
}

func deleteHandler(path string, _ bool, quiet, verbose bool) error {
	if !quiet {
		if verbose {
			fmt.Println("[delete]", path)
		} else {
			fmt.Println(path)
		}
	}
	return os.RemoveAll(path)
}

func touchHandler(path string, isDir bool, quiet, verbose bool) error {
	if isDir {
		return nil
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return err
	}
	return f.Close()
}

// ------------------------------------------------------------
// Entry point
// ------------------------------------------------------------

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: clnup [-r] [-f <file>] [-q] [-v] [-d] [-action print|delete|touch] [path]")
	fmt.Fprintln(os.Stderr, "Options:")
	fmt.Fprintln(os.Stderr, "  -r            Recurse into subdirectories")
	fmt.Fprintln(os.Stderr, "  -f FILE       Specify cleanup rules file (default: .clnup)")
	fmt.Fprintln(os.Stderr, "  -q            Quiet mode (suppress normal output)")
	fmt.Fprintln(os.Stderr, "  -v            Verbose mode (extra logging)")
	fmt.Fprintln(os.Stderr, "  -d            Dry run (equivalent to -action=print)")
	fmt.Fprintln(os.Stderr, "  -action NAME  Handler action: print | delete | touch (default: delete)")
	os.Exit(1)
}

func main() {
	clnupPath := flag.String("f", ".clnup", "Path to .clnup file")
	action    := flag.String("action", "delete", "Handler action: print | delete | touch")
	recursive := flag.Bool("r", false, "Recurse into subdirectories")
	quiet     := flag.Bool("q", false, "Quiet mode")
	verbose   := flag.Bool("v", false, "Verbose mode")
	dryRun    := flag.Bool("d", false, "Dry run (equivalent to -action=print)")
	flag.Usage = usage
	flag.Parse()

	if *dryRun {
		*action = "print"
	}

	root := "."
	if flag.NArg() > 0 {
		root = flag.Arg(0)
	}

	data, err := os.ReadFile(*clnupPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to read .clnup file:", err)
		os.Exit(1)
	}

	rules, err := parseRules(string(data))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to parse .clnup:", err)
		os.Exit(1)
	}

	if *verbose {
		fmt.Fprintf(os.Stderr, "Using rules from: %s\n", *clnupPath)
		fmt.Fprintf(os.Stderr, "Target path:      %s\n", root)
		fmt.Fprintf(os.Stderr, "Action:           %s\n", *action)
		if *recursive {
			fmt.Fprintln(os.Stderr, "Recursion:        enabled")
		}
	}

	var handler handlerFunc
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

	if *recursive {
		err = walk(root, "", rules, handler, *quiet, *verbose)
	} else {
		err = processDir(root, rules, handler, *quiet, *verbose)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
