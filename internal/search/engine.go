package search

import (
	"bufio"
	"context"
	"log"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// SearchEngine represents a content search engine
type SearchEngine int

const (
	EngineBuiltin SearchEngine = iota // Go's built-in file reading
	EngineRipgrep                     // ripgrep (rg)
	EngineUgrep                       // ugrep (ug)
)

func (e SearchEngine) String() string {
	switch e {
	case EngineRipgrep:
		return "ripgrep"
	case EngineUgrep:
		return "ugrep"
	default:
		return "builtin"
	}
}

// EngineInfo contains information about a search engine
type EngineInfo struct {
	Engine    SearchEngine
	Name      string // Display name
	Command   string // Command to run
	Available bool   // Whether it's installed
	Version   string // Version string if available
}

// DetectEngines checks which search engines are available on the system
func DetectEngines() []EngineInfo {
	engines := []EngineInfo{
		{
			Engine:    EngineBuiltin,
			Name:      "Built-in",
			Command:   "",
			Available: true,
			Version:   "native Go",
		},
	}

	// Check for ripgrep
	if path, err := exec.LookPath("rg"); err == nil {
		version := getCommandVersion(path, "--version")
		engines = append(engines, EngineInfo{
			Engine:    EngineRipgrep,
			Name:      "ripgrep",
			Command:   path,
			Available: true,
			Version:   version,
		})
	} else {
		engines = append(engines, EngineInfo{
			Engine:    EngineRipgrep,
			Name:      "ripgrep (not installed)",
			Command:   "rg",
			Available: false,
			Version:   "",
		})
	}

	// Check for ugrep
	if path, err := exec.LookPath("ug"); err == nil {
		version := getCommandVersion(path, "--version")
		engines = append(engines, EngineInfo{
			Engine:    EngineUgrep,
			Name:      "ugrep",
			Command:   path,
			Available: true,
			Version:   version,
		})
	} else if path, err := exec.LookPath("ugrep"); err == nil {
		version := getCommandVersion(path, "--version")
		engines = append(engines, EngineInfo{
			Engine:    EngineUgrep,
			Name:      "ugrep",
			Command:   path,
			Available: true,
			Version:   version,
		})
	} else {
		engines = append(engines, EngineInfo{
			Engine:    EngineUgrep,
			Name:      "ugrep (not installed)",
			Command:   "ugrep",
			Available: false,
			Version:   "",
		})
	}

	return engines
}

func getCommandVersion(cmd string, versionFlag string) string {
	out, err := exec.Command(cmd, versionFlag).Output()
	if err != nil {
		return ""
	}
	// Return first line of version output
	lines := strings.Split(string(out), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0])
	}
	return ""
}

// ExternalSearchResult represents a match from an external search engine
type ExternalSearchResult struct {
	Path    string
	Line    int
	Content string
}

// SearchWithEngine performs a content search using the specified engine
// Returns a list of file paths that match the pattern
// progressFn is called periodically with the number of results found so far
func SearchWithEngine(ctx context.Context, engine SearchEngine, engineCmd, pattern, basePath string, maxDepth int, progressFn func(found int)) ([]string, error) {
	switch engine {
	case EngineRipgrep:
		return searchWithRipgrep(ctx, engineCmd, pattern, basePath, maxDepth, progressFn)
	case EngineUgrep:
		return searchWithUgrep(ctx, engineCmd, pattern, basePath, maxDepth, progressFn)
	default:
		return nil, nil // Builtin doesn't use this function
	}
}

func searchWithRipgrep(ctx context.Context, cmd, pattern, basePath string, maxDepth int, progressFn func(found int)) ([]string, error) {
	args := []string{
		"--files-with-matches", // Only output file names
		"--no-heading",
		"--ignore-case",
		"--max-filesize", "10M", // Skip files larger than 10MB
	}

	// ripgrep depth semantics:
	// --max-depth 1 = search only in the specified directory (no subdirs)
	// --max-depth 2 = specified dir + 1 level of subdirs
	// Our maxDepth=1 means current dir only, which maps to rg --max-depth 1
	if maxDepth > 0 {
		args = append(args, "--max-depth", strconv.Itoa(maxDepth))
	}

	args = append(args, "--", pattern, basePath)

	log.Printf("[RIPGREP] Running: %s %v", cmd, args)
	return runSearchCommand(ctx, cmd, args, progressFn)
}

