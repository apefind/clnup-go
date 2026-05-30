package main

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// ------------------------------------------------------------
// Helpers
// ------------------------------------------------------------

// createTestFiles creates files and directories under base.
// Paths ending in '/' are treated as directories.
func createTestFiles(t *testing.T, base string, paths []string) {
	t.Helper()
	for _, p := range paths {
		full := filepath.Join(base, p)
		if p[len(p)-1] == '/' {
			if err := os.MkdirAll(full, 0755); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(full)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()
		}
	}
}

// collector returns a handlerFunc that appends matched paths to a slice,
// normalised to forward slashes relative to base for portable comparison.
func collector(base string, out *[]string) handlerFunc {
	return func(path string, _ bool, _, _ bool) error {
		rel, _ := filepath.Rel(base, path)
		*out = append(*out, filepath.ToSlash(rel))
		return nil
	}
}

func sortedWalk(t *testing.T, root, relPrefix string, rules []rule) []string {
	t.Helper()
	var got []string
	err := walk(root, relPrefix, rules, collector(root, &got), false, false)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	return got
}

func sortedProcessDir(t *testing.T, root string, rules []rule) []string {
	t.Helper()
	var got []string
	err := processDir(root, rules, collector(root, &got), false, false)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	return got
}

func mustParseRules(t *testing.T, src string) []rule {
	t.Helper()
	rules, err := parseRules(src)
	if err != nil {
		t.Fatalf("parseRules: %v", err)
	}
	return rules
}

