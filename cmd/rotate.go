package cmd

import (
	"fmt"
	"strings"

	"github.com/skarlsson/ws-manager/internal/config"
	"github.com/skarlsson/ws-manager/internal/git"
	"github.com/skarlsson/ws-manager/internal/kitty"
	"github.com/skarlsson/ws-manager/internal/monitor"
	"github.com/skarlsson/ws-manager/internal/ssh"
	"github.com/skarlsson/ws-manager/internal/state"
	"github.com/spf13/cobra"
)

// bringToFront moves a workspace to the work monitor, restoring the previously
// focused workspace to its home position (multi mode) or minimizing others (single mode).
func bringToFront(name string) error {
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}

	winID, err := kitty.PlatformWindowID(name)
	if err != nil {
		return err
	}

	mode := cfg.FocusMode
	if mode == "" {
		mode = "multi"
	}

	prev := state.LoadFocused()

	// Move previous workspace away
	if prev != "" && prev != name {
		prevSt, err := state.Load(prev)
		if err == nil && prevSt.Active && kitty.IsAlive(prev, prevSt.KittyPID) {
			prevWinID, err := kitty.PlatformWindowID(prev)
			if err == nil {
				if mode == "single" {
					monitor.MinimizeWindow(prevWinID)
				} else if prevSt.HomeCaptured {
					monitor.MoveWindow(prevWinID, prevSt.HomeX, prevSt.HomeY)
				}
			}
		}
	}

	if mode == "single" {
		// Minimize remaining active workspaces
		active, _ := state.ListActive()
		for _, other := range active {
			if other.Name == name || other.Name == prev || !kitty.IsAlive(other.Name, other.KittyPID) {
				continue
			}
			otherWinID, err := kitty.PlatformWindowID(other.Name)
			if err == nil {
				monitor.MinimizeWindow(otherWinID)
			}
		}
	}

	// Move target to saved focus position, or fall back to work monitor origin
	var moveX, moveY int
	focusPos := state.LoadFocusPosition()
	if focusPos.Captured {
		moveX, moveY = focusPos.X, focusPos.Y
	} else if cfg.WorkMonitor != "" {
		mon, err := monitor.GetMonitor(cfg.WorkMonitor)
		if err == nil {
			moveX, moveY = mon.X, mon.Y
		}
	}
	monitor.MoveWindow(winID, moveX, moveY)
	state.SaveFocusPosition(moveX, moveY)

	// Calibrate the coordinate offset (getwindowgeometry vs windowmove)
	// so that ws capture can convert read coords to move coords
	if !focusPos.Calibrated {
		dx, dy := monitor.CalibrateOffset(winID, moveX, moveY)
		state.SaveFocusOffset(dx, dy)
	}

	// Activate
	if err := monitor.ActivateWindow(winID); err != nil {
		return err
	}

	// Refresh window title with current branch
	refreshTitle(name)

	state.SaveFocused(name)
	return nil
}

func refreshTitle(name string) {
	st, err := state.Load(name)
	if err != nil {
		return
	}

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
		kitty.SetTitle(name, title)
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
	kitty.SetTitle(name, title)
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

		st, err := state.Load(name)
		if err != nil || !st.Active || !kitty.IsAlive(name, st.KittyPID) {
			state.SaveFocused("")
			return fmt.Errorf("workspace %q is no longer running", name)
		}

		if !st.HomeCaptured {
			fmt.Printf("Workspace %q has no saved home position. Run 'ws capture' first.\n", name)
			return nil
		}

		winID, err := kitty.PlatformWindowID(name)
		if err != nil {
			return err
		}

		if err := monitor.MoveWindow(winID, st.HomeX, st.HomeY); err != nil {
			return fmt.Errorf("moving %q home: %w", name, err)
		}

		state.SaveFocused("")
		fmt.Printf("Sent workspace %q back home\n", name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(rotateCmd)
	rootCmd.AddCommand(focusCmd)
	rootCmd.AddCommand(unfocusCmd)
}
