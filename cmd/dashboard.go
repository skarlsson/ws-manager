package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/skarlsson/ws-manager/internal/config"
	"github.com/skarlsson/ws-manager/internal/deps"
	"github.com/skarlsson/ws-manager/internal/git"
	"github.com/skarlsson/ws-manager/internal/kitty"
	"github.com/skarlsson/ws-manager/internal/process"
	"github.com/skarlsson/ws-manager/internal/ssh"
	"github.com/skarlsson/ws-manager/internal/state"
	"github.com/skarlsson/ws-manager/internal/zellij"
	"github.com/spf13/cobra"
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Interactive TUI dashboard for managing workspaces",
	RunE: func(cmd *cobra.Command, args []string) error {
		p := tea.NewProgram(newDashboardModel(), tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}

func init() {
	rootCmd.AddCommand(dashboardCmd)
}

// Styles
var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Padding(0, 1)
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("229")).Background(lipgloss.Color("57"))
	normalStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	activeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	inactiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("75")).Padding(0, 1)
	warnStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
)

type workspaceEntry struct {
	ws           config.Workspace
	state        state.WorkspaceState
	claude       string
	remoteStatus *ssh.RemoteStatus // populated for remote workspaces
}

// ref returns "host:name" for remote entries or just "name" for local.
func (e workspaceEntry) ref() string {
	if e.ws.Host != "" {
		return e.ws.Host + ":" + e.ws.Name
	}
	return e.ws.Name
}

// sk returns the state file key: "host@name" for remote, "name" for local.
func (e workspaceEntry) sk() string {
	if e.ws.Host != "" {
		return e.ws.Host + "@" + e.ws.Name
	}
	return e.ws.Name
}

type confirmAction int

const (
	confirmNone confirmAction = iota
	confirmKill
	confirmDelete
)

type dashboardModel struct {
	entries    []workspaceEntry
	cursor     int
	width      int
	height     int
	message    string
	msgStyle   lipgloss.Style
	quitting   bool
	confirming confirmAction
	creating   *newWSModel
	tasking    *taskModel
	settings   *settingsModel
}

// Messages for async operations
type openDoneMsg struct{ name string; err error }

func newDashboardModel() dashboardModel {
	m := dashboardModel{msgStyle: normalStyle}
	m.refresh()
	return m
}

