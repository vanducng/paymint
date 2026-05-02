package snapshot_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vanducng/paymint/internal/snapshot"
)

func gitInTest(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
}

func setIdentity(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "git config: %s", string(out))
	}
}

func TestInitAndCommit(t *testing.T) {
	gitInTest(t)
	dir := t.TempDir()
	repo, err := snapshot.New(dir)
	require.NoError(t, err)
	require.NoError(t, repo.Init())
	assert.True(t, repo.IsRepo())

	setIdentity(t, dir)

	// Create a tracked file then commit it.
	path := filepath.Join(dir, "companies.yaml")
	require.NoError(t, os.WriteFile(path, []byte("[]\n"), 0o600))

	require.NoError(t, repo.CommitPaths("paymint sync test", []string{path}))
	sha, err := repo.ShortSHA()
	require.NoError(t, err)
	assert.NotEmpty(t, sha)
}

func TestCommitPaths_RejectsEscape(t *testing.T) {
	gitInTest(t)
	dir := t.TempDir()
	repo, err := snapshot.New(dir)
	require.NoError(t, err)
	require.NoError(t, repo.Init())

	bad := filepath.Join(dir, "..", "outside.yaml")
	err = repo.CommitPaths("nope", []string{bad})
	assert.ErrorContains(t, err, "escapes data dir")
}

func TestCleanlinessCheck(t *testing.T) {
	gitInTest(t)
	dir := t.TempDir()
	repo, err := snapshot.New(dir)
	require.NoError(t, err)
	require.NoError(t, repo.Init())
	setIdentity(t, dir)

	// A file under an allowed top-level dir is fine.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "companies.yaml"), []byte("[]"), 0o600))
	assert.NoError(t, repo.CleanlinessCheck([]string{"companies.yaml", "invoices", "payments"}))

	// A stray file outside allowed top-levels triggers the refusal.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stray.txt"), []byte("hi"), 0o600))
	err = repo.CleanlinessCheck([]string{"companies.yaml"})
	assert.ErrorContains(t, err, "uncommitted changes outside")
}
