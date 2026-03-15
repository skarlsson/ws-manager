package cmd

import (
	"fmt"

	"github.com/skarlsson/workshell/internal/config"
	"github.com/skarlsson/workshell/internal/kitty"
	"github.com/skarlsson/workshell/internal/ssh"
	"github.com/skarlsson/workshell/internal/state"
	"github.com/skarlsson/workshell/internal/zellij"
	"github.com/spf13/cobra"
)

// killWorkspace fully tears down a workspace (kills everything).
// Local: kills kitty + zellij. Remote: kills local kitty + remote zellij.
func killWorkspace(ref string) error {
	hostName, wsName := parseWorkspaceRef(ref)

	if hostName == "" {
		if ws, err := config.LoadWorkspace(wsName); err == nil && ws.IsRemote() {
			hostName = ws.Host
		}
	}

	sk := stateKey(hostName, wsName)
	st, err := state.Load(sk)
	if err != nil && hostName != "" {
		// No local state file — remote workspace with no local kitty.
		// Kill the remote zellij session directly.
		return killRemoteZellij(hostName, wsName)
	}
	if err != nil {
		return fmt.Errorf("loading state for %q: %w", ref, err)
	}
	if !st.Active {
		if hostName != "" {
			// State exists but not active locally — try killing remote zellij
			return killRemoteZellij(hostName, wsName)
		}
		return fmt.Errorf("workspace %q is not open", ref)
	}

	if st.Remote {
		return killRemoteWorkspace(sk, st)
	}

	return killLocalWorkspace(sk, st)
}

// killLocalWorkspace kills a local workspace (active or detached).
func killLocalWorkspace(sk string, st state.WorkspaceState) error {
	if st.ZellijSession != "" {
		if err := zellij.KillSession(st.ZellijSession); err != nil {
			fmt.Printf("Warning: could not kill zellij session %q: %v\n", st.ZellijSession, err)
		}
	}

	if st.KittyPID > 0 && kitty.IsRunning(st.KittyPID) {
		if err := kitty.KillProcess(st.KittyPID); err != nil {
			fmt.Printf("Warning: could not kill kitty process %d: %v\n", st.KittyPID, err)
		}
	}

	// Clear focused workspace if this was focused
	if state.LoadFocused() == sk {
		state.SaveFocused("")
	}

	if err := state.Remove(sk); err != nil {
		fmt.Printf("Warning: could not remove state file: %v\n", err)
	}

	return nil
}

// killRemoteWorkspace kills a remote workspace (active or detached).
// Kills local kitty if running, then kills remote zellij session.
func killRemoteWorkspace(sk string, st state.WorkspaceState) error {
	// Kill local kitty if running
	if st.KittyPID > 0 && kitty.IsRunning(st.KittyPID) {
		if err := kitty.KillProcess(st.KittyPID); err != nil {
			fmt.Printf("Warning: could not kill kitty process %d: %v\n", st.KittyPID, err)
		}
	}

	// Kill remote zellij session
	if st.Host != "" && st.ZellijSession != "" {
		host, err := config.LoadHost(st.Host)
		if err == nil {
			killCmd := fmt.Sprintf(
				"export PATH=\"$HOME/.local/bin:$PATH\" && zellij kill-session %s 2>/dev/null; zellij delete-session %s --force 2>/dev/null",
				st.ZellijSession, st.ZellijSession,
			)
			if _, err := ssh.Run(host.SSH, killCmd); err != nil {
				fmt.Printf("Warning: could not kill remote zellij session: %v\n", err)
			}
		}
	}

	// Clear focused workspace if this was focused
	if state.LoadFocused() == sk {
		state.SaveFocused("")
	}

	if err := state.Remove(sk); err != nil {
		fmt.Printf("Warning: could not remove state file: %v\n", err)
	}

	return nil
}

// killRemoteZellij kills a remote zellij session when there's no local state (auto-discovered workspace).
func killRemoteZellij(hostName, wsName string) error {
	host, err := config.LoadHost(hostName)
	if err != nil {
		return fmt.Errorf("loading host %q: %w", hostName, err)
	}
	session := "ws-" + wsName
	killCmd := fmt.Sprintf(
		"export PATH=\"$HOME/.local/bin:$PATH\" && zellij kill-session %s 2>/dev/null; zellij delete-session %s --force 2>/dev/null",
		session, session,
	)
	if _, err := ssh.Run(host.SSH, killCmd); err != nil {
		return fmt.Errorf("killing remote zellij session: %w", err)
	}
	// Clean up local state file if it exists
	sk := stateKey(hostName, wsName)
	_ = state.Remove(sk)
	return nil
}

var closeCmd = &cobra.Command{
	Use:   "close <workspace>",
	Short: "Close a workspace session (kills everything)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref := args[0]
		if err := killWorkspace(ref); err != nil {
			return err
		}
		fmt.Printf("Closed workspace %q\n", ref)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(closeCmd)
}
