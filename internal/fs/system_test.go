package fs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShouldSkipPath(t *testing.T) {
	testCases := []struct {
		path     string
		expected bool
	}{
		// Root-level system directories
		{"/dev", true},
		{"/proc", true},
		{"/sys", true},
		{"/run", true},
		{"/snap", true},
		{"/boot", true},
		{"/lost+found", true},

		// Subdirectories of system directories
		{"/dev/null", true},
		{"/proc/1/status", true},
		{"/sys/class/net", true},

		// Normal directories
		{"/home", false},
		{"/home/user", false},
		{"/var/log", false},
		{"/tmp", false},
		{"/usr/bin", false},
		{"", false},

		// Edge cases
		{"/development", false}, // Not /dev
		{"/system", false},      // Not /sys
		{"/bootstrap", false},   // Not /boot
	}

	for _, tc := range testCases {
		result := shouldSkipPath(tc.path)
		if result != tc.expected {
			t.Errorf("shouldSkipPath(%q): expected %v, got %v", tc.path, tc.expected, result)
		}
	}
}

func TestNewSystem(t *testing.T) {
	s := NewSystem()
	if s == nil {
		t.Fatal("NewSystem returned nil")
	}
	if s.RequestChan == nil {
		t.Error("RequestChan is nil")
	}
	if s.ResponseChan == nil {
		t.Error("ResponseChan is nil")
	}
	if s.ProgressChan == nil {
		t.Error("ProgressChan is nil")
	}
}

func TestFetchDir(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()

	// Create some files and directories
	dirs := []string{"dir1", "dir2", ".hidden_dir"}
	files := []string{"file1.txt", "file2.go", ".hidden_file"}

	for _, d := range dirs {
		if err := os.Mkdir(filepath.Join(tmpDir, d), 0755); err != nil {
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, f), []byte("test content"), 0644); err != nil {
			t.Fatalf("failed to create file %s: %v", f, err)
		}
	}

	// Create nested files (should not be returned by fetchDir)
	nestedFile := filepath.Join(tmpDir, "dir1", "nested.txt")
	if err := os.WriteFile(nestedFile, []byte("nested"), 0644); err != nil {
		t.Fatalf("failed to create nested file: %v", err)
	}

	s := NewSystem()
	resp := s.fetchDir(tmpDir)

	if resp.Err != nil {
		t.Fatalf("fetchDir returned error: %v", resp.Err)
	}

	if resp.Op != FetchDir {
		t.Errorf("expected Op=FetchDir, got %d", resp.Op)
	}

	if resp.Path != tmpDir {
		t.Errorf("expected Path=%q, got %q", tmpDir, resp.Path)
	}

	// Should have 6 entries: 3 dirs + 3 files
	expectedCount := len(dirs) + len(files)
	if len(resp.Entries) != expectedCount {
		t.Errorf("expected %d entries, got %d", expectedCount, len(resp.Entries))
	}

	// Verify entries
	entryMap := make(map[string]Entry)
	for _, e := range resp.Entries {
		entryMap[e.Name] = e
	}

	// Check directories
	for _, d := range dirs {
		e, ok := entryMap[d]
		if !ok {
			t.Errorf("missing directory entry: %s", d)
			continue
		}
		if !e.IsDir {
			t.Errorf("entry %s should be a directory", d)
		}
	}

	// Check files
	for _, f := range files {
		e, ok := entryMap[f]
		if !ok {
			t.Errorf("missing file entry: %s", f)
			continue
		}
		if e.IsDir {
			t.Errorf("entry %s should be a file", f)
		}
	}

	// Verify nested file is NOT included
	if _, ok := entryMap["nested.txt"]; ok {
		t.Error("nested file should not be included in fetchDir results")
	}
}

