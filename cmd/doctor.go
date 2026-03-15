package cmd

import (
	"fmt"

	"github.com/skarlsson/workshell/internal/deps"
	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check that all dependencies are installed",
	Run: func(cmd *cobra.Command, args []string) {
		results := deps.CheckAll()
		anyMissing := false

		fmt.Println("Dependency check:")
		fmt.Println()
		for _, t := range results {
			kind := "optional"
			if t.Required {
				kind = "required"
			}
			if t.Found {
				fmt.Printf("  OK  %-10s %s (%s)\n", t.Name, t.Path, t.Note)
			} else {
				mark := "MISS"
				if t.Required {
					mark = "FAIL"
					anyMissing = true
				}
				fmt.Printf("  %s %-10s not found — %s (%s)\n", mark, t.Name, t.Note, kind)
			}
		}

		fmt.Println()
		if anyMissing {
			fmt.Println("Some required tools are missing. Run: bash install_deps.sh")
		} else {
			fmt.Println("All required tools found.")
		}
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
