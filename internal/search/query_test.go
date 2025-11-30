package search

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParse_Empty(t *testing.T) {
	q := Parse("")
	if !q.IsEmpty() {
		t.Errorf("expected empty query, got %d directives", len(q.Directives))
	}
	if q.Raw != "" {
		t.Errorf("expected empty raw, got %q", q.Raw)
	}
}

func TestParse_SimpleFilename(t *testing.T) {
	q := Parse("test.go")
	if len(q.Directives) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(q.Directives))
	}
	d := q.Directives[0]
	if d.Type != DirFilename {
		t.Errorf("expected DirFilename, got %d", d.Type)
	}
	if d.Value != "test.go" {
		t.Errorf("expected value 'test.go', got %q", d.Value)
	}
}

func TestParse_ContentsDirective(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"contents:hello", "hello"},
		{"content:world", "world"},
		{"text:foo", "foo"},
		{"body:bar", "bar"},
	}

	for _, tc := range testCases {
		q := Parse(tc.input)
		if len(q.Directives) != 1 {
			t.Fatalf("input %q: expected 1 directive, got %d", tc.input, len(q.Directives))
		}
		d := q.Directives[0]
		if d.Type != DirContents {
			t.Errorf("input %q: expected DirContents, got %d", tc.input, d.Type)
		}
		if d.Value != tc.expected {
			t.Errorf("input %q: expected value %q, got %q", tc.input, tc.expected, d.Value)
		}
	}
}

func TestParse_ExtDirective(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"ext:go", ".go"},
		{"ext:.go", ".go"},
		{"extension:txt", ".txt"},
		{"type:md", ".md"},
	}

	for _, tc := range testCases {
		q := Parse(tc.input)
		if len(q.Directives) != 1 {
			t.Fatalf("input %q: expected 1 directive, got %d", tc.input, len(q.Directives))
		}
		d := q.Directives[0]
		if d.Type != DirExt {
			t.Errorf("input %q: expected DirExt, got %d", tc.input, d.Type)
		}
		if d.Value != tc.expected {
			t.Errorf("input %q: expected value %q, got %q", tc.input, tc.expected, d.Value)
		}
	}
}

func TestParse_SizeDirective(t *testing.T) {
	testCases := []struct {
		input      string
		expectedOp Operator
		expectedSz int64
	}{
		{"size:>1KB", OpGreater, 1024},
		{"size:<10MB", OpLess, 10 * 1024 * 1024},
		{"size:>=1GB", OpGreaterEq, 1024 * 1024 * 1024},
		{"size:<=500B", OpLessEq, 500},
		{"size:=1024", OpEquals, 1024},
		{"size:2048", OpEquals, 2048}, // No operator means equals
	}

	for _, tc := range testCases {
		q := Parse(tc.input)
		if len(q.Directives) != 1 {
			t.Fatalf("input %q: expected 1 directive, got %d", tc.input, len(q.Directives))
		}
		d := q.Directives[0]
		if d.Type != DirSize {
			t.Errorf("input %q: expected DirSize, got %d", tc.input, d.Type)
		}
		if d.Operator != tc.expectedOp {
			t.Errorf("input %q: expected operator %d, got %d", tc.input, tc.expectedOp, d.Operator)
		}
		if d.NumValue != tc.expectedSz {
			t.Errorf("input %q: expected size %d, got %d", tc.input, tc.expectedSz, d.NumValue)
		}
	}
}

func TestParse_ModifiedDirective(t *testing.T) {
	testCases := []struct {
		input      string
		expectedOp Operator
		checkDate  func(t time.Time) bool
	}{
		{
			"modified:>2024-01-01",
			OpGreater,
			func(t time.Time) bool { return t.Year() == 2024 && t.Month() == 1 && t.Day() == 1 },
		},
		{
			"date:<2023-06-15",
			OpLess,
			func(t time.Time) bool { return t.Year() == 2023 && t.Month() == 6 && t.Day() == 15 },
		},
	}

	for _, tc := range testCases {
		q := Parse(tc.input)
		if len(q.Directives) != 1 {
			t.Fatalf("input %q: expected 1 directive, got %d", tc.input, len(q.Directives))
		}
		d := q.Directives[0]
		if d.Type != DirModified {
			t.Errorf("input %q: expected DirModified, got %d", tc.input, d.Type)
		}
		if d.Operator != tc.expectedOp {
			t.Errorf("input %q: expected operator %d, got %d", tc.input, tc.expectedOp, d.Operator)
		}
		if !tc.checkDate(d.TimeVal) {
			t.Errorf("input %q: date check failed, got %v", tc.input, d.TimeVal)
		}
	}
}