func assertMatches(t *testing.T, got, want []string) {
	t.Helper()
	sort.Strings(want)
	if len(got) != len(want) {
		t.Errorf("got %v\nwant %v", got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// ------------------------------------------------------------
// Rule parsing
// ------------------------------------------------------------

func TestParseRules_Basic(t *testing.T) {
	rules := mustParseRules(t, `
# comment
*.log
!important.log
/build/
tmp/
`)
	if len(rules) != 4 {
		t.Fatalf("want 4 rules, got %d", len(rules))
	}
	if rules[0].pattern != "*.log" || rules[0].negated || rules[0].anchored || rules[0].dirOnly {
		t.Errorf("rule 0 unexpected: %+v", rules[0])
	}
	if !rules[1].negated {
		t.Errorf("rule 1 should be negated")
	}
	if !rules[2].anchored || !rules[2].dirOnly {
		t.Errorf("rule 2 should be anchored and dir-only: %+v", rules[2])
	}
	if !rules[3].dirOnly || rules[3].anchored {
		t.Errorf("rule 3 should be dir-only and not anchored: %+v", rules[3])
	}
}

func TestParseRules_PipeQualifier(t *testing.T) {
	rules := mustParseRules(t, "*.log | stat.size > 100mb\n")
	if len(rules) != 1 {
		t.Fatalf("want 1 rule, got %d", len(rules))
	}
	if rules[0].predicate == nil {
		t.Fatal("expected predicate, got nil")
	}
}

func TestParseRules_GlobWithSpaces(t *testing.T) {
	rules := mustParseRules(t, "my projects/*.log | stat.size > 1mb\n")
	if len(rules) != 1 {
		t.Fatalf("want 1 rule, got %d", len(rules))
	}
	if rules[0].pattern != "my projects/*.log" {
		t.Errorf("unexpected pattern: %q", rules[0].pattern)
	}
}

func TestParseRules_BlankAndComment(t *testing.T) {
	rules := mustParseRules(t, "\n# comment only\n\n")
	if len(rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(rules))
	}
}

// ------------------------------------------------------------
// Predicate parser
// ------------------------------------------------------------

func TestParsePredicate_Cmp(t *testing.T) {
	cases := []struct {
		src   string
		field statField
		op    op
		value int64
	}{
		{"stat.size > 100mb", fieldSize, opGt, 100 * 1024 * 1024},
		{"stat.size <= 1gb", fieldSize, opLe, 1024 * 1024 * 1024},
		{"stat.size == 512kb", fieldSize, opEq, 512 * 1024},
		{"stat.uid != 1000", fieldUID, opNe, 1000},
		{"stat.mode == 0644", fieldMode, opEq, 644},
	}
	for _, tc := range cases {
		pred, err := parsePredicate(tc.src)
		if err != nil {
			t.Errorf("%q: unexpected error: %v", tc.src, err)
			continue
		}
		c, ok := pred.(*cmpNode)
		if !ok {
			t.Errorf("%q: expected *cmpNode, got %T", tc.src, pred)
			continue
		}
		if c.field != tc.field || c.op != tc.op || c.value != tc.value {
			t.Errorf("%q: got field=%d op=%d value=%d, want %d %d %d",
				tc.src, c.field, c.op, c.value, tc.field, tc.op, tc.value)
		}
	}
}

func TestParsePredicate_And(t *testing.T) {
	pred, err := parsePredicate("stat.size > 100mb && stat.size < 1gb")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pred.(*andNode); !ok {
		t.Errorf("expected *andNode, got %T", pred)
	}
}

func TestParsePredicate_Or(t *testing.T) {
	pred, err := parsePredicate("stat.size > 1gb || stat.mtime < 0")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pred.(*orNode); !ok {
		t.Errorf("expected *orNode, got %T", pred)
	}
}

func TestParsePredicate_Precedence(t *testing.T) {
	// A || B && C  should parse as  A || (B && C)
	pred, err := parsePredicate("stat.uid == 0 || stat.size > 1mb && stat.size < 1gb")
	if err != nil {
		t.Fatal(err)
	}
	or, ok := pred.(*orNode)
	if !ok {
		t.Fatalf("expected *orNode at top, got %T", pred)
	}
	if _, ok := or.rhs.(*andNode); !ok {
		t.Errorf("expected rhs to be *andNode, got %T", or.rhs)
	}
}

func TestParsePredicate_NowRelative(t *testing.T) {
	before := time.Now().Add(-15 * 24 * time.Hour).UnixNano()
	pred, err := parsePredicate("stat.mtime < now-14d")
	if err != nil {
		t.Fatal(err)
	}
	// A file modified 15 days ago should match "mtime < now-14d".
	s := statInfo{mtime: before}
	if !pred.eval(s) {
		t.Error("15-day-old file should match mtime < now-14d")
	}
	// A file modified 1 second ago should not.
	s.mtime = time.Now().Add(-time.Second).UnixNano()
	if pred.eval(s) {
		t.Error("1-second-old file should not match mtime < now-14d")
	}
}

func TestParsePredicate_Error(t *testing.T) {
	bad := []string{
		"stat.unknown > 1",
		"size > 1",
		"stat.size",
		"stat.size >",
		"stat.size > 1mb garbage",
	}
	for _, src := range bad {
		if _, err := parsePredicate(src); err == nil {
			t.Errorf("%q: expected error, got nil", src)
		}
	}
}

// ------------------------------------------------------------
// Glob matching
// ------------------------------------------------------------

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		pattern, name string
		want          bool
	}{
		{"*.log", "error.log", true},
		{"*.log", "error.txt", false},
		{"foo*", "foobar", true},
		{"foo?", "foob", true},
		{"foo?", "foobar", false},
		{"*.log", "dir/error.log", false}, // filepath.Match does not cross /
		{"build", "build", true},
	}
	for _, tc := range cases {
		got := globMatch(tc.pattern, tc.name)
		if got != tc.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", tc.pattern, tc.name, got, tc.want)
		}
	}
}

// ------------------------------------------------------------
// Rule matching (matchRule)
// ------------------------------------------------------------

func TestMatchRule_NonAnchored(t *testing.T) {
	r := rule{pattern: "*.log"}
	cases := []struct {
		rel  string
		want bool
	}{
		{"error.log", true},
		{"logs/error.log", true},
		{"a/b/c/error.log", true},
		{"error.txt", false},
	}
	for _, tc := range cases {
		if got := matchRule(r, tc.rel); got != tc.want {
			t.Errorf("matchRule(%q, %q) = %v, want %v", r.pattern, tc.rel, got, tc.want)
		}
	}
}

