package cmd

import (
	"fmt"

	"github.com/skarlsson/workshell/internal/config"
	"github.com/skarlsson/workshell/internal/kitty"
	"github.com/skarlsson/workshell/internal/state"
	"github.com/skarlsson/workshell/internal/wm"
	"github.com/spf13/cobra"
)

// detachWorkspace detaches a workspace (hides it but keeps it running).
// Local: minimizes the kitty window. Remote: kills local kitty (zellij persists on server).
func detachWorkspace(ref string) error {
	hostName, wsName := parseWorkspaceRef(ref)

	if hostName == "" {
		if ws, err := config.LoadWorkspace(wsName); err == nil && ws.IsRemote() {
			hostName = ws.Host
		}
	}

	sk := stateKey(hostName, wsName)
	st, err := state.Load(sk)
	if err != nil {
		if hostName != "" {
			// Auto-discovered remote with no local state — already detached
			fmt.Printf("Workspace %s:%s has no local session (already detached)\n", hostName, wsName)
			return nil
		}
		return fmt.Errorf("loading state for %q: %w", ref, err)
	}

	if !st.Active {
		if hostName != "" {
			fmt.Printf("Workspace %q has no local session (already detached)\n", ref)
			return nil
		}
		return fmt.Errorf("workspace %q is not active", ref)
	}

	if st.Detached {
		return fmt.Errorf("workspace %q is already detached", ref)
	}

	if st.Remote {
		return detachRemoteWorkspace(sk, st)
	}

	return detachLocalWorkspace(sk, st)
}

func detachLocalWorkspace(sk string, st state.WorkspaceState) error {
	if !kitty.IsAlive(sk, st.KittyPID) {
		return fmt.Errorf("workspace %q has no running kitty process", sk)
	}

	if err := wm.Default().Minimize(sk); err != nil {
		return fmt.Errorf("minimizing window: %w", err)
	}

	st.Detached = true
	if err := state.Save(st); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	// Clear focused workspace if this was focused
	if state.LoadFocused() == sk {
		state.SaveFocused("")
	}

	return nil
}

func detachRemoteWorkspace(sk string, st state.WorkspaceState) error {
	// Kill local kitty — SSH drops, zellij detaches on server
	if kitty.IsAlive(sk, st.KittyPID) {
		if err := kitty.KillProcess(st.KittyPID); err != nil {
			fmt.Printf("Warning: could not kill kitty process %d: %v\n", st.KittyPID, err)
		}
	}

	st.Detached = true
	st.KittyPID = 0
	if err := state.Save(st); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	// Clear focused workspace if this was focused
	if state.LoadFocused() == sk {
		state.SaveFocused("")
	}

	return nil
}

var detachCmd = &cobra.Command{
	Use:   "detach <workspace>",
	Short: "Detach a workspace (hide/disconnect, keep running)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref := args[0]
		if err := detachWorkspace(ref); err != nil {
			return err
		}
		fmt.Printf("Detached workspace %q\n", ref)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(detachCmd)
}
