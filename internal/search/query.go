package search

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Directive types
type DirectiveType int

const (
	DirFilename DirectiveType = iota
	DirContents
	DirExt
	DirSize
	DirModified
	DirRecursive
)

// Comparison operators for size/date
type Operator int

const (
	OpNone Operator = iota
	OpGreater
	OpLess
	OpGreaterEq
	OpLessEq
	OpEquals
)

// Directive represents a single search directive
type Directive struct {
	Type     DirectiveType
	Value    string
	Operator Operator
	NumValue int64     // Parsed size in bytes
	TimeVal  time.Time // Parsed date
}

// Query holds parsed search directives
type Query struct {
	Directives []Directive
	Raw        string
}

// Parse parses a search string into directives
// Examples:
//   - "foo" -> filename:foo
//   - "contents:hello" -> search file contents for "hello"
//   - "ext:go" -> files with .go extension
//   - "size:>1MB" -> files larger than 1MB
//   - "modified:>2024-01-01" -> files modified after Jan 1, 2024
func Parse(input string) *Query {
	q := &Query{Raw: input}
	input = strings.TrimSpace(input)
	if input == "" {
		return q
	}

	// Split by spaces, but respect quotes
	parts := splitRespectingQuotes(input)

	for _, part := range parts {
		d := parseDirective(part)
		q.Directives = append(q.Directives, d)
	}

	return q
}