func TestMatchRule_Anchored(t *testing.T) {
	r := rule{pattern: "build", anchored: true}
	if !matchRule(r, "build") {
		t.Error("anchored 'build' should match 'build'")
	}
	if matchRule(r, "sub/build") {
		t.Error("anchored 'build' should not match 'sub/build'")
	}
}

// ------------------------------------------------------------
// Evaluate (glob only)
// ------------------------------------------------------------

func TestEvaluate_LastRuleWins(t *testing.T) {
	rules := mustParseRules(t, "*.log\n!error.log\n")
	// error.log: first rule matches (delete), second negates (keep) — keep wins
	if evaluate("error.log", false, rules, nil) != keep {
		t.Error("!error.log should override *.log")
	}
	// other.log: first rule matches, no negation
	if evaluate("other.log", false, rules, nil) != delete_ {
		t.Error("other.log should be deleted by *.log")
	}
}

func TestEvaluate_DirOnly(t *testing.T) {
	rules := mustParseRules(t, "tmp/\n")
	if evaluate("tmp", true, rules, nil) != delete_ {
		t.Error("tmp directory should be deleted")
	}
	if evaluate("tmp", false, rules, nil) != keep {
		t.Error("tmp file should be kept (dir-only rule)")
	}
}

// ------------------------------------------------------------
// Evaluate with stat predicates
// ------------------------------------------------------------

func TestEvaluate_StatPredicate_Size(t *testing.T) {
	rules := mustParseRules(t, "*.log | stat.size > 1024\n")
	small := &statInfo{size: 512}
	large := &statInfo{size: 2048}

	if evaluate("app.log", false, rules, small) != keep {
		t.Error("small file should be kept")
	}
	if evaluate("app.log", false, rules, large) != delete_ {
		t.Error("large file should be deleted")
	}
}

func TestEvaluate_StatPredicate_NilStatSkipsRule(t *testing.T) {
	rules := mustParseRules(t, "*.log | stat.size > 1\n")
	// nil stat → predicate rule skipped → keep
	if evaluate("app.log", false, rules, nil) != keep {
		t.Error("nil stat should cause predicate rule to be skipped")
	}
}

func TestEvaluate_StatPredicate_AndOr(t *testing.T) {
	// Delete if size > 100mb && size < 1gb  OR  size == 0
	rules := mustParseRules(t, "*.bin | stat.size > 104857600 && stat.size < 1073741824 || stat.size == 0\n")

	empty := &statInfo{size: 0}
	mid   := &statInfo{size: 200 * 1024 * 1024}
	huge  := &statInfo{size: 2 * 1024 * 1024 * 1024}
	small := &statInfo{size: 1024}

	if evaluate("a.bin", false, rules, empty) != delete_ {
		t.Error("empty file should be deleted (size == 0)")
	}
	if evaluate("a.bin", false, rules, mid) != delete_ {
		t.Error("mid-sized file should be deleted")
	}
	if evaluate("a.bin", false, rules, huge) != keep {
		t.Error("huge file should be kept (> 1gb)")
	}
	if evaluate("a.bin", false, rules, small) != keep {
		t.Error("small non-zero file should be kept")
	}
}

func TestEvaluate_StatPredicate_Mtime(t *testing.T) {
	rules := mustParseRules(t, "*.tmp | stat.mtime < now-7d\n")
	old   := &statInfo{mtime: time.Now().Add(-8 * 24 * time.Hour).UnixNano()}
	fresh := &statInfo{mtime: time.Now().Add(-1 * time.Hour).UnixNano()}

	if evaluate("a.tmp", false, rules, old) != delete_ {
		t.Error("8-day-old file should be deleted")
	}
	if evaluate("a.tmp", false, rules, fresh) != keep {
		t.Error("1-hour-old file should be kept")
	}
}

// ------------------------------------------------------------
// Directory traversal — walk
// ------------------------------------------------------------