func (m *dashboardModel) refresh() {
	workspaces, _ := config.ListWorkspaces()

	// Fetch remote statuses per host (async, deduplicated)
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

	m.entries = make([]workspaceEntry, 0, len(workspaces))
	seenRemote := make(map[string]bool)

	// Collect claude lookups to run after all appends (avoids slice realloc race)
	type claudeLookup struct {
		idx     int
		session string
	}
	var claudeLookups []claudeLookup

	for _, ws := range workspaces {
		// Use stateKey for remote workspaces
		sk := ws.Name
		if ws.IsRemote() {
			sk = ws.Host + "@" + ws.Name
		}
		st, _ := state.Load(sk)

		// Detect dead kitty — clean up stale state for both local and remote
		if st.KittyPID > 0 && !kitty.IsAlive(sk, st.KittyPID) {
			st.KittyPID = 0
			if !st.Remote {
				st.Active = false
				st.Detached = false
			}
			_ = state.Save(st)
		}

		entry := workspaceEntry{ws: ws, state: st, claude: "-"}

		if ws.IsRemote() {
			seenRemote[ws.Host+":"+ws.Name] = true
			if rs, ok := remoteMap[ws.Host+":"+ws.Name]; ok {
				entry.remoteStatus = rs
				if rs.Active {
					entry.state.Active = true
					if kitty.IsAlive(sk, st.KittyPID) {
						entry.state.Detached = false
						entry.claude = "connected"
					} else {
						entry.state.Detached = true
						entry.claude = "-"
					}
				} else {
					// Remote session no longer running
					entry.state.Active = false
					entry.state.Detached = false
				}
			} else {
				// Remote unreachable or not in map — use kitty as ground truth
				if !kitty.IsAlive(sk, st.KittyPID) {
					entry.state.Active = false
					entry.state.Detached = false
				}
			}
		} else {
			session := zellij.SessionName(ws.Name)
			if zellij.SessionExists(session) {
				entry.state.Active = true
				claudeLookups = append(claudeLookups, claudeLookup{idx: len(m.entries), session: session})
			}
		}
		m.entries = append(m.entries, entry)
	}

	// Auto-discover remote workspaces not locally configured
	for i := range remoteResults {
		hr := remoteResults[i]
		for j := range hr.statuses {
			rs := &hr.statuses[j]
			key := hr.hostName + ":" + rs.Name
			if seenRemote[key] {
				continue
			}
			ws := config.Workspace{
				Name: rs.Name,
				Dir:  rs.Dir,
				Host: hr.hostName,
			}
			sk := hr.hostName + "@" + rs.Name
			st, _ := state.Load(sk)
			entry := workspaceEntry{
				ws:           ws,
				state:        st,
				claude:       "-",
				remoteStatus: rs,
			}
			if !entry.state.Remote {
				entry.state.Remote = true
				entry.state.Host = hr.hostName
			}
			if rs.Active {
				entry.state.Active = true
				if kitty.IsAlive(sk, st.KittyPID) {
					entry.state.Detached = false
				} else {
					entry.state.Detached = true
				}
			} else {
				entry.state.Active = false
				entry.state.Detached = false
			}
			m.entries = append(m.entries, entry)
		}
	}

	// Run claude lookups in parallel — safe now that slice is fully built
	var wg2 sync.WaitGroup
	for _, cl := range claudeLookups {
		wg2.Add(1)
		go func(i int, s string) {
			defer wg2.Done()
			m.entries[i].claude = process.GetClaudeInfo(s).Pretty()
		}(cl.idx, cl.session)
	}
	wg2.Wait()

	if m.cursor >= len(m.entries) && len(m.entries) > 0 {
		m.cursor = len(m.entries) - 1
	}
}

func (m *dashboardModel) setMsg(style lipgloss.Style, format string, args ...any) {
	m.message = fmt.Sprintf(format, args...)
	m.msgStyle = style
}

func (m dashboardModel) selected() (workspaceEntry, bool) {
	if len(m.entries) == 0 {
		return workspaceEntry{}, false
	}
	return m.entries[m.cursor], true
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m dashboardModel) Init() tea.Cmd {
	return tickCmd()
}