func TestParse_ModifiedRelative(t *testing.T) {
	today := time.Now()
	yesterday := today.AddDate(0, 0, -1)

	testCases := []struct {
		input    string
		checkDay func(t time.Time) bool
	}{
		{
			"modified:today",
			func(t time.Time) bool {
				y, m, d := today.Date()
				ty, tm, td := t.Date()
				return y == ty && m == tm && d == td
			},
		},
		{
			"modified:yesterday",
			func(t time.Time) bool {
				y, m, d := yesterday.Date()
				ty, tm, td := t.Date()
				return y == ty && m == tm && d == td
			},
		},
	}

	for _, tc := range testCases {
		q := Parse(tc.input)
		if len(q.Directives) != 1 {
			t.Fatalf("input %q: expected 1 directive, got %d", tc.input, len(q.Directives))
		}
		d := q.Directives[0]
		if !tc.checkDay(d.TimeVal) {
			t.Errorf("input %q: date check failed, got %v", tc.input, d.TimeVal)
		}
	}
}

func TestParse_RecursiveDirective(t *testing.T) {
	testCases := []struct {
		input         string
		expectedDepth int64
	}{
		{"recursive:", 2},   // Default depth
		{"recursive:5", 5},  // Explicit depth
		{"recurse:10", 10},  // Alias
		{"r:3", 3},          // Short alias
		{"depth:7", 7},      // Depth alias
	}

	for _, tc := range testCases {
		q := Parse(tc.input)
		if len(q.Directives) != 1 {
			t.Fatalf("input %q: expected 1 directive, got %d", tc.input, len(q.Directives))
		}
		d := q.Directives[0]
		if d.Type != DirRecursive {
			t.Errorf("input %q: expected DirRecursive, got %d", tc.input, d.Type)
		}
		if d.NumValue != tc.expectedDepth {
			t.Errorf("input %q: expected depth %d, got %d", tc.input, tc.expectedDepth, d.NumValue)
		}
	}
}

func TestParse_MultipleDirectives(t *testing.T) {
	q := Parse("*.go contents:func ext:go size:>1KB")
	if len(q.Directives) != 4 {
		t.Fatalf("expected 4 directives, got %d", len(q.Directives))
	}

	expected := []DirectiveType{DirFilename, DirContents, DirExt, DirSize}
	for i, d := range q.Directives {
		if d.Type != expected[i] {
			t.Errorf("directive %d: expected type %d, got %d", i, expected[i], d.Type)
		}
	}
}

func TestParse_QuotedValues(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{`contents:"hello world"`, "hello world"},
		{`contents:'foo bar'`, "foo bar"},
		{`filename:"my file.txt"`, "my file.txt"},
	}

	for _, tc := range testCases {
		q := Parse(tc.input)
		if len(q.Directives) != 1 {
			t.Fatalf("input %q: expected 1 directive, got %d", tc.input, len(q.Directives))
		}
		if q.Directives[0].Value != tc.expected {
			t.Errorf("input %q: expected value %q, got %q", tc.input, tc.expected, q.Directives[0].Value)
		}
	}
}

func TestSplitRespectingQuotes(t *testing.T) {
	testCases := []struct {
		input    string
		expected []string
	}{
		{"foo bar baz", []string{"foo", "bar", "baz"}},
		{`"foo bar" baz`, []string{"foo bar", "baz"}},
		{`'foo bar' baz`, []string{"foo bar", "baz"}},
		{`foo "bar baz"`, []string{"foo", "bar baz"}},
		{"", []string{}},
		{"single", []string{"single"}},
	}

	for _, tc := range testCases {
		result := splitRespectingQuotes(tc.input)
		if len(result) != len(tc.expected) {
			t.Fatalf("input %q: expected %d parts, got %d: %v", tc.input, len(tc.expected), len(result), result)
		}
		for i, p := range result {
			if p != tc.expected[i] {
				t.Errorf("input %q: part %d: expected %q, got %q", tc.input, i, tc.expected[i], p)
			}
		}
	}
}