func TestWalk_Basic(t *testing.T) {
	tmp := t.TempDir()
	createTestFiles(t, tmp, []string{
		"build/output.txt",
		"dist/app.bin",
		"keep.txt",
		"temp/tmpfile.tmp",
		"logs/error.log",
	})

	rules := mustParseRules(t, "build/\ndist/\n*.tmp\n*.log\n!keep.txt\n")
	got := sortedWalk(t, tmp, "", rules)
	want := []string{
		"build",
		"dist",
		"logs/error.log",
		"temp/tmpfile.tmp",
	}
	assertMatches(t, got, want)
}

func TestWalk_Negation(t *testing.T) {
	tmp := t.TempDir()
	createTestFiles(t, tmp, []string{"a.log", "important.log", "sub/b.log"})

	rules := mustParseRules(t, "*.log\n!important.log\n")
	got := sortedWalk(t, tmp, "", rules)
	want := []string{"a.log", "sub/b.log"}
	assertMatches(t, got, want)
}

func TestWalk_AnchoredRule(t *testing.T) {
	tmp := t.TempDir()
	createTestFiles(t, tmp, []string{
		"build/out.txt",
		"sub/build/out.txt",
	})

	// Anchored: only top-level 'build' should match.
	rules := mustParseRules(t, "/build/\n")
	got := sortedWalk(t, tmp, "", rules)
	want := []string{"build"}
	assertMatches(t, got, want)
}

func TestWalk_NoDescendIntoDeleted(t *testing.T) {
	tmp := t.TempDir()
	createTestFiles(t, tmp, []string{
		"target/foo.o",
		"target/bar.o",
		"keep.txt",
	})

	rules := mustParseRules(t, "target/\n")
	got := sortedWalk(t, tmp, "", rules)
	// target/ is matched and reported; walk should not also report its children.
	want := []string{"target"}
	assertMatches(t, got, want)
}

func TestWalk_MultiSegmentPattern(t *testing.T) {
	tmp := t.TempDir()
	createTestFiles(t, tmp, []string{
		"a/cache/data.bin",
		"b/cache/data.bin",
		"cache/data.bin",
	})

	rules := mustParseRules(t, "cache/\n")
	got := sortedWalk(t, tmp, "", rules)
	want := []string{"a/cache", "b/cache", "cache"}
	assertMatches(t, got, want)
}

// ------------------------------------------------------------
// processDir (non-recursive)
// ------------------------------------------------------------

func TestProcessDir_NonRecursive(t *testing.T) {
	tmp := t.TempDir()
	createTestFiles(t, tmp, []string{
		"a.log",
		"b.txt",
		"sub/c.log",
	})

	rules := mustParseRules(t, "*.log\n")
	got := sortedProcessDir(t, tmp, rules)
	// Only top-level entries: sub/c.log should not appear.
	want := []string{"a.log"}
	assertMatches(t, got, want)
}

// ------------------------------------------------------------
// Integration: stat predicates with real files
// ------------------------------------------------------------

func TestWalk_StatSize_RealFiles(t *testing.T) {
	tmp := t.TempDir()

	// Write a 2KB file and a 512B file.
	write := func(name string, size int) {
		f, err := os.Create(filepath.Join(tmp, name))
		if err != nil {
			t.Fatal(err)
		}
		f.Write(make([]byte, size))
		f.Close()
	}
	write("big.log", 2048)
	write("small.log", 512)

	rules := mustParseRules(t, "*.log | stat.size > 1024\n")
	got := sortedWalk(t, tmp, "", rules)
	want := []string{"big.log"}
	assertMatches(t, got, want)
}

func TestWalk_StatMtime_RealFiles(t *testing.T) {
	tmp := t.TempDir()

	old   := filepath.Join(tmp, "old.tmp")
	fresh := filepath.Join(tmp, "fresh.tmp")

	os.WriteFile(old, []byte("x"), 0644)
	os.WriteFile(fresh, []byte("x"), 0644)

	// Back-date old.tmp by 10 days.
	past := time.Now().Add(-10 * 24 * time.Hour)
	os.Chtimes(old, past, past)

	rules := mustParseRules(t, "*.tmp | stat.mtime < now-7d\n")
	got := sortedWalk(t, tmp, "", rules)
	want := []string{"old.tmp"}
	assertMatches(t, got, want)
}
