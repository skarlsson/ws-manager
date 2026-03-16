package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/skarlsson/workshell/internal/config"
	"github.com/skarlsson/workshell/internal/git"
	"github.com/skarlsson/workshell/internal/ssh"
)

type newWSStep int

const (
	stepSelectHost newWSStep = iota
	stepSelectType
	stepInputGitURL
	stepInputDir
	stepSelectBranch
	stepInputName
	stepConfirm
	stepCreating
)

type projectType int

const (
	projectGit projectType = iota
	projectExisting
	projectBlank
)

// Async messages
type setupDoneMsg struct {
	err error
}

type branchesMsg struct {
	branches []string
	err      error
}

type newWSModel struct {
	step     newWSStep
	urlInput textinput.Model
	nameInput textinput.Model
	dirInput  textinput.Model

	// Host selection
	hosts      []config.HostConfig
	hostCursor int
	hostName   string // empty = local

	// Resolved values
	projType       projectType
	typeCursor     int
	gitURL         string
	repoDir        string
	branches       []string
	branchCursor   int
	selectedBranch string
	wsName         string

	baseDir   string
	message   string
	done      bool
	cancelled bool
}

var typeOptions = []string{"Git clone", "Existing folder", "New repo"}

func newNewWSModel(baseDir string) newWSModel {
	urlTI := textinput.New()
	urlTI.Placeholder = "git@github.com:user/repo.git"
	urlTI.Width = 60

	nameTI := textinput.New()
	nameTI.Placeholder = "my-workspace"
	nameTI.Width = 40

	dirTI := textinput.New()
	dirTI.Placeholder = baseDir + "/my-project"
	dirTI.Width = 60

	hosts, _ := config.LoadHosts()

	startStep := stepSelectType
	if len(hosts) > 0 {
		startStep = stepSelectHost
	}

	return newWSModel{
		step:      startStep,
		urlInput:  urlTI,
		nameInput: nameTI,
		dirInput:  dirTI,
		hosts:     hosts,
		baseDir:   baseDir,
	}
}

func isGitURL(s string) bool {
	return strings.Contains(s, "://") || strings.HasPrefix(s, "git@")
}

func (m newWSModel) Init() tea.Cmd {
	return nil
}

func (m newWSModel) Update(msg tea.Msg) (newWSModel, tea.Cmd) {
	switch msg := msg.(type) {
	case setupDoneMsg:
		if msg.err != nil {
			m.message = fmt.Sprintf("Failed: %v", msg.err)
			m.step = stepConfirm
			return m, nil
		}
		m.done = true
		return m, nil

	case branchesMsg:
		if msg.err != nil || len(msg.branches) == 0 {
			m.step = stepInputName
			m.nameInput.SetValue(m.defaultName())
			m.nameInput.Focus()
			return m, textinput.Blink
		}
		m.branches = msg.branches
		m.branchCursor = 0
		for i, b := range m.branches {
			if b == "main" || b == "master" {
				m.branchCursor = i
				break
			}
		}
		m.step = stepSelectBranch
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "esc" {
			m.cancelled = true
			return m, nil
		}
		return m.handleKey(msg)
	}

	// Update active text input
	var cmd tea.Cmd
	switch m.step {
	case stepInputGitURL:
		m.urlInput, cmd = m.urlInput.Update(msg)
	case stepInputName:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case stepInputDir:
		m.dirInput, cmd = m.dirInput.Update(msg)
	}
	return m, cmd
}

