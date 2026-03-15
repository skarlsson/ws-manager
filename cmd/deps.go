package cmd

import (
	"fmt"

	"github.com/skarlsson/workshell/internal/deps"
	"github.com/spf13/cobra"
)

var depsCmd = &cobra.Command{
	Use:   "deps",
	Short: "Manage workshell dependencies",
}

var depsInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install missing required dependencies",
	RunE: func(cmd *cobra.Command, args []string) error {
		remoteOnly, _ := cmd.Flags().GetBool("remote")

		label := "local"
		if remoteOnly {
			label = "remote"
		}
		fmt.Printf("Installing missing %s dependencies...\n", label)

		installed := deps.InstallMissing(remoteOnly)
		if len(installed) == 0 {
			fmt.Println("All required dependencies already installed.")
		} else {
			for _, name := range installed {
				fmt.Printf("  Installed %s\n", name)
			}
		}
		return nil
	},
}

func init() {
	depsInstallCmd.Flags().Bool("remote", false, "Only install dependencies needed on remote servers")
	depsCmd.AddCommand(depsInstallCmd)
	rootCmd.AddCommand(depsCmd)
}