func (m dashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Delegate to new-workspace flow when active
	if m.creating != nil {
		sub := *m.creating
		var cmd tea.Cmd
		sub, cmd = sub.Update(msg)
		if sub.cancelled {
			m.creating = nil
			m.setMsg(normalStyle, "Cancelled")
			return m, nil
		}
		if sub.done {
			m.creating = nil
			m.refresh()
			wsRef := sub.wsName
			if sub.hostName != "" {
				wsRef = sub.hostName + ":" + sub.wsName
			}
			m.setMsg(successStyle, "Created workspace %s", wsRef)
			return m, nil
		}
		m.creating = &sub
		return m, cmd
	}

	// Delegate to task flow when active
	if m.tasking != nil {
		sub := *m.tasking
		var cmd tea.Cmd
		sub, cmd = sub.Update(msg)
		if sub.cancelled {
			m.tasking = nil
			m.setMsg(normalStyle, "")
			return m, nil
		}
		if sub.done {
			m.tasking = nil
			m.refresh()
			m.setMsg(successStyle, "Done")
			return m, nil
		}
		m.tasking = &sub
		return m, cmd
	}

	// Delegate to settings flow when active
	if m.settings != nil {
		sub := *m.settings
		var cmd tea.Cmd
		sub, cmd = sub.Update(msg)
		if sub.cancelled {
			m.settings = nil
			m.setMsg(normalStyle, "")
			return m, nil
		}
		if sub.done {
			m.settings = nil
			m.refresh()
			m.setMsg(successStyle, "Settings saved")
			return m, nil
		}
		m.settings = &sub
		return m, cmd
	}

	switch msg := msg.(type) {
	case tickMsg:
		m.refresh()
		return m, tickCmd()

	case openDoneMsg:
		m.refresh()
		if msg.err != nil {
			m.setMsg(errorStyle, "Open failed: %v", msg.err)
		} else {
			m.setMsg(successStyle, "Opened %s", msg.name)
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Handle confirm prompt
		if m.confirming != confirmNone {
			return m.handleConfirm(msg)
		}

		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("esc", "ctrl+c"))):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("down"))):
			if m.cursor < len(m.entries)-1 {
				m.cursor++
				m.message = ""
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("up"))):
			if m.cursor > 0 {
				m.cursor--
				m.message = ""
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("n"))):
			cfg, _ := config.LoadGlobalConfig()
			baseDir := cfg.WorkspaceBaseDir
			if baseDir == "" {
				home, _ := os.UserHomeDir()
				baseDir = home + "/dev"
			}
			sub := newNewWSModel(baseDir)
			m.creating = &sub
			return m, sub.Init()

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			e, ok := m.selected()
			if !ok {
				break
			}
			esk := e.sk()
			eref := e.ref()
			if kitty.IsAlive(esk, e.state.KittyPID) {
				// Kitty already running — focus it
				if err := bringToFront(esk); err != nil {
					m.setMsg(errorStyle, "Focus failed: %v", err)
				} else {
					m.setMsg(successStyle, "Focused %s", eref)
				}
			} else if !e.ws.IsRemote() && zellij.SessionExists(zellij.SessionName(e.ws.Name)) && !deps.HasTool("kitty") {
				// No kitty available (e.g. on remote server) — attach directly in this terminal
				return m, tea.ExecProcess(attachExecCmd(e.ws.Name), func(err error) tea.Msg {
					return openDoneMsg{name: e.ws.Name, err: err}
				})
			} else {
				suffix := ""
				if e.state.Detached {
					suffix = " (reattaching)"
				}
				m.setMsg(normalStyle, "Opening %s%s...", eref, suffix)
				return m, func() tea.Msg {
					err := openWorkspace(eref)
					return openDoneMsg{name: eref, err: err}
				}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			e, ok := m.selected()
			if !ok {
				break
			}
			if !e.state.Active || e.state.Detached {
				m.setMsg(warnStyle, "%s is not active", e.ref())
			} else {
				if err := detachWorkspace(e.ref()); err != nil {
					m.setMsg(errorStyle, "Detach failed: %v", err)
				} else {
					m.refresh()
					m.setMsg(successStyle, "Detached %s", e.ref())
				}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("x"))):
			e, ok := m.selected()
			if !ok {
				break
			}
			isActive := e.state.Active || zellij.SessionExists(zellij.SessionName(e.ws.Name))
			if !isActive {
				m.setMsg(warnStyle, "%s is not active", e.ref())
			} else {
				m.confirming = confirmKill
				m.setMsg(warnStyle, "Kill %s? (y/n)", e.ref())
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("D"))):
			e, ok := m.selected()
			if !ok {
				break
			}
			if e.state.Active {
				m.setMsg(warnStyle, "Kill %s first before deleting", e.ref())
			} else {
				m.confirming = confirmDelete
				m.setMsg(warnStyle, "Delete workspace %s? This removes the config. (y/n)", e.ref())
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
			sub := newSettingsModel()
			m.settings = &sub
			return m, sub.Init()

		case key.Matches(msg, key.NewBinding(key.WithKeys("t"))):
			e, ok := m.selected()
			if !ok {
				break
			}
			sub := newTaskModel(e)
			m.tasking = &sub
			return m, sub.Init()
		}
	}
	return m, nil
}

