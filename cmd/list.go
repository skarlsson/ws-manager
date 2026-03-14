package cmd

import (
	"fmt"
	"os"
	"sync"
	"text/tabwriter"

	"github.com/skarlsson/ws-manager/internal/config"
	"github.com/skarlsson/ws-manager/internal/git"
	"github.com/skarlsson/ws-manager/internal/kitty"
	"github.com/skarlsson/ws-manager/internal/process"
	"github.com/skarlsson/ws-manager/internal/ssh"
	"github.com/skarlsson/ws-manager/internal/state"
	"github.com/skarlsson/ws-manager/internal/zellij"
	"github.com/spf13/cobra"
)

type listEntry struct {
	ws     config.Workspace
	branch string
	status string
	task   string
	claude string
	host   string
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all workspaces",
	RunE: func(cmd *cobra.Command, args []string) error {
		workspaces, err := config.ListWorkspaces()
		if err != nil {
			return fmt.Errorf("listing workspaces: %w", err)
		}

		if len(workspaces) == 0 {
			fmt.Println("No workspaces configured. Use 'ws new' to create one.")
			return nil
		}

		entries := make([]listEntry, len(workspaces))
		var wg sync.WaitGroup
		hasRemote := false

		for i, ws := range workspaces {
			entries[i].ws = ws
			entries[i].task = ws.CurrentTask
			if entries[i].task == "" {
				entries[i].task = "-"
			}
			entries[i].host = ws.Host
			if ws.Host != "" {
				hasRemote = true
			}

			if !ws.IsRemote() {
				if branch, err := git.CurrentBranch(ws.Dir); err == nil {
					entries[i].branch = branch
				} else {
					entries[i].branch = "-"
				}
			} else {
				entries[i].branch = "-"
			}

			st, _ := state.Load(ws.Name)
			if st.Active && kitty.IsRunning(st.KittyPID) {
				entries[i].status = "active"
				if !ws.IsRemote() {
					wg.Add(1)
					go func(idx int, session string) {
						defer wg.Done()
						entries[idx].claude = process.GetClaudeInfo(session).Pretty()
					}(i, st.ZellijSession)
				} else {
					entries[i].claude = "-"
				}
			} else if ws.IsRemote() {
				// Check for detached remote session
				entries[i].claude = "-"
				wg.Add(1)
				go func(idx int, w config.Workspace) {
					defer wg.Done()
					session := zellij.SessionName(w.Name)
					host, err := config.LoadHost(w.Host)
					if err == nil && ssh.CheckZellijSession(host.SSH, session) {
						entries[idx].status = "detached"
					} else {
						entries[idx].status = "inactive"
					}
				}(i, ws)
			} else {
				entries[i].status = "inactive"
				entries[i].claude = "-"
			}
		}

		wg.Wait()

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		if hasRemote {
			fmt.Fprintln(w, "NAME\tDIR\tBRANCH\tTASK\tHOST\tSTATUS\tCLAUDE")
			fmt.Fprintln(w, "----\t---\t------\t----\t----\t------\t------")
			for _, e := range entries {
				host := e.host
				if host == "" {
					host = "local"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", e.ws.Name, e.ws.Dir, e.branch, e.task, host, e.status, e.claude)
			}
		} else {
			fmt.Fprintln(w, "NAME\tDIR\tBRANCH\tTASK\tSTATUS\tCLAUDE")
			fmt.Fprintln(w, "----\t---\t------\t----\t------\t------")
			for _, e := range entries {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", e.ws.Name, e.ws.Dir, e.branch, e.task, e.status, e.claude)
			}
		}
		w.Flush()
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
