package cmd

import (
	"fmt"

	"github.com/skarlsson/workshell/internal/kitty"
	"github.com/skarlsson/workshell/internal/monitor"
	"github.com/skarlsson/workshell/internal/state"
	"github.com/spf13/cobra"
)

func captureHome(name string) error {
	st, err := state.Load(name)
	if err != nil {
		return fmt.Errorf("loading state for %q: %w", name, err)
	}
	if !st.Active || !kitty.IsRunning(st.KittyPID) {
		return fmt.Errorf("workspace %q is not running", name)
	}

	winID, err := kitty.PlatformWindowID(name)
	if err != nil {
		return fmt.Errorf("getting window ID for %q: %w", name, err)
	}

	x, y, err := monitor.GetWindowPosition(winID)
	if err != nil {
		return fmt.Errorf("getting position for %q: %w", name, err)
	}

	// Convert from read-space (client area) to move-space (frame) using calibrated offset
	fp := state.LoadFocusPosition()
	if fp.Calibrated {
		x -= fp.OffsetX
		y -= fp.OffsetY
	}

	st.HomeX = x
	st.HomeY = y
	st.HomeCaptured = true
	if err := state.Save(st); err != nil {
		return fmt.Errorf("saving state for %q: %w", name, err)
	}

	fmt.Printf("  %s: home set to (%d, %d)\n", name, x, y)
	return nil
}

var captureCmd = &cobra.Command{
	Use:   "capture [workspace]",
	Short: "Snapshot current window positions as home positions",
	Long:  "Captures the current position of active workspace windows and saves them as home positions. Without arguments, captures all active workspaces.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 1 {
			return captureHome(args[0])
		}

		active, err := state.ListActive()
		if err != nil {
			return fmt.Errorf("listing active workspaces: %w", err)
		}

		if len(active) == 0 {
			fmt.Println("No active workspaces.")
			return nil
		}

		var captured int
		for _, ws := range active {
			if err := captureHome(ws.Name); err != nil {
				fmt.Printf("  %s: skipped (%v)\n", ws.Name, err)
				continue
			}
			captured++
		}
		fmt.Printf("Captured home positions for %d workspace(s).\n", captured)
		return nil
	},
}

var captureFocusCmd = &cobra.Command{
	Use:   "capture-focus",
	Short: "Save the focused workspace's current position as the focus/rotate target",
	Long:  "Drag your focused workspace window where you want it, then run this. All future ws focus/rotate will place windows at this position.",
	RunE: func(cmd *cobra.Command, args []string) error {
		name := state.LoadFocused()
		if name == "" {
			return fmt.Errorf("no workspace currently focused — run 'ws focus <name>' first")
		}

		st, err := state.Load(name)
		if err != nil || !st.Active || !kitty.IsRunning(st.KittyPID) {
			return fmt.Errorf("workspace %q is not running", name)
		}

		winID, err := kitty.PlatformWindowID(name)
		if err != nil {
			return err
		}

		x, y, err := monitor.GetWindowPosition(winID)
		if err != nil {
			return err
		}

		// Convert from read-space to move-space using calibrated offset
		fp := state.LoadFocusPosition()
		if fp.Calibrated {
			x -= fp.OffsetX
			y -= fp.OffsetY
		}

		state.SaveFocusPosition(x, y)
		fmt.Printf("Focus position set to (%d, %d)\n", x, y)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(captureCmd)
	rootCmd.AddCommand(captureFocusCmd)
}
