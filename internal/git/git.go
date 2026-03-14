package git

import (
	"fmt"
	"os/exec"
	"path"
	"strings"
)

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

func CurrentBranch(dir string) (string, error) {
	return run(dir, "rev-parse", "--abbrev-ref", "HEAD")
}

func BranchExists(dir, branch string) bool {
	_, err := run(dir, "rev-parse", "--verify", branch)
	return err == nil
}

func CreateAndCheckout(dir, branch string) error {
	_, err := run(dir, "checkout", "-b", branch)
	return err
}

func Checkout(dir, branch string) error {
	_, err := run(dir, "checkout", branch)
	return err
}

func StashPush(dir, message string) error {
	_, err := run(dir, "stash", "push", "-m", message)
	return err
}

func StashPop(dir string) error {
	_, err := run(dir, "stash", "pop")
	return err
}

func HasChanges(dir string) (bool, error) {
	out, err := run(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

func ListBranches(dir, prefix string) ([]string, error) {
	out, err := run(dir, "branch", "--list", prefix+"*")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var branches []string
	for _, line := range strings.Split(out, "\n") {
		branch := strings.TrimSpace(strings.TrimPrefix(line, "*"))
		branch = strings.TrimSpace(branch)
		if branch != "" {
			branches = append(branches, branch)
		}
	}
	return branches, nil
}

func IsGitRepo(dir string) bool {
	_, err := run(dir, "rev-parse", "--git-dir")
	return err == nil
}

// ListRemoteBranches lists branches from a remote URL via git ls-remote.
func ListRemoteBranches(url string) ([]string, error) {
	cmd := exec.Command("git", "ls-remote", "--heads", url)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git ls-remote: %w\n%s", err, string(out))
	}
	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 {
			ref := parts[1]
			branch := strings.TrimPrefix(ref, "refs/heads/")
			branches = append(branches, branch)
		}
	}
	return branches, nil
}

// ListLocalBranches lists all local branches in a git repo.
func ListLocalBranches(dir string) ([]string, error) {
	out, err := run(dir, "branch", "--list")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var branches []string
	for _, line := range strings.Split(out, "\n") {
		branch := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "*"))
		branch = strings.TrimSpace(branch)
		if branch != "" {
			branches = append(branches, branch)
		}
	}
	return branches, nil
}

// RepoName extracts a repository name from a URL.
// e.g. "git@github.com:user/repo.git" → "repo"
func RepoName(url string) string {
	// Handle SSH URLs like git@github.com:user/repo.git
	if i := strings.LastIndex(url, ":"); i >= 0 && !strings.Contains(url, "://") {
		url = url[i+1:]
	}
	// Take last path segment
	name := path.Base(url)
	// Strip .git suffix
	name = strings.TrimSuffix(name, ".git")
	return name
}

// Clone clones a git repository.
func Clone(url, dir string) error {
	cmd := exec.Command("git", "clone", "--recursive", url, dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git clone: %w\n%s", err, string(out))
	}
	return nil
}

// DefaultBranch returns the default branch name (e.g. "main" or "master")
// by checking the remote HEAD. Falls back to "main" if detection fails.
func DefaultBranch(dir string) string {
	out, err := run(dir, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err == nil {
		// output is like "refs/remotes/origin/main"
		parts := strings.Split(out, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	return "main"
}
