// Package snapshot wraps git for paymint's data-repo commits. Every git
// invocation goes through exec.Command (no shell), with --git-dir and
// --work-tree explicit so a parent .git can never get involved (Red Team F9).
package snapshot

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo represents the data-dir git repository.
type Repo struct {
	dataDir string // canonical absolute path
	gitDir  string // <dataDir>/.git
}

// New returns a Repo handle. The dataDir must already exist; presence of
// `.git` is asserted lazily (operations fail with a clear message if missing).
func New(dataDir string) (*Repo, error) {
	if dataDir == "" || strings.ContainsAny(dataDir, "\n\x00") {
		return nil, errors.New("snapshot: data dir must be non-empty and shell-safe")
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("snapshot: abs: %w", err)
	}
	return &Repo{dataDir: abs, gitDir: filepath.Join(abs, ".git")}, nil
}

// Init runs `git init` on the data dir. No-ops if `.git` already exists.
func (r *Repo) Init() error {
	if _, err := os.Stat(r.gitDir); err == nil {
		return nil
	}
	cmd := r.git("init", "--initial-branch=main", r.dataDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// IsRepo reports whether the data dir has a .git directory.
func (r *Repo) IsRepo() bool {
	fi, err := os.Stat(r.gitDir)
	return err == nil && fi.IsDir()
}

// CleanlinessCheck refuses if any path outside the entity dirs is dirty —
// catches the case where a user committed unrelated work into the data repo
// and the next sync would silently sweep it up.
func (r *Repo) CleanlinessCheck(allowedTopLevel []string) error {
	if !r.IsRepo() {
		return errors.New("snapshot: data dir is not a git repo (run 'paymint init --git-init' or git init manually)")
	}
	cmd := r.git("status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}
	allowed := make(map[string]bool, len(allowedTopLevel))
	for _, p := range allowedTopLevel {
		allowed[p] = true
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		// porcelain format: "XY <path>"
		if len(line) < 4 {
			continue
		}
		path := strings.TrimSpace(line[3:])
		first := topLevel(path)
		if !allowed[first] {
			return fmt.Errorf("data repo has uncommitted changes outside paymint's tracked dirs: %s", path)
		}
	}
	return nil
}

// CommitPaths stages exactly the named paths and creates a commit with the
// given message. Returns nil if there's nothing to commit.
func (r *Repo) CommitPaths(message string, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	if !r.IsRepo() {
		return errors.New("snapshot: data dir is not a git repo")
	}

	// Normalise + de-dupe paths, fail if any escape the data dir.
	staged := make([]string, 0, len(paths))
	seen := make(map[string]bool)
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(r.dataDir, abs)
		if err != nil || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("snapshot: %s escapes data dir", p)
		}
		if !seen[rel] {
			staged = append(staged, rel)
			seen[rel] = true
		}
	}

	addArgs := append([]string{"add", "--"}, staged...)
	if out, err := r.git(addArgs...).CombinedOutput(); err != nil {
		return fmt.Errorf("git add: %w: %s", err, strings.TrimSpace(string(out)))
	}

	// If nothing actually staged (paths matched .gitignore'd files etc), bail.
	if dirty, err := r.indexDirty(); err != nil {
		return err
	} else if !dirty {
		return nil
	}

	// Commit message via stdin (-F -) defeats flag injection in messages.
	cmd := r.git("commit", "-F", "-")
	cmd.Stdin = bytes.NewBufferString(message)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git commit: %w: %s", err, strings.TrimSpace(out.String()))
	}
	return nil
}

// ShortSHA returns HEAD's abbreviated git SHA, or "" if no commits exist.
func (r *Repo) ShortSHA() (string, error) {
	if !r.IsRepo() {
		return "", nil
	}
	cmd := r.git("rev-parse", "--short", "HEAD")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", nil // no commits yet
	}
	return strings.TrimSpace(out.String()), nil
}

// indexDirty reports whether `git diff --cached --quiet` would exit non-zero.
func (r *Repo) indexDirty() (bool, error) {
	cmd := r.git("diff", "--cached", "--quiet")
	err := cmd.Run()
	if err == nil {
		return false, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return true, nil
	}
	return false, fmt.Errorf("git diff --cached: %w", err)
}

// git constructs an exec.Cmd with --git-dir and --work-tree pinned. Defeats
// git's discovery walking up to a parent .git and prevents shell injection
// because args are passed as a slice.
func (r *Repo) git(args ...string) *exec.Cmd {
	all := append([]string{
		"--git-dir=" + r.gitDir,
		"--work-tree=" + r.dataDir,
	}, args...)
	return exec.Command("git", all...) //nolint:gosec // args are constructed from validated inputs
}

func topLevel(p string) string {
	for i := 0; i < len(p); i++ {
		if p[i] == os.PathSeparator || p[i] == '/' {
			return p[:i]
		}
	}
	return p
}
