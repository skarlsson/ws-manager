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

		// Fetch remote statuses per host
		hosts, _ := config.LoadHosts()
		type hostResult struct {
			hostName string
			statuses []ssh.RemoteStatus
		}
		remoteResults := make([]hostResult, len(hosts))
		var wg sync.WaitGroup
		for i, h := range hosts {
			wg.Add(1)
			go func(idx int, host config.HostConfig) {
				defer wg.Done()
				statuses, _ := ssh.GetRemoteStatuses(host.SSH)
				remoteResults[idx] = hostResult{hostName: host.Name, statuses: statuses}
			}(i, h)
		}
		wg.Wait()

		// Build map: "host:name" -> RemoteStatus
		remoteMap := make(map[string]*ssh.RemoteStatus)
		for i := range remoteResults {
			for j := range remoteResults[i].statuses {
				rs := &remoteResults[i].statuses[j]
				key := remoteResults[i].hostName + ":" + rs.Name
				remoteMap[key] = rs
			}
		}

		hasRemote := len(hosts) > 0
		seenRemote := make(map[string]bool)
		var entries []listEntry

		type claudeLookup struct {
			idx     int
			session string
		}
		var claudeLookups []claudeLookup

		for _, ws := range workspaces {
			e := listEntry{
				ws:   ws,
				task: ws.CurrentTask,
				host: ws.Host,
			}
			if e.task == "" {
				e.task = "-"
			}

			if ws.IsRemote() {
				seenRemote[ws.Host+":"+ws.Name] = true
				if rs, ok := remoteMap[ws.Host+":"+ws.Name]; ok {
					e.branch = rs.Branch
					if e.branch == "" {
						e.branch = "-"
					}
					sk := ws.Host + "@" + ws.Name
					st, _ := state.Load(sk)
					kittyUp := kitty.IsAlive(sk, st.KittyPID)
					if rs.Active && kittyUp {
						e.status = "active"
					} else if rs.Active {
						e.status = "detached"
					} else {
						e.status = "inactive"
					}
					e.claude = "-"
				} else {
					e.branch = "-"
					e.status = "inactive"
					e.claude = "-"
				}
			} else {
				if branch, err := git.CurrentBranch(ws.Dir); err == nil {
					e.branch = branch
				} else {
					e.branch = "-"
				}
				st, _ := state.Load(ws.Name)
				session := zellij.SessionName(ws.Name)
				if st.Active && st.Detached {
					e.status = "detached"
				} else if zellij.SessionExists(session) {
					e.status = "active"
					claudeLookups = append(claudeLookups, claudeLookup{idx: len(entries), session: session})
				} else {
					e.status = "inactive"
				}
				e.claude = "-"
			}

			entries = append(entries, e)
		}

		// Auto-discover remote workspaces
		for i := range remoteResults {
			hr := remoteResults[i]
			for j := range hr.statuses {
				rs := &hr.statuses[j]
				key := hr.hostName + ":" + rs.Name
				if seenRemote[key] {
					continue
				}
				e := listEntry{
					ws:     config.Workspace{Name: rs.Name, Dir: rs.Dir, Host: hr.hostName},
					branch: rs.Branch,
					task:   "-",
					host:   hr.hostName,
					claude: "-",
				}
				if e.branch == "" {
					e.branch = "-"
				}
				if rs.Active {
					e.status = "detached"
				} else {
					e.status = "inactive"
				}
				entries = append(entries, e)
			}
		}

		// Run claude lookups in parallel — safe now that slice is fully built
		var wg2 sync.WaitGroup
		for _, cl := range claudeLookups {
			wg2.Add(1)
			go func(i int, s string) {
				defer wg2.Done()
				entries[i].claude = process.GetClaudeInfo(s).Pretty()
			}(cl.idx, cl.session)
		}
		wg2.Wait()

		if len(entries) == 0 {
			fmt.Println("No workspaces configured. Use 'ws new' to create one.")
			return nil
		}

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