func (m newWSModel) handleKey(msg tea.KeyMsg) (newWSModel, tea.Cmd) {
	switch m.step {
	case stepSelectHost:
		numOpts := 1 + len(m.hosts)
		switch msg.String() {
		case "down":
			if m.hostCursor < numOpts-1 {
				m.hostCursor++
			}
		case "up":
			if m.hostCursor > 0 {
				m.hostCursor--
			}
		case "enter":
			if m.hostCursor == 0 {
				m.hostName = ""
			} else {
				m.hostName = m.hosts[m.hostCursor-1].Name
			}
			m.step = stepSelectType
			m.typeCursor = 0
		}
		return m, nil

	case stepSelectType:
		switch msg.String() {
		case "down":
			if m.typeCursor < len(typeOptions)-1 {
				m.typeCursor++
			}
		case "up":
			if m.typeCursor > 0 {
				m.typeCursor--
			}
		case "enter":
			m.projType = projectType(m.typeCursor)
			if m.projType == projectGit {
				m.step = stepInputGitURL
				m.urlInput.Focus()
				return m, textinput.Blink
			}
			// Existing folder or new repo: ask for directory
			m.dirInput.SetValue(m.defaultBaseDir() + "/")
			m.dirInput.Focus()
			m.step = stepInputDir
			return m, textinput.Blink
		}
		return m, nil

	case stepInputGitURL:
		if msg.String() == "enter" {
			url := strings.TrimSpace(m.urlInput.Value())
			if url == "" {
				m.message = "Git URL is required"
				return m, nil
			}
			if !isGitURL(url) {
				m.message = "Doesn't look like a git URL"
				return m, nil
			}
			m.gitURL = url
			m.message = ""
			// Pre-fill dir with baseDir/repoName
			defaultDir := m.defaultBaseDir()
			if defaultDir != "" {
				defaultDir += "/" + git.RepoName(url)
			}
			m.dirInput.SetValue(defaultDir)
			m.dirInput.Focus()
			m.step = stepInputDir
			return m, textinput.Blink
		}
		var cmd tea.Cmd
		m.urlInput, cmd = m.urlInput.Update(msg)
		return m, cmd

	case stepInputDir:
		switch msg.String() {
		case "enter":
			dir := strings.TrimSpace(m.dirInput.Value())
			if dir == "" {
				m.message = "Directory is required"
				return m, nil
			}
			// Expand ~ to absolute path
			if strings.HasPrefix(dir, "~/") {
				home, _ := os.UserHomeDir()
				dir = filepath.Join(home, dir[2:])
			}
			dir = strings.TrimSuffix(dir, "/")
			m.repoDir = dir
			m.message = ""
			m.step = stepInputName
			m.nameInput.SetValue(filepath.Base(dir))
			m.nameInput.Focus()
			return m, textinput.Blink
		case "tab":
			completed, matches := completePath(m.dirInput.Value())
			m.dirInput.SetValue(completed)
			m.dirInput.SetCursor(len(completed))
			if matches > 1 {
				m.message = fmt.Sprintf("%d matches", matches)
			} else {
				m.message = ""
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.dirInput, cmd = m.dirInput.Update(msg)
		return m, cmd

	case stepSelectBranch:
		switch msg.String() {
		case "down":
			if m.branchCursor < len(m.branches)-1 {
				m.branchCursor++
			}
		case "up":
			if m.branchCursor > 0 {
				m.branchCursor--
			}
		case "enter":
			m.selectedBranch = m.branches[m.branchCursor]
			m.step = stepInputName
			m.nameInput.SetValue(m.defaultName())
			m.nameInput.Focus()
			return m, textinput.Blink
		}
		return m, nil

	case stepInputName:
		if msg.String() == "enter" {
			name := strings.TrimSpace(m.nameInput.Value())
			if name == "" {
				m.message = "Name is required"
				return m, nil
			}
			if m.hostName == "" && config.WorkspaceExists(name) {
				m.message = fmt.Sprintf("Workspace %q already exists", name)
				return m, nil
			}
			m.wsName = name
			m.message = ""
			m.step = stepConfirm
			return m, nil
		}
		var cmd tea.Cmd
		m.nameInput, cmd = m.nameInput.Update(msg)
		return m, cmd

	case stepConfirm:
		if msg.String() == "enter" {
			m.step = stepCreating
			m.message = "Creating workspace..."
			return m, m.doSetup()
		}
		return m, nil
	}

	return m, nil
}

func (m newWSModel) defaultBaseDir() string {
	if m.hostName != "" {
		if host, err := config.LoadHost(m.hostName); err == nil && host.WorkspaceDir != "" {
			return host.WorkspaceDir
		}
	}
	return m.baseDir
}

func (m newWSModel) defaultName() string {
	if m.gitURL != "" {
		return git.RepoName(m.gitURL)
	}
	return filepath.Base(m.repoDir)
}

func (m newWSModel) loadBranches() tea.Cmd {
	dir := m.repoDir
	return func() tea.Msg {
		if !git.IsGitRepo(dir) {
			return branchesMsg{}
		}
		branches, err := git.ListLocalBranches(dir)
		return branchesMsg{branches: branches, err: err}
	}
}

// doSetup performs clone (if needed), directory creation, and workspace save on confirm.
func (m newWSModel) doSetup() tea.Cmd {
	name := m.wsName
	dir := m.repoDir
	gitURL := m.gitURL
	projType := m.projType
	branch := m.selectedBranch
	hostName := m.hostName

	return func() tea.Msg {
		if hostName != "" {
			return m.doRemoteSetup(name, dir, gitURL, hostName)
		}

		// Clone if git
		if projType == projectGit && gitURL != "" {
			if err := git.Clone(gitURL, dir); err != nil {
				return setupDoneMsg{err: fmt.Errorf("clone: %w", err)}
			}
		}

		// Existing folder: verify it exists, git init if needed
		if projType == projectExisting {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				return setupDoneMsg{err: fmt.Errorf("directory does not exist: %s", dir)}
			}
			if !git.IsGitRepo(dir) {
				if err := exec.Command("git", "init", dir).Run(); err != nil {
					return setupDoneMsg{err: fmt.Errorf("git init: %w", err)}
				}
			}
		}

		// Create dir + git init if new repo
		if projType == projectBlank {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return setupDoneMsg{err: fmt.Errorf("mkdir: %w", err)}
				}
			}
			if !git.IsGitRepo(dir) {
				if err := exec.Command("git", "init", dir).Run(); err != nil {
					return setupDoneMsg{err: fmt.Errorf("git init: %w", err)}
				}
			}
		}

		defaultBranch := "main"
		if git.IsGitRepo(dir) {
			defaultBranch = git.DefaultBranch(dir)
		}
		if branch == "" {
			branch = defaultBranch
		}

		ws := config.Workspace{
			Name:          name,
			Dir:           dir,
			DefaultBranch: defaultBranch,
			Layout:        "default",
			AutoClaude:    true,
		}
		if err := config.SaveWorkspace(ws); err != nil {
			return setupDoneMsg{err: fmt.Errorf("save: %w", err)}
		}
		return setupDoneMsg{}
	}
}

