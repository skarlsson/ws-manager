package ssh

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Run executes a command on the remote host and returns its output.
func Run(target, command string) (string, error) {
	cmd := exec.Command("ssh", target, command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ssh %s: %w\n%s", target, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// RunWithTimeout executes a command with a ConnectTimeout.
func RunWithTimeout(target, command string, timeout time.Duration) (string, error) {
	secs := int(timeout.Seconds())
	if secs < 1 {
		secs = 1
	}
	cmd := exec.Command("ssh", "-o", fmt.Sprintf("ConnectTimeout=%d", secs), target, command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ssh %s: %w\n%s", target, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// InteractiveCommand returns the full SSH command string for sending to kitty via send-text.
func InteractiveCommand(target, command string) string {
	return fmt.Sprintf("ssh %s -t '%s'\n", target, command)
}

// CheckConnection verifies SSH connectivity to a host.
func CheckConnection(target string) error {
	_, err := RunWithTimeout(target, "echo ok", 5*time.Second)
	return err
}

// GetArch returns the remote machine architecture (e.g. "x86_64", "aarch64").
func GetArch(target string) (string, error) {
	return RunWithTimeout(target, "uname -m", 5*time.Second)
}

// CheckZellijSession checks if a named zellij session exists on the remote host.
func CheckZellijSession(target, session string) bool {
	out, err := RunWithTimeout(target, "zellij list-sessions --short 2>/dev/null", 5*time.Second)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == session {
			return true
		}
	}
	return false
}

// CopyFile copies a local file to a remote path via scp.
func CopyFile(target, localPath, remotePath string) error {
	cmd := exec.Command("scp", localPath, fmt.Sprintf("%s:%s", target, remotePath))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("scp to %s: %w\n%s", target, err, strings.TrimSpace(string(out)))
	}
	return nil
}
