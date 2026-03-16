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
		zellijBin := zellij.FindBin()

		// Change to workspace directory
		if err := os.Chdir(ws.Dir); err != nil {
			return fmt.Errorf("changing to workspace dir %s: %w", ws.Dir, err)
		}

		// Ensure ~/.local/bin is in PATH so zellij panes inherit it
		env := ensureLocalBinInPath()

		if zellij.SessionExists(session) {
			if !zellij.CleanupDeadSession(session) {
				// Session is alive — attach to it, preserving running programs
				fmt.Printf("Attaching to existing session %q...\n", session)
				return syscall.Exec(zellijBin, []string{"zellij", "attach", session}, env)
			}
			// Dead session was cleaned up — fall through to create
		}

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

func init() {
	rootCmd.AddCommand(attachCmd)
}