func (m dashboardModel) handleConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	action := m.confirming
	m.confirming = confirmNone

	if key.Matches(msg, key.NewBinding(key.WithKeys("y", "Y"))) {
		e, ok := m.selected()
		if !ok {
			return m, nil
		}
		switch action {
		case confirmKill:
			if err := killWorkspace(e.ref()); err != nil {
				m.setMsg(errorStyle, "Kill failed: %v", err)
			} else {
				m.refresh()
				m.setMsg(successStyle, "Killed %s", e.ref())
			}
		case confirmDelete:
			if err := deleteWorkspace(e); err != nil {
				m.setMsg(errorStyle, "Delete failed: %v", err)
			} else {
				m.refresh()
				m.setMsg(successStyle, "Deleted %s", e.ref())
			}
		}
	} else {
		m.setMsg(normalStyle, "Cancelled")
	}
	return m, nil
}

func (m dashboardModel) View() string {
	if m.quitting {
		return ""
	}

	if m.creating != nil {
		return m.creating.View()
	}

	if m.tasking != nil {
		return m.tasking.View()
	}

	if m.settings != nil {
		return m.settings.View()
	}

	var b strings.Builder

	b.WriteString(titleStyle.Render("ws-manager dashboard"))
	b.WriteString("\n\n")

	if len(m.entries) == 0 {
		b.WriteString(normalStyle.Render("  No workspaces configured. Use 'ws new' to create one."))
		b.WriteString("\n")
	} else {
		// Build row data first to compute column widths
		type row struct {
			name, dir, branch, task, host, status, claude string
			isActive, isDetached                          bool
		}
		rows := make([]row, len(m.entries))
		hasRemote := false
		for i, e := range m.entries {
			r := row{}
			r.name = e.ws.Name
			r.dir = e.ws.Dir
			home, _ := os.UserHomeDir()
			if home != "" && strings.HasPrefix(r.dir, home) {
				r.dir = "~" + r.dir[len(home):]
			}
			r.branch = "-"
			if e.remoteStatus != nil && e.remoteStatus.Branch != "" {
				r.branch = e.remoteStatus.Branch
			} else if !e.ws.IsRemote() {
				if br, err := git.CurrentBranch(e.ws.Dir); err == nil {
					r.branch = br
				}
			}
			r.task = e.ws.CurrentTask
			if r.task == "" {
				r.task = "-"
			}
			r.host = e.ws.Host
			if r.host != "" {
				hasRemote = true
			}
			r.isActive = e.state.Active && !e.state.Detached
			r.isDetached = e.state.Active && e.state.Detached
			if r.isActive {
				r.status = "active"
			} else if r.isDetached {
				r.status = "detached"
			} else {
				r.status = "inactive"
			}
			if r.isDetached {
				r.claude = "-"
			} else {
				r.claude = e.claude
			}
			rows[i] = r
		}

		// Compute column widths
		numCols := 6
		if hasRemote {
			numCols = 7
		}
		colW := make([]int, numCols)
		// Min widths from headers
		if hasRemote {
			colW = []int{4, 3, 6, 4, 4, 8, 6} // NAME, DIR, BRANCH, TASK, HOST, STATUS, CLAUDE
		} else {
			colW = []int{4, 3, 6, 4, 6, 6} // NAME, DIR, BRANCH, TASK, STATUS, CLAUDE
		}
		for _, r := range rows {
			var vals []int
			if hasRemote {
				host := r.host
				if host == "" {
					host = "local"
				}
				vals = []int{len(r.name), len(r.dir), len(r.branch), len(r.task), len(host), len(r.status), len(r.claude)}
			} else {
				vals = []int{len(r.name), len(r.dir), len(r.branch), len(r.task), len(r.status), len(r.claude)}
			}
			for j, v := range vals {
				if v > colW[j] {
					colW[j] = v
				}
			}
		}
		// Cap dir and branch columns
		if colW[1] > 35 {
			colW[1] = 35
		}
		if colW[2] > 25 {
			colW[2] = 25
		}

		truncate := func(s string, max int) string {
			if len(s) <= max {
				return s
			}
			if max <= 3 {
				return s[:max]
			}
			return s[:max-3] + "..."
		}

		detachedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

		if hasRemote {
			fmtStr := fmt.Sprintf("  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%s", colW[0], colW[1], colW[2], colW[3], colW[4], colW[5])
			b.WriteString(headerStyle.Render(fmt.Sprintf(fmtStr, "NAME", "DIR", "BRANCH", "TASK", "HOST", "STATUS", "CLAUDE")))
			b.WriteString("\n")

			for i, r := range rows {
				host := r.host
				if host == "" {
					host = "local"
				}
				var statusStr string
				if r.isActive {
					statusStr = activeStyle.Render(pad(r.status, colW[5]))
				} else if r.isDetached {
					statusStr = detachedStyle.Render(pad(r.status, colW[5]))
				} else {
					statusStr = inactiveStyle.Render(pad(r.status, colW[5]))
				}

				line := fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s  %-*s  %s  %s",
					colW[0], truncate(r.name, colW[0]),
					colW[1], truncate(r.dir, colW[1]),
					colW[2], truncate(r.branch, colW[2]),
					colW[3], truncate(r.task, colW[3]),
					colW[4], truncate(host, colW[4]),
					statusStr,
					r.claude,
				)
				if i == m.cursor {
					b.WriteString(selectedStyle.Render(line))
				} else {
					b.WriteString(normalStyle.Render(line))
				}
				b.WriteString("\n")
			}
		} else {
			fmtStr := fmt.Sprintf("  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%-%ds  %%s", colW[0], colW[1], colW[2], colW[3], colW[4])
			b.WriteString(headerStyle.Render(fmt.Sprintf(fmtStr, "NAME", "DIR", "BRANCH", "TASK", "STATUS", "CLAUDE")))
			b.WriteString("\n")

			for i, r := range rows {
				statusStr := r.status
				if r.isActive {
					statusStr = activeStyle.Render(pad(r.status, colW[4]))
				} else {
					statusStr = inactiveStyle.Render(pad(r.status, colW[4]))
				}

				line := fmt.Sprintf("  %-*s  %-*s  %-*s  %-*s  %s  %s",
					colW[0], truncate(r.name, colW[0]),
					colW[1], truncate(r.dir, colW[1]),
					colW[2], truncate(r.branch, colW[2]),
					colW[3], truncate(r.task, colW[3]),
					statusStr,
					r.claude,
				)
				if i == m.cursor {
					b.WriteString(selectedStyle.Render(line))
				} else {
					b.WriteString(normalStyle.Render(line))
				}
				b.WriteString("\n")
			}
		}
	}

	b.WriteString("\n")
	if m.message != "" {
		b.WriteString("  " + m.msgStyle.Render(m.message) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("  ↑/↓: navigate  Enter: open  n: new  t: tasks  d: detach  x: kill  D: delete  s: settings  Esc: quit"))
	b.WriteString("\n")

	return b.String()
}