func searchWithUgrep(ctx context.Context, cmd, pattern, basePath string, maxDepth int, progressFn func(found int)) ([]string, error) {
	args := []string{
		"-l",              // Only output file names
		"-i",              // Ignore case
		"--ignore-binary", // Skip binary files
		"-r",              // Recursive (required for directory searching)
	}

	// ugrep depth semantics:
	// --max-depth=1 = search only in the specified directory (no subdirs)
	// --max-depth=2 = specified dir + 1 level of subdirs
	// Our maxDepth=1 means current dir only, which maps to ug --max-depth=1
	if maxDepth > 0 {
		args = append(args, "--max-depth="+strconv.Itoa(maxDepth))
	}

	args = append(args, "--", pattern, basePath)

	log.Printf("[UGREP] Running: %s %v", cmd, args)
	return runSearchCommand(ctx, cmd, args, progressFn)
}

func runSearchCommand(ctx context.Context, cmd string, args []string, progressFn func(found int)) ([]string, error) {
	log.Printf("[EXTERNAL_SEARCH] Running: %s %v", cmd, args)
	
	c := exec.CommandContext(ctx, cmd, args...)
	
	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := c.Start(); err != nil {
		log.Printf("[EXTERNAL_SEARCH] Start error: %v", err)
		return nil, err
	}

	var results []string
	scanner := bufio.NewScanner(stdout)
	lastReport := 0
	
	for scanner.Scan() {
		if ctx.Err() != nil {
			c.Process.Kill()
			return results, ctx.Err()
		}
		path := strings.TrimSpace(scanner.Text())
		if path != "" {
			// Normalize path
			if abs, err := filepath.Abs(path); err == nil {
				results = append(results, abs)
			} else {
				results = append(results, path)
			}
			
			// Report progress every 10 files found
			if progressFn != nil && len(results)-lastReport >= 10 {
				progressFn(len(results))
				lastReport = len(results)
			}
		}
	}

	// Final progress report
	if progressFn != nil && len(results) > lastReport {
		progressFn(len(results))
	}

	// Wait for command to finish and check exit code
	err = c.Wait()
	if err != nil {
		// grep/ripgrep/ugrep return exit code 1 when no matches are found - this is not an error
		// Exit code 2+ typically indicates a real error (syntax, I/O, etc.)
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				exitCode := status.ExitStatus()
				if exitCode == 1 {
					// Exit code 1 = no matches found, not an error
					log.Printf("[EXTERNAL_SEARCH] Complete: no matches found")
					return results, scanner.Err()
				}
				// Exit code 2+ = real error
				log.Printf("[EXTERNAL_SEARCH] Error: exit code %d: %v", exitCode, err)
				return results, err
			}
		}
		log.Printf("[EXTERNAL_SEARCH] Error: %v", err)
		return results, err
	}

	log.Printf("[EXTERNAL_SEARCH] Complete: found %d files", len(results))
	return results, scanner.Err()
}

// MatchesExternalResults checks if a path is in the external search results
func MatchesExternalResults(path string, results map[string]bool) bool {
	if results == nil {
		return true // No external search was done
	}
	// Check normalized path
	absPath, _ := filepath.Abs(path)
	return results[absPath] || results[path]
}

// GetEngineByName returns the engine for a given name
func GetEngineByName(name string) SearchEngine {
	switch strings.ToLower(name) {
	case "ripgrep", "rg":
		return EngineRipgrep
	case "ugrep", "ug":
		return EngineUgrep
	default:
		return EngineBuiltin
	}
}

// GetEngineCommand returns the command for an engine from detected engines
func GetEngineCommand(engine SearchEngine, engines []EngineInfo) string {
	for _, e := range engines {
		if e.Engine == engine && e.Available {
			return e.Command
		}
	}
	return ""
}
