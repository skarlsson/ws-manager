package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/skarlsson/workshell/internal/config"
	"github.com/skarlsson/workshell/internal/git"
	"github.com/skarlsson/workshell/internal/ssh"
)

type taskDoneMsg struct {
	action string // "switched", "started", "finished"
	task   string
	err    error
}

type taskModel struct {
	wsName    string
	wsDir     string
	current   string   // current task name from config
	branch    string   // current git branch
	tasks     []string // task names (without task/ prefix)
	cursor    int
	nameInput textinput.Model
	creating  bool
	message   string
	done      bool
	cancelled bool
	remote    bool
	hostSSH   string // SSH target for remote workspaces
}

func newTaskModel(e workspaceEntry) taskModel {
	ti := textinput.New()
	ti.Placeholder = "feature-name"
	ti.Width = 40

	m := taskModel{
		wsName:    e.ws.Name,
		wsDir:     e.ws.Dir,
		current:   e.ws.CurrentTask,
		remote:    e.ws.IsRemote(),
		nameInput: ti,
	}

	if m.remote {
		if h, err := config.LoadHost(e.ws.Host); err == nil {
			m.hostSSH = h.SSH
		} else {
			m.message = fmt.Sprintf("Host %q not found", e.ws.Host)
			return m
		}
		m.loadRemoteTasks()
		// Get current task from remote status if available
		if e.remoteStatus != nil {
			m.branch = e.remoteStatus.Branch
			if e.remoteStatus.Task != "" {
				m.current = e.remoteStatus.Task
			}
		}
		return m
	}

	if !git.IsGitRepo(m.wsDir) {
		m.message = "Not a git repository"
		return m
	}

	m.branch, _ = git.CurrentBranch(m.wsDir)

	branches, _ := git.ListBranches(m.wsDir, "task/")
	for _, b := range branches {
		name := strings.TrimPrefix(b, "task/")
		m.tasks = append(m.tasks, name)
	}

	return m
}

func (m *taskModel) loadRemoteTasks() {
	wsCmd := fmt.Sprintf("export PATH=\"$HOME/.local/bin:$PATH\" && ws task list -w %s 2>/dev/null", m.wsName)
	out, err := ssh.Run(m.hostSSH, wsCmd)
	if err != nil {
		return
	}
	m.tasks = nil
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "No task branches found." {
			continue
		}
		// Lines look like "* task/foo" or "  task/foo"
		branch := strings.TrimPrefix(strings.TrimPrefix(line, "* "), "  ")
		branch = strings.TrimSpace(branch)
		name := strings.TrimPrefix(branch, "task/")
		if name != "" {
			m.tasks = append(m.tasks, name)
		}
	}
}

func (m taskModel) Init() tea.Cmd {
	return nil
}

func (m taskModel) Update(msg tea.Msg) (taskModel, tea.Cmd) {
	switch msg := msg.(type) {
	case taskDoneMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Failed: %v", msg.err)
			m.creating = false
			m.refreshTasks()
			return m, nil
		}
		m.done = true
		return m, nil

	case tea.KeyMsg:
		if m.creating {
			return m.handleCreateInput(msg)
		}

		switch msg.String() {
		case "esc":
			m.cancelled = true
			return m, nil

		case "down":
			if m.cursor < len(m.tasks)-1 {
				m.cursor++
				m.message = ""
			}

		case "up":
			if m.cursor > 0 {
				m.cursor--
				m.message = ""
			}

		case "enter":
			if len(m.tasks) == 0 {
				break
			}
			task := m.tasks[m.cursor]
			if task == m.current {
				m.message = fmt.Sprintf("Already on task %q", task)
				break
			}
			if m.remote {
				hostSSH := m.hostSSH
				wsName := m.wsName
				return m, func() tea.Msg {
					return doRemoteTaskSwitch(hostSSH, wsName, task)
				}
			}
			wsName := m.wsName
			wsDir := m.wsDir
			return m, func() tea.Msg {
				return doTaskSwitch(wsName, wsDir, task)
			}

		case "n":
			m.creating = true
			m.nameInput.SetValue("")
			m.nameInput.Focus()
			m.message = ""
			return m, textinput.Blink

		case "d":
			if m.current == "" {
				m.message = "No active task to finish"
				break
			}
			if m.remote {
				hostSSH := m.hostSSH
				wsName := m.wsName
				return m, func() tea.Msg {
					return doRemoteTaskDone(hostSSH, wsName)
				}
			}
			wsName := m.wsName
			wsDir := m.wsDir
			return m, func() tea.Msg {
				return doTaskDone(wsName, wsDir)
			}
		}
	}

	return m, nil
}

