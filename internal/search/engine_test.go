package search

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestSearchEngine_String(t *testing.T) {
	testCases := []struct {
		engine   SearchEngine
		expected string
	}{
		{EngineBuiltin, "builtin"},
		{EngineRipgrep, "ripgrep"},
		{EngineUgrep, "ugrep"},
		{SearchEngine(999), "builtin"}, // Unknown defaults to builtin
	}

	for _, tc := range testCases {
		result := tc.engine.String()
		if result != tc.expected {
			t.Errorf("SearchEngine(%d).String(): expected %q, got %q", tc.engine, tc.expected, result)
		}
	}
}

func TestDetectEngines(t *testing.T) {
	engines := DetectEngines()

	// Should always have at least builtin
	if len(engines) < 1 {
		t.Fatal("DetectEngines returned empty slice")
	}

	// First engine should be builtin
	if engines[0].Engine != EngineBuiltin {
		t.Errorf("first engine should be EngineBuiltin, got %d", engines[0].Engine)
	}
	if !engines[0].Available {
		t.Error("builtin engine should always be available")
	}

	// Verify EngineInfo fields are populated
	for _, e := range engines {
		if e.Name == "" {
			t.Errorf("engine %d has empty name", e.Engine)
		}
	}
}

func TestGetEngineByName(t *testing.T) {
	testCases := []struct {
		name     string
		expected SearchEngine
	}{
		{"builtin", EngineBuiltin},
		{"Builtin", EngineBuiltin},
		{"BUILTIN", EngineBuiltin},
		{"ripgrep", EngineRipgrep},
		{"rg", EngineRipgrep},
		{"Ripgrep", EngineRipgrep},
		{"ugrep", EngineUgrep},
		{"ug", EngineUgrep},
		{"unknown", EngineBuiltin}, // Unknown defaults to builtin
		{"", EngineBuiltin},        // Empty defaults to builtin
	}

	for _, tc := range testCases {
		result := GetEngineByName(tc.name)
		if result != tc.expected {
			t.Errorf("GetEngineByName(%q): expected %d, got %d", tc.name, tc.expected, result)
		}
	}
}

func TestGetEngineCommand(t *testing.T) {
	engines := []EngineInfo{
		{Engine: EngineBuiltin, Command: "", Available: true},
		{Engine: EngineRipgrep, Command: "/usr/bin/rg", Available: true},
		{Engine: EngineUgrep, Command: "/usr/bin/ug", Available: true},
	}

	testCases := []struct {
		engine   SearchEngine
		expected string
	}{
		{EngineBuiltin, ""},
		{EngineRipgrep, "/usr/bin/rg"},
		{EngineUgrep, "/usr/bin/ug"},
		{SearchEngine(999), ""}, // Unknown returns empty
	}

	for _, tc := range testCases {
		result := GetEngineCommand(tc.engine, engines)
		if result != tc.expected {
			t.Errorf("GetEngineCommand(%d): expected %q, got %q", tc.engine, tc.expected, result)
		}
	}
}

func TestGetEngineCommand_Unavailable(t *testing.T) {
	// Test that unavailable engines return empty command
	engines := []EngineInfo{
		{Engine: EngineRipgrep, Command: "/usr/bin/rg", Available: false},
	}

	result := GetEngineCommand(EngineRipgrep, engines)
	if result != "" {
		t.Errorf("GetEngineCommand for unavailable engine should return empty, got %q", result)
	}
}

func TestMatchesExternalResults(t *testing.T) {
	results := map[string]bool{
		"/home/user/file1.txt": true,
		"/home/user/file2.txt": true,
	}

	testCases := []struct {
		path     string
		results  map[string]bool
		expected bool
	}{
		{"/home/user/file1.txt", results, true},
		{"/home/user/file2.txt", results, true},
		{"/home/user/file3.txt", results, false},
		{"/home/user/file1.txt", nil, true}, // nil results means no external search
		{"/any/path", nil, true},            // nil results always returns true
	}

	for _, tc := range testCases {
		result := MatchesExternalResults(tc.path, tc.results)
		if result != tc.expected {
			t.Errorf("MatchesExternalResults(%q): expected %v, got %v", tc.path, tc.expected, result)
		}
	}
}

func TestSearchEngine_Constants(t *testing.T) {
	// Verify constants have distinct values
	if EngineBuiltin == EngineRipgrep || EngineRipgrep == EngineUgrep {
		t.Error("SearchEngine constants should have distinct values")
	}
}

func TestEngineInfo_Struct(t *testing.T) {
	info := EngineInfo{
		Engine:    EngineRipgrep,
		Name:      "ripgrep",
		Command:   "/usr/bin/rg",
		Available: true,
		Version:   "14.0.0",
	}

	if info.Engine != EngineRipgrep {
		t.Errorf("expected Engine=EngineRipgrep, got %d", info.Engine)
	}
	if info.Name != "ripgrep" {
		t.Errorf("expected Name='ripgrep', got %q", info.Name)
	}
	if info.Command != "/usr/bin/rg" {
		t.Errorf("expected Command='/usr/bin/rg', got %q", info.Command)
	}
	if !info.Available {
		t.Error("expected Available=true")
	}
	if info.Version != "14.0.0" {
		t.Errorf("expected Version='14.0.0', got %q", info.Version)
	}
}

