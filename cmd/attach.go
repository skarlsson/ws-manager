package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/skarlsson/workshell/internal/config"
	"github.com/skarlsson/workshell/internal/zellij"
	"github.com/spf13/cobra"
)

var attachCmd = &cobra.Command{
	Use:   "attach <workspace>",
	Short: "Attach to a workspace's zellij session (runs on remote)",
	Long:  "Generates the layout and attaches or creates the zellij session. Meant to run on the remote server via SSH. Uses exec to replace the process so SSH closes cleanly on detach.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		ws, err := config.LoadWorkspace(name)
		if err != nil {
			return fmt.Errorf("workspace %q not found: %w", name, err)
		}

		layoutPath, err := zellij.GenerateLayout(ws)
		if err != nil {
			return fmt.Errorf("generating layout: %w", err)
		}

		session := zellij.SessionName(name)

		// Find zellij binary
		zellijBin, err := findZellij()
		if err != nil {
			return err
		}

		// Change to workspace directory
		if err := os.Chdir(ws.Dir); err != nil {
			return fmt.Errorf("changing to workspace dir %s: %w", ws.Dir, err)
		}

		// Ensure ~/.local/bin is in PATH so zellij panes inherit it
		env := ensureLocalBinInPath()

		if zellij.SessionExists(session) {
			// Attach to existing session
			fmt.Printf("Attaching to existing session %q...\n", session)
			return syscall.Exec(zellijBin, []string{"zellij", "attach", session}, env)
		}

		// Clean up any dead session with the same name, then create new
		zellij.CleanupSession(session)
		fmt.Printf("Creating session %q with layout...\n", session)
		return syscall.Exec(zellijBin, []string{"zellij", "-s", session, "-n", layoutPath}, env)
	},
}

// ensureLocalBinInPath returns os.Environ() with ~/.local/bin prepended to PATH if missing.
func ensureLocalBinInPath() []string {
	home, _ := os.UserHomeDir()
	localBin := filepath.Join(home, ".local", "bin")

	env := os.Environ()
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			path := e[5:]
			found := false
			for _, dir := range strings.Split(path, ":") {
				if dir == localBin {
					found = true
					break
				}
			}
			if !found {
				env[i] = "PATH=" + localBin + ":" + path
			}
			return env
		}
	}
	// No PATH at all — set one
	return append(env, "PATH="+localBin+":/usr/local/bin:/usr/bin:/bin")
}

func findZellij() (string, error) {
	home, _ := os.UserHomeDir()

	// Check common locations including user-local installs
	paths := []string{
		filepath.Join(home, ".local", "bin", "zellij"),
		filepath.Join(home, ".cargo", "bin", "zellij"),
		"/usr/local/bin/zellij",
		"/usr/bin/zellij",
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// Fall back to PATH lookup
	pathEnv := os.Getenv("PATH")
	if pathEnv != "" {
		for _, dir := range splitPath(pathEnv) {
			candidate := filepath.Join(dir, "zellij")
			if _, err := os.Stat(candidate); err == nil {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("zellij not found — checked ~/.local/bin, ~/.cargo/bin, and PATH")
}

func splitPath(path string) []string {
	if path == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == ':' {
			result = append(result, path[start:i])
			start = i + 1
		}
	}
	result = append(result, path[start:])
	return result
}

func init() {
	rootCmd.AddCommand(attachCmd)
}
