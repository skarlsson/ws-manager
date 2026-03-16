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
		session := deps.SessionType()
		fmt.Printf("Session: %s\n\n", session)

		results := deps.CheckAll()
		anyMissing := false

		categories := []struct {
			key   string
			label string
		}{
			{"core", "Core"},
			{"x11", "X11"},
			{"wayland", "Wayland"},
		}

		for _, cat := range categories {
			var items []deps.ToolStatus
			for _, t := range results {
				if t.Category == cat.key {
					items = append(items, t)
				}
			}
			if len(items) == 0 {
				continue
			}

			fmt.Printf("%s dependencies:\n", cat.label)
			for _, t := range items {
				kind := "optional"
				if t.Required {
					kind = "required"
				}
				if t.Found {
					fmt.Printf("  OK  %-18s %s (%s)\n", t.Name, t.Path, t.Note)
				} else {
					mark := "MISS"
					if t.Required {
						mark = "FAIL"
						anyMissing = true
					}
					fmt.Printf("  %s %-18s not found — %s (%s)\n", mark, t.Name, t.Note, kind)
				}
			}
			fmt.Println()
		}

		if anyMissing {
			fmt.Println("Some required tools are missing. Run: ws deps install")
		} else {
			fmt.Println("All required tools found.")
		}
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
