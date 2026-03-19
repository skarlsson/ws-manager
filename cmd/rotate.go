package cmd

import (
	"fmt"
	"strings"

	"github.com/skarlsson/workshell/internal/config"
	"github.com/skarlsson/workshell/internal/git"
	"github.com/skarlsson/workshell/internal/kitty"
	"github.com/skarlsson/workshell/internal/monitor"
	"github.com/skarlsson/workshell/internal/ssh"
	"github.com/skarlsson/workshell/internal/state"
	"github.com/skarlsson/workshell/internal/wm"
	"github.com/spf13/cobra"
)

// bringToFront focuses a workspace window.
//
// Multi-monitor (WorkMonitor configured + >1 monitor):
//   - Capture previous workspace's position as home (before it was moved to work monitor)
//   - Move previous back to its home position (stays visible on its own monitor)
//   - Capture target's current position as home, move it to work monitor, activate
//
// Single monitor:
//   - Minimize previous workspace
//   - Activate target (unminimizes it)
func bringToFront(name string) error {
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	mgr := wm.Default()
	prev := state.LoadFocused()

	// Determine if we're in multi-monitor mode
	var focusX, focusY int
	multiMonitor := false
	monitors, _ := monitor.ListMonitors()
	if len(monitors) > 1 && cfg.WorkMonitor != "" {
		if mon, err := monitor.GetMonitor(cfg.WorkMonitor); err == nil {
			focusX, focusY = mon.X+50, mon.Y+50
			multiMonitor = true
		}
	}

	// 1. Handle previous workspace
	if prev != "" && prev != name {
		prevSt, err := state.Load(prev)
		if err == nil && prevSt.Active && kitty.IsAlive(prev, prevSt.KittyPID) {
			if multiMonitor && prevSt.HomeCaptured {
				mgr.Move(prev, prevSt.HomeX, prevSt.HomeY)
				if prevSt.HomeMaximized {
					mgr.Maximize(prev)
				}
			} else {
				mgr.Minimize(prev)
			}
		}
	}

	// 2. Capture target's current position as home before moving it.
	//    Only capture when switching workspaces — if re-focusing the same one,
	//    it's already on the work monitor so its position isn't "home".
	if multiMonitor {
		if prev != name {
			targetSt, err := state.Load(name)
			if err == nil {
				targetSt.HomeMaximized = mgr.IsMaximized(name)
				if x, y, err := mgr.GetPosition(name); err == nil {
					targetSt.HomeX = x
					targetSt.HomeY = y
					targetSt.HomeCaptured = true
					state.Save(targetSt)
				}
			}
		}
		mgr.Move(name, focusX, focusY)
	}

	// 3. Activate (also unminimizes on single monitor)
	if err := mgr.Activate(name); err != nil {
		return err
	}

	refreshTitle(name)
	state.SaveFocused(name)
	return nil
}

func refreshTitle(name string) {
	st, err := state.Load(name)
	if err != nil {
		return
	}

	mgr := wm.Default()

	if st.Remote {
		// Remote workspace — title includes host, and branch comes from remote
		host, err := config.LoadHost(st.Host)
		if err != nil {
			return
		}
		// Parse workspace name from stateKey (host@wsName)
		wsName := name
		if i := strings.IndexByte(name, '@'); i > 0 {
			wsName = name[i+1:]
		}
		title := fmt.Sprintf("ws: %s [%s]", wsName, st.Host)
		statuses, err := ssh.GetRemoteStatuses(host.SSH)
		if err == nil {
			for _, rs := range statuses {
				if rs.Name == wsName && rs.Branch != "" {
					title = fmt.Sprintf("ws: %s [%s] (%s)", wsName, rs.Branch, st.Host)
					break
				}
			}
		}
		mgr.SetTitle(name, title)
		return
	}

	ws, err := config.LoadWorkspace(name)
	if err != nil {
		return
	}
	title := fmt.Sprintf("ws: %s", name)
	if git.IsGitRepo(ws.Dir) {
		if branch, err := git.CurrentBranch(ws.Dir); err == nil {
			title = fmt.Sprintf("ws: %s [%s]", name, branch)
		}
	}
	mgr.SetTitle(name, title)
}

var rotateCmd = &cobra.Command{
	Use:   "rotate",
	Short: "Cycle to the next active workspace on the work monitor",
	RunE: func(cmd *cobra.Command, args []string) error {
		active, err := state.ListActive()
		if err != nil {
			return fmt.Errorf("listing active workspaces: %w", err)
		}

		var running []state.WorkspaceState
		for _, st := range active {
			if kitty.IsAlive(st.Name, st.KittyPID) && !st.Detached {
				running = append(running, st)
			}
		}

		if len(running) == 0 {
			fmt.Println("No active workspaces to rotate.")
			return nil
		}

		current := state.LoadRotateIndex()
		next := (current + 1) % len(running)
		state.SaveRotateIndex(next)

		target := running[next]
		if err := bringToFront(target.Name); err != nil {
			return fmt.Errorf("focusing %q: %w", target.Name, err)
		}

		fmt.Printf("Rotated to workspace %q (%d/%d)\n", target.Name, next+1, len(running))
		return nil
	},
}

var focusCmd = &cobra.Command{
	Use:   "focus <workspace>",
	Short: "Bring a workspace to the work monitor, restore the previous one",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		st, err := state.Load(name)
		if err != nil || !st.Active || !kitty.IsAlive(name, st.KittyPID) {
			return fmt.Errorf("workspace %q is not running", name)
		}

		if err := bringToFront(name); err != nil {
			return fmt.Errorf("focusing %q: %w", name, err)
		}

		fmt.Printf("Focused workspace %q\n", name)
		return nil
	},
}

var unfocusCmd = &cobra.Command{
	Use:   "unfocus",
	Short: "Send the currently focused workspace back to its home position",
	RunE: func(cmd *cobra.Command, args []string) error {
		name := state.LoadFocused()
		if name == "" {
			fmt.Println("No workspace currently focused.")
			return nil
		}

		cfg, _ := config.LoadGlobalConfig()
		st, err := state.Load(name)
		if err != nil || !st.Active || !kitty.IsAlive(name, st.KittyPID) {
			state.SaveFocused("")
			return fmt.Errorf("workspace %q is no longer running", name)
		}

		mgr := wm.Default()

		// Check if multi-monitor
		multiMonitor := false
		monitors, _ := monitor.ListMonitors()
		if len(monitors) > 1 && cfg.WorkMonitor != "" {
			multiMonitor = true
		}

		if multiMonitor && st.HomeCaptured {
			// Multi-monitor: move back to home position (stays visible)
			mgr.Move(name, st.HomeX, st.HomeY)
			if st.HomeMaximized {
				mgr.Maximize(name)
			}
		} else {
			// Single monitor: minimize
			mgr.Minimize(name)
		}

		state.SaveFocused("")
		fmt.Printf("Unfocused workspace %q\n", name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(rotateCmd)
	rootCmd.AddCommand(focusCmd)
	rootCmd.AddCommand(unfocusCmd)
}