func (m taskModel) handleCreateInput(msg tea.KeyMsg) (taskModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.creating = false
		m.message = ""
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.nameInput.Value())
		if name == "" {
			m.message = "Task name is required"
			return m, nil
		}
		m.creating = false
		if m.remote {
			hostSSH := m.hostSSH
			wsName := m.wsName
			return m, func() tea.Msg {
				return doRemoteTaskStart(hostSSH, wsName, name)
			}
		}
		wsName := m.wsName
		wsDir := m.wsDir
		return m, func() tea.Msg {
			return doTaskStart(wsName, wsDir, name)
		}
	}

	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	return m, cmd
}

func (m *taskModel) refreshTasks() {
	if m.remote {
		m.loadRemoteTasks()
		// Re-query branch info via SSH
		branchCmd := fmt.Sprintf("export PATH=\"$HOME/.local/bin:$PATH\" && cd %s && git rev-parse --abbrev-ref HEAD 2>/dev/null", m.wsDir)
		if out, err := ssh.Run(m.hostSSH, branchCmd); err == nil {
			m.branch = strings.TrimSpace(out)
		}
	} else {
		m.tasks = nil
		branches, _ := git.ListBranches(m.wsDir, "task/")
		for _, b := range branches {
			m.tasks = append(m.tasks, strings.TrimPrefix(b, "task/"))
		}
		m.branch, _ = git.CurrentBranch(m.wsDir)
		if ws, err := config.LoadWorkspace(m.wsName); err == nil {
			m.current = ws.CurrentTask
		}
	}
	if m.cursor >= len(m.tasks) && len(m.tasks) > 0 {
		m.cursor = len(m.tasks) - 1
	}
}

func (m taskModel) View() string {
	var b strings.Builder

	title := m.wsName
	if m.remote {
		title += " (remote)"
	}
	b.WriteString(titleStyle.Render(fmt.Sprintf("Tasks: %s", title)))
	b.WriteString("\n\n")

	// Current state
	branchStr := m.branch
	if branchStr == "" {
		branchStr = "-"
	}
	b.WriteString(normalStyle.Render(fmt.Sprintf("  Branch: %s", branchStr)))
	b.WriteString("\n")
	taskStr := m.current
	if taskStr == "" {
		taskStr = "(none)"
	}
	b.WriteString(normalStyle.Render(fmt.Sprintf("  Task:   %s", taskStr)))
	b.WriteString("\n\n")

	if m.creating {
		b.WriteString(normalStyle.Render("  New task name:"))
		b.WriteString("\n  ")
		b.WriteString(m.nameInput.View())
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("  Enter: create  Esc: cancel"))
		b.WriteString("\n")
	} else {
		if len(m.tasks) == 0 {
			b.WriteString(inactiveStyle.Render("  No task branches. Press 'n' to start one."))
			b.WriteString("\n")
		} else {
			b.WriteString(normalStyle.Render("  Task branches:"))
			b.WriteString("\n")
			for i, task := range m.tasks {
				marker := "  "
				if task == m.current {
					marker = "● "
				}
				line := fmt.Sprintf("  %s%s", marker, task)
				if i == m.cursor {
					b.WriteString(selectedStyle.Render(line))
				} else if task == m.current {
					b.WriteString(activeStyle.Render(line))
				} else {
					b.WriteString(normalStyle.Render(line))
				}
				b.WriteString("\n")
			}
		}

		b.WriteString("\n")
		if m.message != "" {
			b.WriteString("  " + warnStyle.Render(m.message) + "\n\n")
		}
		b.WriteString(helpStyle.Render("  ↑/↓: navigate  Enter: switch  n: new task  d: finish current  Esc: back"))
		b.WriteString("\n")
	}

	return b.String()
}

// Remote task operations — delegate to ws CLI on the remote host via SSH

func remoteWsCmd(hostSSH, args string) (string, error) {
	cmd := fmt.Sprintf("export PATH=\"$HOME/.local/bin:$PATH\" && ws %s", args)
	return ssh.Run(hostSSH, cmd)
}

