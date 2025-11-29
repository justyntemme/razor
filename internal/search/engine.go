package search

import (
	"bufio"
	"context"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
func SearchWithEngine(ctx context.Context, engine SearchEngine, engineCmd, pattern, basePath string, maxDepth int) ([]string, error) {
	switch engine {
	case EngineRipgrep:
		return searchWithRipgrep(ctx, engineCmd, pattern, basePath, maxDepth)
	case EngineUgrep:
		return searchWithUgrep(ctx, engineCmd, pattern, basePath, maxDepth)
	default:
		return nil, nil // Builtin doesn't use this function
	}
}

func searchWithRipgrep(ctx context.Context, cmd, pattern, basePath string, maxDepth int) ([]string, error) {
	args := []string{
		"--files-with-matches", // Only output file names
		"--no-heading",
		"--ignore-case",
		"--max-filesize", "10M", // Skip files larger than 10MB
	}

	if maxDepth > 0 {
		args = append(args, "--max-depth", itoa(maxDepth))
	}

	args = append(args, pattern, basePath)

	return runSearchCommand(ctx, cmd, args)
}

func searchWithUgrep(ctx context.Context, cmd, pattern, basePath string, maxDepth int) ([]string, error) {
	args := []string{
		"-l",              // Only output file names
		"--ignore-case",
		"--max-file-size=10M",
	}

	if maxDepth > 0 {
		args = append(args, "--max-depth="+itoa(maxDepth))
	}

	args = append(args, pattern, basePath)

	return runSearchCommand(ctx, cmd, args)
}

func runSearchCommand(ctx context.Context, cmd string, args []string) ([]string, error) {
	c := exec.CommandContext(ctx, cmd, args...)
	
	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := c.Start(); err != nil {
		return nil, err
	}

	var results []string
	scanner := bufio.NewScanner(stdout)
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
		}
	}

	// Wait for command to finish (ignore exit code - grep returns 1 for no matches)
	c.Wait()

	return results, scanner.Err()
}

func itoa(i int) string {
	return strconv.Itoa(i)
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