// deleteWorkspace deletes a workspace config. For remote workspaces, delegates to remote ws.
func deleteWorkspace(e workspaceEntry) error {
	if e.ws.IsRemote() {
		host, err := config.LoadHost(e.ws.Host)
		if err != nil {
			return err
		}
		delCmd := fmt.Sprintf("export PATH=\"$HOME/.local/bin:$PATH\" && rm -f ~/.config/ws-manager/workspaces/%s.yaml", e.ws.Name)
		if _, err := ssh.Run(host.SSH, delCmd); err != nil {
			return fmt.Errorf("remote delete: %w", err)
		}
		// Also remove local state if any
		_ = state.Remove(e.sk())
		return nil
	}
	// Local: remove config and state
	if err := config.DeleteWorkspace(e.ws.Name); err != nil {
		return err
	}
	_ = state.Remove(e.ws.Name)
	return nil
}

// attachExecCmd returns an exec.Cmd for "ws attach <name>" to hand off to tea.ExecProcess.
func attachExecCmd(name string) *exec.Cmd {
	self, _ := os.Executable()
	return exec.Command(self, "attach", name)
}

func pad(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// RunDashboardIfNoArgs returns true if dashboard should be shown (no args)
func RunDashboardIfNoArgs() bool {
	if len(os.Args) <= 1 {
		return true
	}
	return false
}
