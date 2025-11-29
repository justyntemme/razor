package search

import (
	"os"
	"testing"
	"time"
)

func TestParseSimpleFilename(t *testing.T) {
	q := Parse("readme")
	if len(q.Directives) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(q.Directives))
	}
	if q.Directives[0].Type != DirFilename {
		t.Errorf("expected DirFilename, got %d", q.Directives[0].Type)
	}
	if q.Directives[0].Value != "readme" {
		t.Errorf("expected 'readme', got '%s'", q.Directives[0].Value)
	}
}

func TestParseContentsDirective(t *testing.T) {
	q := Parse("contents:TODO")
	if len(q.Directives) != 1 {
		t.Fatalf("expected 1 directive, got %d", len(q.Directives))
	}
	if q.Directives[0].Type != DirContents {
		t.Errorf("expected DirContents, got %d", q.Directives[0].Type)
	}
	if q.Directives[0].Value != "TODO" {
		t.Errorf("expected 'TODO', got '%s'", q.Directives[0].Value)
	}
}

func TestParseExtDirective(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"ext:go", ".go"},
		{"ext:.go", ".go"},
		{"extension:txt", ".txt"},
		{"type:MD", ".md"},
	}

	for _, tt := range tests {
		q := Parse(tt.input)
		if len(q.Directives) != 1 {
			t.Fatalf("input %s: expected 1 directive, got %d", tt.input, len(q.Directives))
		}
		if q.Directives[0].Type != DirExt {
			t.Errorf("input %s: expected DirExt, got %d", tt.input, q.Directives[0].Type)
		}
		if q.Directives[0].Value != tt.expected {
			t.Errorf("input %s: expected '%s', got '%s'", tt.input, tt.expected, q.Directives[0].Value)
		}
	}
}

func TestParseSizeDirective(t *testing.T) {
	tests := []struct {
		input    string
		op       Operator
		expected int64
	}{
		{"size:>1MB", OpGreater, 1024 * 1024},
		{"size:<100KB", OpLess, 100 * 1024},
		{"size:>=500B", OpGreaterEq, 500},
		{"size:1GB", OpEquals, 1024 * 1024 * 1024},
		{"size:<=2MB", OpLessEq, 2 * 1024 * 1024},
	}

	for _, tt := range tests {
		q := Parse(tt.input)
		if len(q.Directives) != 1 {
			t.Fatalf("input %s: expected 1 directive, got %d", tt.input, len(q.Directives))
		}
		d := q.Directives[0]
		if d.Type != DirSize {
			t.Errorf("input %s: expected DirSize, got %d", tt.input, d.Type)
		}
		if d.Operator != tt.op {
			t.Errorf("input %s: expected operator %d, got %d", tt.input, tt.op, d.Operator)
		}
		if d.NumValue != tt.expected {
			t.Errorf("input %s: expected %d bytes, got %d", tt.input, tt.expected, d.NumValue)
		}
	}
}

func TestParseModifiedDirective(t *testing.T) {
	tests := []struct {
		input string
		op    Operator
	}{
		{"modified:>2024-01-01", OpGreater},
		{"modified:<2024-06-15", OpLess},
		{"modified:>=2024-01", OpGreaterEq},
		{"modified:today", OpEquals},
		{"date:yesterday", OpEquals},
	}

	for _, tt := range tests {
		q := Parse(tt.input)
		if len(q.Directives) != 1 {
			t.Fatalf("input %s: expected 1 directive, got %d", tt.input, len(q.Directives))
		}
		d := q.Directives[0]
		if d.Type != DirModified {
			t.Errorf("input %s: expected DirModified, got %d", tt.input, d.Type)
		}
		if d.Operator != tt.op {
			t.Errorf("input %s: expected operator %d, got %d", tt.input, tt.op, d.Operator)
		}
	}
}

func TestParseCombinedDirectives(t *testing.T) {
	q := Parse("ext:go contents:func size:>1KB")
	if len(q.Directives) != 3 {
		t.Fatalf("expected 3 directives, got %d", len(q.Directives))
	}
	if q.Directives[0].Type != DirExt {
		t.Errorf("expected first directive to be DirExt")
	}
	if q.Directives[1].Type != DirContents {
		t.Errorf("expected second directive to be DirContents")
	}
	if q.Directives[2].Type != DirSize {
		t.Errorf("expected third directive to be DirSize")
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		match   bool
	}{
		{"readme.md", "readme", true},
		{"README.md", "readme", true}, // case insensitive
		{"test.go", "*.go", true},
		{"main_test.go", "*_test.go", true},
		{"main.go", "*_test.go", false},
		{"config.yaml", "config*", true},
		{"myconfig.yaml", "*config*", true},
	}

	for _, tt := range tests {
		result := matchGlob(tt.name, tt.pattern)
		if result != tt.match {
			t.Errorf("matchGlob(%s, %s) = %v, want %v", tt.name, tt.pattern, result, tt.match)
		}
	}
}

func TestHasContentSearch(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"readme", false},
		{"ext:go", false},
		{"contents:TODO", true},
		{"ext:go contents:func", true},
		{"size:>1MB", false},
	}

	for _, tt := range tests {
		q := Parse(tt.input)
		if q.HasContentSearch() != tt.expected {
			t.Errorf("Parse(%s).HasContentSearch() = %v, want %v", tt.input, q.HasContentSearch(), tt.expected)
		}
	}
}

// mockFileInfo implements os.FileInfo for testing
type mockFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
}

func (m mockFileInfo) Name() string       { return m.name }
func (m mockFileInfo) Size() int64        { return m.size }
func (m mockFileInfo) Mode() os.FileMode  { return m.mode }
func (m mockFileInfo) ModTime() time.Time { return m.modTime }
func (m mockFileInfo) IsDir() bool        { return m.isDir }
func (m mockFileInfo) Sys() interface{}   { return nil }

func TestMatcherMatch(t *testing.T) {
	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7)

	tests := []struct {
		query   string
		name    string
		size    int64
		modTime time.Time
		match   bool
	}{
		{"readme", "README.md", 100, now, true},
		{"readme", "main.go", 100, now, false},
		{"ext:go", "main.go", 100, now, true},
		{"ext:go", "main.py", 100, now, false},
		{"size:>1KB", "large.txt", 2000, now, true},
		{"size:>1KB", "small.txt", 500, now, false},
		{"size:<1KB", "small.txt", 500, now, true},
		{"modified:>week", "new.txt", 100, now, true},
		{"modified:>week", "old.txt", 100, weekAgo.AddDate(0, 0, -1), false},
	}

	for _, tt := range tests {
		q := Parse(tt.query)
		m := NewMatcher(q)
		// Override content func to avoid actual file reads
		m.SetContentFunc(func(path string) (string, error) {
			return "", nil
		})

		info := mockFileInfo{
			name:    tt.name,
			size:    tt.size,
			modTime: tt.modTime,
		}

		result := m.Match("/fake/"+tt.name, info)
		if result != tt.match {
			t.Errorf("Match(query=%s, name=%s, size=%d) = %v, want %v",
				tt.query, tt.name, tt.size, result, tt.match)
		}
	}
}