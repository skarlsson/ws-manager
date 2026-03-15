package cmd

import (
	"fmt"
	"strconv"

	"github.com/skarlsson/workshell/internal/kitty"
	"github.com/skarlsson/workshell/internal/state"
	"github.com/spf13/cobra"
)

var focusIndexCmd = &cobra.Command{
	Use:   "focus-index <n>",
	Short: "Focus the Nth active workspace (1-based)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		idx, err := strconv.Atoi(args[0])
		if err != nil || idx < 1 {
			return fmt.Errorf("invalid index %q, must be a positive number", args[0])
		}

		active, err := state.ListActive()
		if err != nil {
			return fmt.Errorf("listing active workspaces: %w", err)
		}

		var running []state.WorkspaceState
		for _, st := range active {
			if kitty.IsRunning(st.KittyPID) {
				running = append(running, st)
			}
		}

		if idx > len(running) {
			return fmt.Errorf("only %d active workspaces, requested #%d", len(running), idx)
		}

		target := running[idx-1]
		if err := bringToFront(target.Name); err != nil {
			return fmt.Errorf("focusing %q: %w", target.Name, err)
		}

		fmt.Printf("Focused workspace %q (#%d)\n", target.Name, idx)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(focusIndexCmd)
}
