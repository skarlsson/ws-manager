package cmd

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/skarlsson/ws-manager/internal/config"
	"github.com/skarlsson/ws-manager/internal/git"
	"github.com/skarlsson/ws-manager/internal/kitty"
	"github.com/skarlsson/ws-manager/internal/process"
	"github.com/skarlsson/ws-manager/internal/state"
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
	ws     config.Workspace
	state  state.WorkspaceState
	claude string
}

type confirmAction int

const (
	confirmNone confirmAction = iota
	confirmClose
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
	m.entries = make([]workspaceEntry, len(workspaces))

	var wg sync.WaitGroup
	for i, ws := range workspaces {
		st, _ := state.Load(ws.Name)
		// Clean up stale state: marked active but process is dead
		if st.Active && !kitty.IsRunning(st.KittyPID) {
			st.Active = false
			st.KittyPID = 0
			_ = state.Save(st)
		}
		m.entries[i] = workspaceEntry{ws: ws, state: st, claude: "-"}
		if st.Active {
			wg.Add(1)
			go func(idx int, session string) {
				defer wg.Done()
				m.entries[idx].claude = process.GetClaudeInfo(session).Pretty()
			}(i, st.ZellijSession)
		}
	}
	wg.Wait()

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
			m.setMsg(successStyle, "Created workspace %s", sub.wsName)
			return m, nil
		}
		m.creating = &sub
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
		case key.Matches(msg, key.NewBinding(key.WithKeys("q", "esc", "ctrl+c"))):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(msg, key.NewBinding(key.WithKeys("j", "down"))):
			if m.cursor < len(m.entries)-1 {
				m.cursor++
				m.message = ""
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("k", "up"))):
			if m.cursor > 0 {
				m.cursor--
				m.message = ""
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
			m.refresh()
			m.setMsg(successStyle, "Refreshed")

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

		case key.Matches(msg, key.NewBinding(key.WithKeys("enter", "o"))):
			e, ok := m.selected()
			if !ok {
				break
			}
			if e.state.Active && kitty.IsRunning(e.state.KittyPID) {
				m.setMsg(warnStyle, "%s is already active", e.ws.Name)
			} else {
				name := e.ws.Name
				m.setMsg(normalStyle, "Opening %s...", name)
				return m, func() tea.Msg {
					err := openWorkspace(name)
					return openDoneMsg{name: name, err: err}
				}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
			e, ok := m.selected()
			if !ok {
				break
			}
			if !e.state.Active || !kitty.IsRunning(e.state.KittyPID) {
				m.setMsg(warnStyle, "%s is not active", e.ws.Name)
			} else {
				m.confirming = confirmClose
				m.setMsg(warnStyle, "Close %s? (y/n)", e.ws.Name)
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("f"))):
			e, ok := m.selected()
			if !ok {
				break
			}
			if !e.state.Active || !kitty.IsRunning(e.state.KittyPID) {
				m.setMsg(warnStyle, "%s is not active", e.ws.Name)
			} else {
				if err := bringToFront(e.ws.Name); err != nil {
					m.setMsg(errorStyle, "Focus failed: %v", err)
				} else {
					m.setMsg(successStyle, "Focused %s", e.ws.Name)
				}
			}

		case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
			e, ok := m.selected()
			if !ok {
				break
			}
			if e.state.Active && kitty.IsRunning(e.state.KittyPID) {
				m.setMsg(warnStyle, "Close %s first before deleting", e.ws.Name)
			} else {
				m.confirming = confirmDelete
				m.setMsg(warnStyle, "Delete workspace %s? This removes the config. (y/n)", e.ws.Name)
			}
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
		case confirmClose:
			if err := closeWorkspace(e.ws.Name); err != nil {
				m.setMsg(errorStyle, "Close failed: %v", err)
			} else {
				m.refresh()
				m.setMsg(successStyle, "Closed %s", e.ws.Name)
			}
		case confirmDelete:
			if err := config.DeleteWorkspace(e.ws.Name); err != nil {
				m.setMsg(errorStyle, "Delete failed: %v", err)
			} else {
				m.refresh()
				m.setMsg(successStyle, "Deleted %s", e.ws.Name)
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

	var b strings.Builder

	b.WriteString(titleStyle.Render("ws-manager dashboard"))
	b.WriteString("\n\n")

	if len(m.entries) == 0 {
		b.WriteString(normalStyle.Render("  No workspaces configured. Use 'ws new' to create one."))
		b.WriteString("\n")
	} else {
		// Build row data first to compute column widths
		type row struct {
			name, dir, branch, task, status, claude string
			isActive                                bool
		}
		rows := make([]row, len(m.entries))
		for i, e := range m.entries {
			r := row{}
			r.name = e.ws.Name
			r.dir = e.ws.Dir
			home, _ := os.UserHomeDir()
			if home != "" && strings.HasPrefix(r.dir, home) {
				r.dir = "~" + r.dir[len(home):]
			}
			r.branch = "-"
			if br, err := git.CurrentBranch(e.ws.Dir); err == nil {
				r.branch = br
			}
			r.task = e.ws.CurrentTask
			if r.task == "" {
				r.task = "-"
			}
			r.isActive = e.state.Active && kitty.IsRunning(e.state.KittyPID)
			if r.isActive {
				r.status = "active"
			} else {
				r.status = "inactive"
			}
			r.claude = e.claude
			rows[i] = r
		}

		// Compute column widths (min from headers, max from data)
		colW := [6]int{4, 3, 6, 4, 6, 6} // min widths from header labels
		for _, r := range rows {
			vals := [6]int{len(r.name), len(r.dir), len(r.branch), len(r.task), len(r.status), len(r.claude)}
			for j, v := range vals {
				if v > colW[j] {
					colW[j] = v
				}
			}
		}
		// Cap dir and branch columns if terminal is narrow
		maxTotal := m.width - 4 // leave margin
		if maxTotal < 80 {
			maxTotal = 80
		}
		// Cap individual columns
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

	b.WriteString("\n")
	if m.message != "" {
		b.WriteString("  " + m.msgStyle.Render(m.message) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(helpStyle.Render("  j/k: navigate  o/Enter: open  n: new  c: close  f: focus  d: delete  r: refresh  q: quit"))
	b.WriteString("\n")

	return b.String()
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