func doRemoteTaskStart(hostSSH, wsName, task string) taskDoneMsg {
	_, err := remoteWsCmd(hostSSH, fmt.Sprintf("task start %s -w %s", task, wsName))
	if err != nil {
		return taskDoneMsg{err: fmt.Errorf("remote task start: %w", err)}
	}
	return taskDoneMsg{action: "started", task: task}
}

func doRemoteTaskSwitch(hostSSH, wsName, task string) taskDoneMsg {
	_, err := remoteWsCmd(hostSSH, fmt.Sprintf("task switch %s -w %s", task, wsName))
	if err != nil {
		return taskDoneMsg{err: fmt.Errorf("remote task switch: %w", err)}
	}
	return taskDoneMsg{action: "switched", task: task}
}

func doRemoteTaskDone(hostSSH, wsName string) taskDoneMsg {
	_, err := remoteWsCmd(hostSSH, fmt.Sprintf("task done -w %s", wsName))
	if err != nil {
		return taskDoneMsg{err: fmt.Errorf("remote task done: %w", err)}
	}
	return taskDoneMsg{action: "finished"}
}

// Local task operations

func doTaskSwitch(wsName, wsDir, task string) taskDoneMsg {
	branch := "task/" + task
	if !git.BranchExists(wsDir, branch) {
		return taskDoneMsg{err: fmt.Errorf("branch %q does not exist", branch)}
	}

	if dirty, _ := git.HasChanges(wsDir); dirty {
		if err := git.StashPush(wsDir, "workshell: switching to "+task); err != nil {
			return taskDoneMsg{err: fmt.Errorf("stash: %w", err)}
		}
	}

	if err := git.Checkout(wsDir, branch); err != nil {
		return taskDoneMsg{err: fmt.Errorf("checkout: %w", err)}
	}

	ws, err := config.LoadWorkspace(wsName)
	if err != nil {
		return taskDoneMsg{err: err}
	}
	ws.CurrentTask = task
	refreshTitle(wsName)
	if err := config.SaveWorkspace(ws); err != nil {
		return taskDoneMsg{err: err}
	}

	return taskDoneMsg{action: "switched", task: task}
}

func doTaskStart(wsName, wsDir, task string) taskDoneMsg {
	branch := "task/" + task

	if dirty, _ := git.HasChanges(wsDir); dirty {
		if err := git.StashPush(wsDir, "workshell: before task "+task); err != nil {
			return taskDoneMsg{err: fmt.Errorf("stash: %w", err)}
		}
	}

	if git.BranchExists(wsDir, branch) {
		if err := git.Checkout(wsDir, branch); err != nil {
			return taskDoneMsg{err: fmt.Errorf("checkout: %w", err)}
		}
	} else {
		if err := git.CreateAndCheckout(wsDir, branch); err != nil {
			return taskDoneMsg{err: fmt.Errorf("create branch: %w", err)}
		}
	}

	ws, err := config.LoadWorkspace(wsName)
	if err != nil {
		return taskDoneMsg{err: err}
	}
	ws.CurrentTask = task
	refreshTitle(wsName)
	if err := config.SaveWorkspace(ws); err != nil {
		return taskDoneMsg{err: err}
	}

	return taskDoneMsg{action: "started", task: task}
}

func doTaskDone(wsName, wsDir string) taskDoneMsg {
	ws, err := config.LoadWorkspace(wsName)
	if err != nil {
		return taskDoneMsg{err: err}
	}

	if ws.CurrentTask == "" {
		return taskDoneMsg{err: fmt.Errorf("no active task")}
	}

	if dirty, _ := git.HasChanges(wsDir); dirty {
		if err := git.StashPush(wsDir, "workshell: finishing task "+ws.CurrentTask); err != nil {
			return taskDoneMsg{err: fmt.Errorf("stash: %w", err)}
		}
	}

	base := ws.DefaultBranch
	if base == "" {
		base = "main"
	}
	if err := git.Checkout(wsDir, base); err != nil {
		return taskDoneMsg{err: fmt.Errorf("checkout %s: %w", base, err)}
	}

	task := ws.CurrentTask
	ws.CurrentTask = ""
	refreshTitle(wsName)
	if err := config.SaveWorkspace(ws); err != nil {
		return taskDoneMsg{err: err}
	}

	return taskDoneMsg{action: "finished", task: task}
}