// TestSearchWithEngine_Ripgrep tests the ripgrep integration if available
func TestSearchWithEngine_Ripgrep(t *testing.T) {
	// Check if ripgrep is available
	rgPath, err := exec.LookPath("rg")
	if err != nil {
		t.Skip("ripgrep not installed, skipping test")
	}

	// Create test directory with files containing searchable content
	tmpDir := t.TempDir()

	// File with match
	matchFile := filepath.Join(tmpDir, "match.txt")
	if err := os.WriteFile(matchFile, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	// File without match
	noMatchFile := filepath.Join(tmpDir, "nomatch.txt")
	if err := os.WriteFile(noMatchFile, []byte("goodbye"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results, err := SearchWithEngine(ctx, EngineRipgrep, rgPath, "hello", tmpDir, 1, nil)
	if err != nil {
		t.Fatalf("SearchWithEngine failed: %v", err)
	}

	// Should find match.txt
	found := false
	for _, path := range results {
		if filepath.Base(path) == "match.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find match.txt in results")
	}

	// Should NOT find nomatch.txt
	for _, path := range results {
		if filepath.Base(path) == "nomatch.txt" {
			t.Error("nomatch.txt should not be in results")
		}
	}
}

// TestSearchWithEngine_Ugrep tests the ugrep integration if available
func TestSearchWithEngine_Ugrep(t *testing.T) {
	// Check if ugrep is available
	ugPath, err := exec.LookPath("ug")
	if err != nil {
		ugPath, err = exec.LookPath("ugrep")
		if err != nil {
			t.Skip("ugrep not installed, skipping test")
		}
	}

	// Create test directory with files containing searchable content
	tmpDir := t.TempDir()

	// File with match
	matchFile := filepath.Join(tmpDir, "match.go")
	if err := os.WriteFile(matchFile, []byte("func main() {}"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	results, err := SearchWithEngine(ctx, EngineUgrep, ugPath, "func", tmpDir, 1, nil)
	if err != nil {
		t.Fatalf("SearchWithEngine failed: %v", err)
	}

	// Should find match.go
	found := false
	for _, path := range results {
		if filepath.Base(path) == "match.go" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected to find match.go in results")
	}
}

// TestSearchWithEngine_Builtin tests that builtin returns empty (not supported via SearchWithEngine)
func TestSearchWithEngine_Builtin(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	results, err := SearchWithEngine(ctx, EngineBuiltin, "", "test", tmpDir, 1, nil)

	// Builtin engine should return error or empty results when called via SearchWithEngine
	// because SearchWithEngine is for external engines only
	if err == nil && len(results) > 0 {
		t.Error("SearchWithEngine with EngineBuiltin should not return results")
	}
}

// TestSearchWithEngine_Cancellation tests that context cancellation works
func TestSearchWithEngine_Cancellation(t *testing.T) {
	rgPath, err := exec.LookPath("rg")
	if err != nil {
		t.Skip("ripgrep not installed, skipping cancellation test")
	}

	// Create a directory with many files to search
	tmpDir := t.TempDir()
	for i := 0; i < 100; i++ {
		filename := filepath.Join(tmpDir, "file"+string(rune('a'+i%26))+".txt")
		os.WriteFile(filename, []byte("content"), 0644)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = SearchWithEngine(ctx, EngineRipgrep, rgPath, "content", tmpDir, 10, nil)

	// Should return quickly due to cancellation
	// The error may or may not be set depending on timing
	// Just verify it doesn't hang
}

// TestSearchWithEngine_WithProgress tests progress callback
func TestSearchWithEngine_WithProgress(t *testing.T) {
	rgPath, err := exec.LookPath("rg")
	if err != nil {
		t.Skip("ripgrep not installed, skipping progress test")
	}

	tmpDir := t.TempDir()
	for i := 0; i < 20; i++ {
		filename := filepath.Join(tmpDir, "file"+string(rune('a'+i%26))+".txt")
		os.WriteFile(filename, []byte("searchterm"), 0644)
	}

	ctx := context.Background()
	progressCalled := false
	progressFn := func(count int) {
		progressCalled = true
	}

	_, err = SearchWithEngine(ctx, EngineRipgrep, rgPath, "searchterm", tmpDir, 1, progressFn)
	if err != nil {
		t.Fatalf("SearchWithEngine failed: %v", err)
	}

	if !progressCalled {
		t.Error("progress callback was not called")
	}
}

// Benchmark for GetEngineByName
func BenchmarkGetEngineByName(b *testing.B) {
	names := []string{"builtin", "ripgrep", "ugrep", "unknown"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, name := range names {
			GetEngineByName(name)
		}
	}
}
