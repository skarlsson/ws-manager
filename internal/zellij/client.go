package zellij

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// FindBin resolves the zellij binary path, checking user-local installs first.
func FindBin() string {
	if p, err := exec.LookPath("zellij"); err == nil {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, dir := range []string{
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, ".cargo", "bin"),
	} {
		p := filepath.Join(dir, "zellij")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "zellij"
}

func run(args ...string) (string, error) {
	cmd := exec.Command(FindBin(), args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("zellij %s: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

func ListSessions() ([]string, error) {
	out, err := run("list-sessions", "--short")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

func KillSession(name string) error {
	_, err := run("kill-session", name)
	return err
}

// DeleteSession removes a dead session. Use before creating a new session with the same name.
func DeleteSession(name string) error {
	_, err := run("delete-session", name, "--force")
	return err
}

// CleanupSession kills and deletes a session, ignoring errors (session may not exist).
func CleanupSession(name string) {
	_ = KillSession(name)
	_ = DeleteSession(name)
}

// CleanupDeadSession only removes a session if it's in EXITED state.
// Returns true if the session was cleaned up or didn't exist.
func CleanupDeadSession(name string) bool {
	alive, exists := sessionStatus(name)
	if !exists {
		return true
	}
	if alive {
		return false
	}
	// Session is exited — safe to delete
	_ = DeleteSession(name)
	return true
}

// sessionStatus returns (alive, exists) for a named session.
func sessionStatus(name string) (alive bool, exists bool) {
	out, err := run("list-sessions", "--no-formatting")
	if err != nil {
		return false, false
	}
	for _, line := range strings.Split(out, "\n") {
		// Lines look like: "ws-foo [Created ...] (EXITED - attach to resurrect)"
		// or: "ws-foo [Created ...]" (alive)
		sessionName := strings.Fields(line)
		if len(sessionName) == 0 {
			continue
		}
		if sessionName[0] != name {
			continue
		}
		return !strings.Contains(line, "EXITED"), true
	}
	return false, false
}

func SessionExists(name string) bool {
	sessions, err := ListSessions()
	if err != nil {
		return false
	}
	for _, s := range sessions {
		// zellij list-sessions --short may include metadata after the name
		// e.g. "ws-foo [Created ...] (EXITED)" — match on prefix
		if s == name || strings.HasPrefix(s, name+" ") {
			return true
		}
	}
	return false
}

// LaunchCommand returns the shell command string to create a new zellij session.
func LaunchCommand(session, layoutPath, cwd string) string {
	if layoutPath != "" {
		return fmt.Sprintf("cd %s && zellij -s %s -n %s\n", cwd, session, layoutPath)
	}
	return fmt.Sprintf("cd %s && zellij -s %s\n", cwd, session)
}

// AttachCommand returns the shell command string to attach to an existing zellij session.
func AttachCommand(session, cwd string) string {
	return fmt.Sprintf("cd %s && zellij attach %s\n", cwd, session)
}