func TestParseSize(t *testing.T) {
	testCases := []struct {
		input    string
		expected int64
	}{
		{"100", 100},
		{"1KB", 1024},
		{"1kb", 1024},
		{"10MB", 10 * 1024 * 1024},
		{"1GB", 1024 * 1024 * 1024},
		{"500B", 500},
		{"1.5MB", int64(1.5 * 1024 * 1024)},
		{"invalid", 0},
		{"", 0},
	}

	for _, tc := range testCases {
		result := parseSize(tc.input)
		if result != tc.expected {
			t.Errorf("parseSize(%q): expected %d, got %d", tc.input, tc.expected, result)
		}
	}
}

func TestParseOperator(t *testing.T) {
	testCases := []struct {
		input       string
		expectedOp  Operator
		expectedVal string
	}{
		{">100", OpGreater, "100"},
		{"<50", OpLess, "50"},
		{">=200", OpGreaterEq, "200"},
		{"<=300", OpLessEq, "300"},
		{"=400", OpEquals, "400"},
		{"500", OpEquals, "500"},
	}

	for _, tc := range testCases {
		op, val := parseOperator(tc.input)
		if op != tc.expectedOp {
			t.Errorf("parseOperator(%q): expected op %d, got %d", tc.input, tc.expectedOp, op)
		}
		if val != tc.expectedVal {
			t.Errorf("parseOperator(%q): expected val %q, got %q", tc.input, tc.expectedVal, val)
		}
	}
}

func TestMatchGlob(t *testing.T) {
	testCases := []struct {
		name     string
		pattern  string
		expected bool
	}{
		// Substring match (no wildcards)
		{"test.go", "test", true},
		{"test.go", "go", true},
		{"test.go", "txt", false},

		// Wildcard matches
		{"test.go", "*.go", true},
		{"test.go", "test.*", true},
		{"test.go", "*", true},
		{"test.go", "*.txt", false},
		{"main.go", "*.go", true},
		{"hello_world.go", "*_*", true},
		{"helloworld.go", "*_*", false},

		// Prefix/suffix wildcards
		{"prefix_file.txt", "prefix_*", true},
		{"file_suffix.txt", "*_suffix.txt", true},
	}

	for _, tc := range testCases {
		result := MatchGlob(tc.name, tc.pattern)
		if result != tc.expected {
			t.Errorf("MatchGlob(%q, %q): expected %v, got %v", tc.name, tc.pattern, tc.expected, result)
		}
	}
}

func TestCompareInt(t *testing.T) {
	testCases := []struct {
		val      int64
		target   int64
		op       Operator
		expected bool
	}{
		{100, 50, OpGreater, true},
		{50, 100, OpGreater, false},
		{100, 100, OpGreater, false},

		{50, 100, OpLess, true},
		{100, 50, OpLess, false},
		{100, 100, OpLess, false},

		{100, 100, OpGreaterEq, true},
		{101, 100, OpGreaterEq, true},
		{99, 100, OpGreaterEq, false},

		{100, 100, OpLessEq, true},
		{99, 100, OpLessEq, true},
		{101, 100, OpLessEq, false},

		{100, 100, OpEquals, true},
		{99, 100, OpEquals, false},
	}

	for _, tc := range testCases {
		result := CompareInt(tc.val, tc.target, tc.op)
		if result != tc.expected {
			t.Errorf("CompareInt(%d, %d, %d): expected %v, got %v", tc.val, tc.target, tc.op, tc.expected, result)
		}
	}
}

func TestCompareTime(t *testing.T) {
	base := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	before := base.Add(-24 * time.Hour)
	after := base.Add(24 * time.Hour)

	testCases := []struct {
		val      time.Time
		target   time.Time
		op       Operator
		expected bool
	}{
		{after, base, OpGreater, true},
		{before, base, OpGreater, false},
		{base, base, OpGreater, false},

		{before, base, OpLess, true},
		{after, base, OpLess, false},
		{base, base, OpLess, false},

		{after, base, OpGreaterEq, true},
		{base, base, OpGreaterEq, true},
		{before, base, OpGreaterEq, false},

		{before, base, OpLessEq, true},
		{base, base, OpLessEq, true},
		{after, base, OpLessEq, false},
	}

	for _, tc := range testCases {
		result := CompareTime(tc.val, tc.target, tc.op)
		if result != tc.expected {
			t.Errorf("CompareTime(%v, %v, %d): expected %v, got %v", tc.val, tc.target, tc.op, tc.expected, result)
		}
	}
}