func splitRespectingQuotes(s string) []string {
	var parts []string
	var current strings.Builder
	inQuotes := false
	quoteChar := rune(0)

	for _, r := range s {
		switch {
		case (r == '"' || r == '\'') && !inQuotes:
			inQuotes = true
			quoteChar = r
		case r == quoteChar && inQuotes:
			inQuotes = false
			quoteChar = 0
		case r == ' ' && !inQuotes:
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

func parseDirective(s string) Directive {
	// Check for directive:value pattern
	if idx := strings.Index(s, ":"); idx > 0 {
		directive := strings.ToLower(s[:idx])
		value := s[idx+1:]
		value = strings.Trim(value, "\"'")

		switch directive {
		case "filename", "name", "file":
			return Directive{Type: DirFilename, Value: value}

		case "contents", "content", "text", "body":
			return Directive{Type: DirContents, Value: value}

		case "ext", "extension", "type":
			if !strings.HasPrefix(value, ".") {
				value = "." + value
			}
			return Directive{Type: DirExt, Value: strings.ToLower(value)}

		case "size":
			op, numStr := parseOperator(value)
			bytes := parseSize(numStr)
			return Directive{Type: DirSize, Value: value, Operator: op, NumValue: bytes}

		case "modified", "date", "mtime":
			op, dateStr := parseOperator(value)
			t := parseDate(dateStr)
			return Directive{Type: DirModified, Value: value, Operator: op, TimeVal: t}

		case "recursive", "recurse", "r", "depth":
			// Parse depth value, default to 10 if not specified or invalid
			depth := int64(10)
			if value != "" {
				if d, err := strconv.ParseInt(value, 10, 64); err == nil && d > 0 {
					depth = d
				}
			}
			return Directive{Type: DirRecursive, Value: value, NumValue: depth}
		}
	}

	// Default to filename search
	return Directive{Type: DirFilename, Value: s}
}

func parseOperator(s string) (Operator, string) {
	s = strings.TrimSpace(s)
	switch {
	case strings.HasPrefix(s, ">="):
		return OpGreaterEq, strings.TrimSpace(s[2:])
	case strings.HasPrefix(s, "<="):
		return OpLessEq, strings.TrimSpace(s[2:])
	case strings.HasPrefix(s, ">"):
		return OpGreater, strings.TrimSpace(s[1:])
	case strings.HasPrefix(s, "<"):
		return OpLess, strings.TrimSpace(s[1:])
	case strings.HasPrefix(s, "="):
		return OpEquals, strings.TrimSpace(s[1:])
	default:
		return OpEquals, s
	}
}

// parseSize converts size strings like "1KB", "10MB", "1GB" to bytes
func parseSize(s string) int64 {
	s = strings.ToUpper(strings.TrimSpace(s))

	multiplier := int64(1)
	numStr := s

	switch {
	case strings.HasSuffix(s, "GB"):
		multiplier = 1024 * 1024 * 1024
		numStr = s[:len(s)-2]
	case strings.HasSuffix(s, "MB"):
		multiplier = 1024 * 1024
		numStr = s[:len(s)-2]
	case strings.HasSuffix(s, "KB"):
		multiplier = 1024
		numStr = s[:len(s)-2]
	case strings.HasSuffix(s, "B"):
		numStr = s[:len(s)-1]
	}

	n, err := strconv.ParseFloat(strings.TrimSpace(numStr), 64)
	if err != nil {
		return 0
	}

	return int64(n * float64(multiplier))
}

// parseDate parses date strings like "2024-01-01", "2024-01", "today", "yesterday"
func parseDate(s string) time.Time {
	s = strings.ToLower(strings.TrimSpace(s))
	now := time.Now()

	switch s {
	case "today":
		y, m, d := now.Date()
		return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	case "yesterday":
		y, m, d := now.AddDate(0, 0, -1).Date()
		return time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	case "week":
		return now.AddDate(0, 0, -7)
	case "month":
		return now.AddDate(0, -1, 0)
	case "year":
		return now.AddDate(-1, 0, 0)
	}

	// Try various date formats
	formats := []string{
		"2006-01-02",
		"2006-01",
		"2006/01/02",
		"01/02/2006",
		"Jan 2, 2006",
	}

	for _, fmt := range formats {
		if t, err := time.Parse(fmt, s); err == nil {
			return t
		}
	}

	return time.Time{}
}

// Matcher evaluates files against a query
type Matcher struct {
	query           *Query
	contentFunc     func(path string) (string, error)
	ctx             context.Context
	externalResults map[string]bool // Results from external search engine (ripgrep/ugrep)
}

// NewMatcher creates a new Matcher for the given query
func NewMatcher(q *Query) *Matcher {
	return &Matcher{
		query: q,
		ctx:   context.Background(),
		contentFunc: func(path string) (string, error) {
			data, err := os.ReadFile(path)
			if err != nil {
				return "", err
			}
			return string(data), nil
		},
	}
}

// NewMatcherWithContext creates a new Matcher with context for cancellation
func NewMatcherWithContext(ctx context.Context, q *Query) *Matcher {
	return &Matcher{
		query: q,
		ctx:   ctx,
		contentFunc: func(path string) (string, error) {
			// Check context before reading
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			
			// Open file with a timeout approach - read in chunks and check context
			f, err := os.Open(path)
			if err != nil {
				return "", err
			}
			defer f.Close()
			
			// Read file in chunks, checking context periodically
			const chunkSize = 64 * 1024 // 64KB chunks
			var buf strings.Builder
			chunk := make([]byte, chunkSize)
			
			for {
				if ctx.Err() != nil {
					return "", ctx.Err()
				}
				
				n, err := f.Read(chunk)
				if n > 0 {
					// Check for binary content (null bytes) - skip binary files
					for i := 0; i < n; i++ {
						if chunk[i] == 0 {
							return "", nil // Empty string means no match for binary files
						}
					}
					buf.Write(chunk[:n])
				}
				if err == io.EOF {
					break
				}
				if err != nil {
					return "", err
				}
			}
			
			return buf.String(), nil
		},
	}
}

// SetContentFunc allows setting a custom content reader (e.g., for mmap or caching)
func (m *Matcher) SetContentFunc(f func(path string) (string, error)) {
	m.contentFunc = f
}

// SetExternalResults sets the results from an external search engine
// When set, content matching will check against these results instead of reading files
func (m *Matcher) SetExternalResults(results map[string]bool) {
	m.externalResults = results
}

// Match checks if a file matches all directives in the query (AND logic)
func (m *Matcher) Match(path string, info os.FileInfo) bool {
	if len(m.query.Directives) == 0 {
		return true
	}

	// All directives must match (implicit AND)
	for _, d := range m.query.Directives {
		if !m.matchDirective(d, path, info) {
			return false
		}
	}

	return true
}

func (m *Matcher) matchDirective(d Directive, path string, info os.FileInfo) bool {
	switch d.Type {
	case DirFilename:
		return MatchGlob(strings.ToLower(info.Name()), strings.ToLower(d.Value))

	case DirContents:
		if info.IsDir() {
			return false
		}
		
		// If we have external results (from ripgrep/ugrep), use those
		if m.externalResults != nil {
			// Normalize path for comparison
			absPath, _ := filepath.Abs(path)
			return m.externalResults[absPath] || m.externalResults[path]
		}
		
		// Builtin content search
		// Skip large files (>10MB) for content search
		if info.Size() > 10*1024*1024 {
			return false
		}
		content, err := m.contentFunc(path)
		if err != nil {
			return false
		}
		return strings.Contains(strings.ToLower(content), strings.ToLower(d.Value))

	case DirExt:
		ext := strings.ToLower(filepath.Ext(info.Name()))
		return ext == d.Value

	case DirSize:
		return CompareInt(info.Size(), d.NumValue, d.Operator)

	case DirModified:
		if d.TimeVal.IsZero() {
			return true
		}
		return CompareTime(info.ModTime(), d.TimeVal, d.Operator)

	case DirRecursive:
		// Recursive is a control directive, not a filter - always matches
		return true
	}

	return true
}

// MatchGlob does simple glob matching with * wildcards
func MatchGlob(name, pattern string) bool {
	// If pattern has no wildcards, do substring match
	if !strings.Contains(pattern, "*") {
		return strings.Contains(name, pattern)
	}

	// Simple glob matching
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return name == pattern
	}

	// Check prefix
	if parts[0] != "" && !strings.HasPrefix(name, parts[0]) {
		return false
	}

	// Check suffix
	last := parts[len(parts)-1]
	if last != "" && !strings.HasSuffix(name, last) {
		return false
	}

	// Check middle parts exist in order
	pos := len(parts[0])
	for _, part := range parts[1 : len(parts)-1] {
		if part == "" {
			continue
		}
		idx := strings.Index(name[pos:], part)
		if idx < 0 {
			return false
		}
		pos += idx + len(part)
	}

	return true
}

// CompareInt compares two integers with an operator
func CompareInt(val, target int64, op Operator) bool {
	switch op {
	case OpGreater:
		return val > target
	case OpLess:
		return val < target
	case OpGreaterEq:
		return val >= target
	case OpLessEq:
		return val <= target
	default:
		return val == target
	}
}

// CompareTime compares two times with an operator
func CompareTime(val, target time.Time, op Operator) bool {
	switch op {
	case OpGreater:
		return val.After(target)
	case OpLess:
		return val.Before(target)
	case OpGreaterEq:
		return val.After(target) || val.Equal(target)
	case OpLessEq:
		return val.Before(target) || val.Equal(target)
	default:
		// For equals, compare just the date part
		vy, vm, vd := val.Date()
		ty, tm, td := target.Date()
		return vy == ty && vm == tm && vd == td
	}
}

// HasContentSearch returns true if query includes content search
func (q *Query) HasContentSearch() bool {
	for _, d := range q.Directives {
		if d.Type == DirContents {
			return true
		}
	}
	return false
}

// GetContentPattern returns the content search pattern (first contents: directive value)
func (q *Query) GetContentPattern() string {
	for _, d := range q.Directives {
		if d.Type == DirContents {
			return d.Value
		}
	}
	return ""
}

// HasRecursive returns true if query includes recursive directive
func (q *Query) HasRecursive() bool {
	for _, d := range q.Directives {
		if d.Type == DirRecursive {
			return true
		}
	}
	return false
}

// GetRecursiveDepth returns the recursive depth (default 10 if recursive but no depth specified)
func (q *Query) GetRecursiveDepth() int {
	for _, d := range q.Directives {
		if d.Type == DirRecursive {
			if d.NumValue > 0 {
				return int(d.NumValue)
			}
			return 10 // Default depth
		}
	}
	return 1 // Not recursive, depth 1
}

// IsEmpty returns true if query has no directives
func (q *Query) IsEmpty() bool {
	return len(q.Directives) == 0
}