func (m newWSModel) doRemoteSetup(name, dir, gitURL, hostName string) setupDoneMsg {
	host, err := config.LoadHost(hostName)
	if err != nil {
		return setupDoneMsg{err: err}
	}

	if gitURL != "" {
		ssh.EnsureGitHubHostKey(host.SSH)
	}
	remoteArgs := fmt.Sprintf("~/.local/bin/ws new --non-interactive --name %s --dir %s", name, dir)
	if gitURL != "" {
		remoteArgs += fmt.Sprintf(" --git-url %s", gitURL)
	}
	if _, err := ssh.Run(host.SSH, remoteArgs); err != nil {
		return setupDoneMsg{err: fmt.Errorf("remote setup: %w", err)}
	}

	return setupDoneMsg{}
}

func (m newWSModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("New Workspace"))
	b.WriteString("\n\n")

	switch m.step {
	case stepSelectHost:
		b.WriteString(normalStyle.Render("  Where to create workspace:"))
		b.WriteString("\n\n")
		if m.hostCursor == 0 {
			b.WriteString(selectedStyle.Render("  > Local"))
		} else {
			b.WriteString(normalStyle.Render("    Local"))
		}
		b.WriteString("\n")
		for i, h := range m.hosts {
			label := fmt.Sprintf("%s (%s)", h.Name, h.SSH)
			if i+1 == m.hostCursor {
				b.WriteString(selectedStyle.Render(fmt.Sprintf("  > %s", label)))
			} else {
				b.WriteString(normalStyle.Render(fmt.Sprintf("    %s", label)))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("  ↑/↓: navigate  Enter: select  Esc: cancel"))
		b.WriteString("\n")

	case stepSelectType:
		location := "Local"
		if m.hostName != "" {
			location = m.hostName
		}
		b.WriteString(normalStyle.Render(fmt.Sprintf("  Location: %s", location)))
		b.WriteString("\n\n")
		b.WriteString(normalStyle.Render("  Project type:"))
		b.WriteString("\n")
		for i, opt := range typeOptions {
			if i == m.typeCursor {
				b.WriteString(selectedStyle.Render(fmt.Sprintf("  > %s", opt)))
			} else {
				b.WriteString(normalStyle.Render(fmt.Sprintf("    %s", opt)))
			}
			b.WriteString("\n")
		}

	case stepInputGitURL:
		location := "Local"
		if m.hostName != "" {
			location = m.hostName
		}
		b.WriteString(normalStyle.Render(fmt.Sprintf("  Location: %s", location)))
		b.WriteString("\n\n")
		b.WriteString(normalStyle.Render("  Git URL:"))
		b.WriteString("\n  ")
		b.WriteString(m.urlInput.View())
		b.WriteString("\n")

	case stepInputDir:
		location := "Local"
		if m.hostName != "" {
			location = m.hostName
		}
		b.WriteString(normalStyle.Render(fmt.Sprintf("  Location: %s", location)))
		b.WriteString("\n")
		if m.gitURL != "" {
			b.WriteString(normalStyle.Render(fmt.Sprintf("  Clone: %s", m.gitURL)))
			b.WriteString("\n")
		}
		if m.projType == projectExisting {
			b.WriteString(normalStyle.Render("  Type: Existing folder"))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(normalStyle.Render("  Directory:"))
		b.WriteString("\n  ")
		b.WriteString(m.dirInput.View())
		b.WriteString("\n")

	case stepSelectBranch:
		b.WriteString(normalStyle.Render(fmt.Sprintf("  Dir: %s", m.repoDir)))
		b.WriteString("\n\n")
		b.WriteString(normalStyle.Render("  Select branch:"))
		b.WriteString("\n")
		maxVisible := 15
		start := 0
		if m.branchCursor >= maxVisible {
			start = m.branchCursor - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(m.branches) {
			end = len(m.branches)
		}
		for i := start; i < end; i++ {
			br := m.branches[i]
			if i == m.branchCursor {
				b.WriteString(selectedStyle.Render(fmt.Sprintf("  > %s", br)))
			} else {
				b.WriteString(normalStyle.Render(fmt.Sprintf("    %s", br)))
			}
			b.WriteString("\n")
		}
		if end < len(m.branches) {
			b.WriteString(helpStyle.Render(fmt.Sprintf("    ... and %d more", len(m.branches)-end)))
			b.WriteString("\n")
		}

	case stepInputName:
		b.WriteString(normalStyle.Render("  Workspace name:"))
		b.WriteString("\n  ")
		b.WriteString(m.nameInput.View())
		b.WriteString("\n")

	case stepConfirm:
		b.WriteString(normalStyle.Render("  Review:"))
		b.WriteString("\n")
		b.WriteString(normalStyle.Render(fmt.Sprintf("    Name:   %s", m.wsName)))
		b.WriteString("\n")
		if m.hostName != "" {
			b.WriteString(normalStyle.Render(fmt.Sprintf("    Host:   %s", m.hostName)))
			b.WriteString("\n")
		}
		b.WriteString(normalStyle.Render(fmt.Sprintf("    Dir:    %s", m.repoDir)))
		b.WriteString("\n")
		if m.gitURL != "" {
			b.WriteString(normalStyle.Render(fmt.Sprintf("    Clone:  %s", m.gitURL)))
			b.WriteString("\n")
		}
		if m.selectedBranch != "" {
			b.WriteString(normalStyle.Render(fmt.Sprintf("    Branch: %s", m.selectedBranch)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("  Enter: create  Esc: cancel"))
		b.WriteString("\n")

	case stepCreating:
		b.WriteString(normalStyle.Render("  " + m.message))
		b.WriteString("\n")
	}

	if m.message != "" && m.step != stepCreating {
		b.WriteString("\n")
		b.WriteString("  " + warnStyle.Render(m.message))
		b.WriteString("\n")
	}

	if m.step != stepConfirm {
		b.WriteString("\n")
		if m.step == stepInputDir {
			b.WriteString(helpStyle.Render("  Tab: complete  Esc: cancel"))
		} else {
			b.WriteString(helpStyle.Render("  Esc: cancel"))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// completePath does bash-style tab completion on a partial path.
// Returns the completed path and the number of matches found.
func completePath(partial string) (string, int) {
	if partial == "" {
		return partial, 0
	}

	// Expand ~ to home directory
	expanded := partial
	home, _ := os.UserHomeDir()
	hasTilde := false
	if strings.HasPrefix(expanded, "~/") || expanded == "~" {
		hasTilde = true
		expanded = filepath.Join(home, expanded[1:])
	}

	// If it already ends with / and is a valid dir, list its contents
	if strings.HasSuffix(expanded, "/") {
		entries, err := os.ReadDir(expanded)
		if err != nil {
			return partial, 0
		}
		// Filter to directories only
		var dirs []string
		for _, e := range entries {
			if e.IsDir() {
				dirs = append(dirs, e.Name())
			}
		}
		if len(dirs) == 1 {
			result := expanded + dirs[0] + "/"
			if hasTilde {
				result = "~/" + strings.TrimPrefix(result, home+"/")
			}
			return result, 1
		}
		return partial, len(dirs)
	}

	dir := filepath.Dir(expanded)
	prefix := filepath.Base(expanded)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return partial, 0
	}

	var matches []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), prefix) {
			matches = append(matches, e.Name())
		}
	}

	if len(matches) == 0 {
		return partial, 0
	}

	if len(matches) == 1 {
		result := filepath.Join(dir, matches[0]) + "/"
		if hasTilde {
			result = "~/" + strings.TrimPrefix(result, home+"/")
		}
		return result, 1
	}

	// Multiple matches: complete to longest common prefix
	common := matches[0]
	for _, m := range matches[1:] {
		i := 0
		for i < len(common) && i < len(m) && common[i] == m[i] {
			i++
		}
		common = common[:i]
	}

	result := filepath.Join(dir, common)
	if hasTilde {
		result = "~/" + strings.TrimPrefix(result, home+"/")
	}
	return result, len(matches)
}