func TestQuery_HasContentSearch(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"contents:foo", true},
		{"content:bar", true},
		{"text:baz", true},
		{"body:qux", true},
		{"filename:test", false},
		{"ext:go", false},
		{"foo bar", false},
		{"", false},
	}

	for _, tc := range testCases {
		q := Parse(tc.input)
		if q.HasContentSearch() != tc.expected {
			t.Errorf("Parse(%q).HasContentSearch(): expected %v, got %v", tc.input, tc.expected, !tc.expected)
		}
	}
}

func TestQuery_HasRecursive(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
	}{
		{"recursive:", true},
		{"recursive:5", true},
		{"recurse:10", true},
		{"r:3", true},
		{"depth:7", true},
		{"contents:foo", false},
		{"ext:go", false},
		{"", false},
	}

	for _, tc := range testCases {
		q := Parse(tc.input)
		if q.HasRecursive() != tc.expected {
			t.Errorf("Parse(%q).HasRecursive(): expected %v, got %v", tc.input, tc.expected, !tc.expected)
		}
	}
}

func TestQuery_GetRecursiveDepth(t *testing.T) {
	testCases := []struct {
		input        string
		defaultDepth int
		expected     int
	}{
		{"recursive:5", 10, 5},
		{"recursive:", 10, 2},    // Empty value uses hardcoded default of 2 from parseDirective
		{"depth:3", 10, 3},
		{"filename:test", 10, 1}, // Not recursive, returns 1
		{"", 10, 1},              // Empty, returns 1
	}

	for _, tc := range testCases {
		q := Parse(tc.input)
		result := q.GetRecursiveDepth(tc.defaultDepth)
		if result != tc.expected {
			t.Errorf("Parse(%q).GetRecursiveDepth(%d): expected %d, got %d", tc.input, tc.defaultDepth, tc.expected, result)
		}
	}
}

func TestQuery_GetContentPattern(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"contents:hello", "hello"},
		{"content:world", "world"},
		{"ext:go", ""},
		{"", ""},
	}

	for _, tc := range testCases {
		q := Parse(tc.input)
		result := q.GetContentPattern()
		if result != tc.expected {
			t.Errorf("Parse(%q).GetContentPattern(): expected %q, got %q", tc.input, tc.expected, result)
		}
	}
}

// TestMatcher_Match tests the Matcher.Match function with file info
func TestMatcher_Match(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create test files
	testFile := filepath.Join(tmpDir, "test.go")
	if err := os.WriteFile(testFile, []byte("package main\nfunc main() {}"), 0644); err != nil {
		t.Fatal(err)
	}

	smallFile := filepath.Join(tmpDir, "small.txt")
	if err := os.WriteFile(smallFile, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	largeFile := filepath.Join(tmpDir, "large.txt")
	largeContent := make([]byte, 2048)
	if err := os.WriteFile(largeFile, largeContent, 0644); err != nil {
		t.Fatal(err)
	}

	testCases := []struct {
		query    string
		path     string
		expected bool
	}{
		// Filename matching
		{"test", testFile, true},
		{"*.go", testFile, true},
		{"*.txt", testFile, false},
		{"small", smallFile, true},

		// Extension matching
		{"ext:go", testFile, true},
		{"ext:txt", testFile, false},
		{"ext:txt", smallFile, true},

		// Size matching
		{"size:<100", smallFile, true},
		{"size:>100", smallFile, false},
		{"size:>1000", largeFile, true},
		{"size:<1000", largeFile, false},

		// Content matching
		{"contents:func", testFile, true},
		{"contents:notfound", testFile, false},
		{"contents:hello", smallFile, true},
	}

	for _, tc := range testCases {
		q := Parse(tc.query)
		m := NewMatcher(q)
		info, err := os.Stat(tc.path)
		if err != nil {
			t.Fatalf("could not stat %s: %v", tc.path, err)
		}

		result := m.Match(tc.path, info)
		if result != tc.expected {
			t.Errorf("Match(%q, %s): expected %v, got %v", tc.query, filepath.Base(tc.path), tc.expected, result)
		}
	}
}
