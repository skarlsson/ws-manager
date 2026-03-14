package cmd

import (
	"fmt"

	"github.com/skarlsson/ws-manager/internal/config"
	"github.com/skarlsson/ws-manager/internal/kitty"
	"github.com/skarlsson/ws-manager/internal/ssh"
	"github.com/skarlsson/ws-manager/internal/state"
	"github.com/skarlsson/ws-manager/internal/zellij"
	"github.com/spf13/cobra"
)

// closeWorkspace closes a workspace by killing kitty + zellij and removing state.
// For remote workspaces, only the local kitty is killed by default (zellij persists on remote).
func closeWorkspace(name string) error {
	st, err := state.Load(name)
	if err != nil {
		return fmt.Errorf("loading state for %q: %w", name, err)
	}
	if !st.Active {
		return fmt.Errorf("workspace %q is not open", name)
	}

	if st.Remote {
		return closeRemoteWorkspace(name, st, false)
	}

	if st.ZellijSession != "" {
		if err := zellij.KillSession(st.ZellijSession); err != nil {
			fmt.Printf("Warning: could not kill zellij session %q: %v\n", st.ZellijSession, err)
		}
	}

	if st.KittyPID > 0 {
		if err := kitty.KillProcess(st.KittyPID); err != nil {
			fmt.Printf("Warning: could not kill kitty process %d: %v\n", st.KittyPID, err)
		}
	}

	if err := state.Remove(name); err != nil {
		fmt.Printf("Warning: could not remove state file: %v\n", err)
	}

	return nil
}

// closeRemoteWorkspace closes a remote workspace.
// Only kills local kitty by default. If kill=true, also kills the remote zellij session.
func closeRemoteWorkspace(name string, st state.WorkspaceState, kill bool) error {
	// Kill local kitty
	if st.KittyPID > 0 {
		if err := kitty.KillProcess(st.KittyPID); err != nil {
			fmt.Printf("Warning: could not kill kitty process %d: %v\n", st.KittyPID, err)
		}
	}

	if kill && st.Host != "" && st.ZellijSession != "" {
		host, err := config.LoadHost(st.Host)
		if err == nil {
			killCmd := fmt.Sprintf("zellij kill-session %s 2>/dev/null; zellij delete-session %s --force 2>/dev/null", st.ZellijSession, st.ZellijSession)
			if _, err := ssh.Run(host.SSH, killCmd); err != nil {
				fmt.Printf("Warning: could not kill remote zellij session: %v\n", err)
			}
		}
	}

	if err := state.Remove(name); err != nil {
		fmt.Printf("Warning: could not remove state file: %v\n", err)
	}

	return nil
}

var closeCmd = &cobra.Command{
	Use:   "close <workspace>",
	Short: "Close a workspace session",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		kill, _ := cmd.Flags().GetBool("kill")

		st, err := state.Load(name)
		if err != nil {
			return fmt.Errorf("loading state for %q: %w", name, err)
		}
		if !st.Active {
			return fmt.Errorf("workspace %q is not open", name)
		}

		if st.Remote && kill {
			if err := closeRemoteWorkspace(name, st, true); err != nil {
				return err
			}
			fmt.Printf("Closed workspace %q (killed remote zellij session)\n", name)
			return nil
		}

		if err := closeWorkspace(name); err != nil {
			return err
		}
		if st.Remote {
			fmt.Printf("Closed workspace %q (remote zellij session preserved)\n", name)
		} else {
			fmt.Printf("Closed workspace %q\n", name)
		}
		return nil
	},
}

func init() {
	closeCmd.Flags().Bool("kill", false, "Also kill the remote zellij session (remote workspaces only)")
	rootCmd.AddCommand(closeCmd)
}