func TestFetchDir_NonExistent(t *testing.T) {
	s := NewSystem()
	resp := s.fetchDir("/nonexistent/path/that/does/not/exist")

	if resp.Err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestFetchDir_SymlinkHandling(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a regular directory
	realDir := filepath.Join(tmpDir, "realdir")
	if err := os.Mkdir(realDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a regular file
	realFile := filepath.Join(tmpDir, "realfile.txt")
	if err := os.WriteFile(realFile, []byte("real content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create symlinks
	linkToDir := filepath.Join(tmpDir, "linkdir")
	linkToFile := filepath.Join(tmpDir, "linkfile.txt")
	if err := os.Symlink(realDir, linkToDir); err != nil {
		t.Skipf("cannot create symlinks: %v", err)
	}
	if err := os.Symlink(realFile, linkToFile); err != nil {
		t.Fatal(err)
	}

	s := NewSystem()
	resp := s.fetchDir(tmpDir)

	if resp.Err != nil {
		t.Fatalf("fetchDir returned error: %v", resp.Err)
	}

	entryMap := make(map[string]Entry)
	for _, e := range resp.Entries {
		entryMap[e.Name] = e
	}

	// Symlink to directory should appear as a directory (following symlink)
	if e, ok := entryMap["linkdir"]; ok {
		if !e.IsDir {
			t.Error("symlink to directory should appear as directory")
		}
	} else {
		t.Error("missing symlink to directory")
	}

	// Symlink to file should appear as a file
	if e, ok := entryMap["linkfile.txt"]; ok {
		if e.IsDir {
			t.Error("symlink to file should appear as file")
		}
	} else {
		t.Error("missing symlink to file")
	}
}

func TestEntry_Fields(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	content := []byte("hello world")
	if err := os.WriteFile(testFile, content, 0644); err != nil {
		t.Fatal(err)
	}

	s := NewSystem()
	resp := s.fetchDir(tmpDir)

	if len(resp.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(resp.Entries))
	}

	e := resp.Entries[0]

	if e.Name != "test.txt" {
		t.Errorf("expected Name='test.txt', got %q", e.Name)
	}

	if e.Path != testFile {
		t.Errorf("expected Path=%q, got %q", testFile, e.Path)
	}

	if e.IsDir {
		t.Error("expected IsDir=false")
	}

	if e.Size != int64(len(content)) {
		t.Errorf("expected Size=%d, got %d", len(content), e.Size)
	}

	// ModTime should be recent
	if time.Since(e.ModTime) > time.Minute {
		t.Errorf("ModTime seems too old: %v", e.ModTime)
	}
}

func TestSystem_Start_FetchDir(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	s := NewSystem()
	go s.Start()

	// Send a fetch request
	s.RequestChan <- Request{
		Op:   FetchDir,
		Path: tmpDir,
		Gen:  1,
	}

	// Wait for response
	select {
	case resp := <-s.ResponseChan:
		if resp.Err != nil {
			t.Fatalf("unexpected error: %v", resp.Err)
		}
		if resp.Op != FetchDir {
			t.Errorf("expected Op=FetchDir, got %d", resp.Op)
		}
		if resp.Gen != 1 {
			t.Errorf("expected Gen=1, got %d", resp.Gen)
		}
		if len(resp.Entries) != 1 {
			t.Errorf("expected 1 entry, got %d", len(resp.Entries))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for response")
	}
}

func TestSystem_Start_CancelSearch(t *testing.T) {
	s := NewSystem()
	go s.Start()

	// Create a cancellation context
	ctx, cancel := context.WithCancel(context.Background())
	s.cancelMu.Lock()
	s.cancelFunc = cancel
	s.cancelMu.Unlock()

	// Send cancel request
	s.RequestChan <- Request{Op: CancelSearch}

	// Give it a moment
	time.Sleep(50 * time.Millisecond)

	// Verify context was cancelled
	if ctx.Err() == nil {
		t.Error("expected context to be cancelled")
	}
}

func TestProgress_Struct(t *testing.T) {
	p := Progress{
		Gen:     123,
		Current: 50,
		Total:   100,
		Label:   "Processing...",
	}

	if p.Gen != 123 {
		t.Errorf("expected Gen=123, got %d", p.Gen)
	}
	if p.Current != 50 {
		t.Errorf("expected Current=50, got %d", p.Current)
	}
	if p.Total != 100 {
		t.Errorf("expected Total=100, got %d", p.Total)
	}
	if p.Label != "Processing..." {
		t.Errorf("expected Label='Processing...', got %q", p.Label)
	}
}

func TestRequest_Struct(t *testing.T) {
	r := Request{
		Op:           SearchDir,
		Path:         "/home/user",
		Query:        "contents:test",
		Gen:          42,
		SearchEngine: 1,
		EngineCmd:    "/usr/bin/rg",
		DefaultDepth: 5,
	}

	if r.Op != SearchDir {
		t.Errorf("expected Op=SearchDir, got %d", r.Op)
	}
	if r.Path != "/home/user" {
		t.Errorf("expected Path='/home/user', got %q", r.Path)
	}
	if r.Query != "contents:test" {
		t.Errorf("expected Query='contents:test', got %q", r.Query)
	}
	if r.Gen != 42 {
		t.Errorf("expected Gen=42, got %d", r.Gen)
	}
	if r.SearchEngine != 1 {
		t.Errorf("expected SearchEngine=1, got %d", r.SearchEngine)
	}
	if r.EngineCmd != "/usr/bin/rg" {
		t.Errorf("expected EngineCmd='/usr/bin/rg', got %q", r.EngineCmd)
	}
	if r.DefaultDepth != 5 {
		t.Errorf("expected DefaultDepth=5, got %d", r.DefaultDepth)
	}
}

func TestResponse_Cancelled(t *testing.T) {
	r := Response{
		Op:        SearchDir,
		Path:      "/test",
		Cancelled: true,
	}

	if !r.Cancelled {
		t.Error("expected Cancelled=true")
	}
}

func TestOpType_Constants(t *testing.T) {
	// Verify the constants have distinct values
	if FetchDir == SearchDir || SearchDir == CancelSearch || FetchDir == CancelSearch {
		t.Error("OpType constants should have distinct values")
	}
}

// Benchmark for fetchDir
func BenchmarkFetchDir(b *testing.B) {
	tmpDir := b.TempDir()

	// Create 100 files
	for i := 0; i < 100; i++ {
		name := filepath.Join(tmpDir, "file"+string(rune('0'+i%10))+string(rune('0'+i/10%10))+".txt")
		os.WriteFile(name, []byte("content"), 0644)
	}

	s := NewSystem()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.fetchDir(tmpDir)
	}
}

// Benchmark for shouldSkipPath
func BenchmarkShouldSkipPath(b *testing.B) {
	paths := []string{
		"/dev/null",
		"/home/user/file.txt",
		"/proc/1/status",
		"/var/log/syslog",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, p := range paths {
			shouldSkipPath(p)
		}
	}
}
