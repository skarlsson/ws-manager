package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/skarlsson/ws-manager/internal/config"
	"github.com/skarlsson/ws-manager/internal/git"
)

type newWSStep int

const (
	stepBrowseDir newWSStep = iota
	stepSelectType
	stepInputGitURL
	stepSelectBranch
	stepInputName
	stepConfirm
	stepCreating
)

type projectType int

const (
	projectGit projectType = iota
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
	step      newWSStep
	urlInput  textinput.Model
	nameInput textinput.Model

	// Directory browser
	browseDir  string
	browseDirs []string
	browseCur  int

	// Resolved values
	rootDir        string
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

var typeOptions = []string{"Git clone", "Blank project"}

func newNewWSModel(baseDir string) newWSModel {
	urlTI := textinput.New()
	urlTI.Placeholder = "git@github.com:user/repo.git"
	urlTI.Width = 60

	nameTI := textinput.New()
	nameTI.Placeholder = "my-workspace"
	nameTI.Width = 40

	m := newWSModel{
		step:      stepBrowseDir,
		urlInput:  urlTI,
		nameInput: nameTI,
		baseDir:   baseDir,
		browseDir: baseDir,
	}
	m.refreshBrowse()
	return m
}

func (m *newWSModel) refreshBrowse() {
	m.browseDirs = nil
	entries, err := os.ReadDir(m.browseDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			m.browseDirs = append(m.browseDirs, e.Name())
		}
	}
	sort.Strings(m.browseDirs)
	m.browseCur = 0
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
	}
	return m, cmd
}

func (m newWSModel) handleKey(msg tea.KeyMsg) (newWSModel, tea.Cmd) {
	switch m.step {
	case stepBrowseDir:
		switch msg.String() {
		case "j", "down":
			if m.browseCur < len(m.browseDirs)-1 {
				m.browseCur++
			}
		case "k", "up":
			if m.browseCur > 0 {
				m.browseCur--
			}
		case "l", "right":
			// Descend into selected directory
			if len(m.browseDirs) > 0 {
				m.browseDir = filepath.Join(m.browseDir, m.browseDirs[m.browseCur])
				m.refreshBrowse()
			}
		case "backspace", "h", "left":
			// Go up
			parent := filepath.Dir(m.browseDir)
			if parent != m.browseDir {
				prev := filepath.Base(m.browseDir)
				m.browseDir = parent
				m.refreshBrowse()
				for i, d := range m.browseDirs {
					if d == prev {
						m.browseCur = i
						break
					}
				}
			}
		case "enter":
			// Select the highlighted directory (or current dir if empty)
			selected := m.browseDir
			if len(m.browseDirs) > 0 {
				selected = filepath.Join(m.browseDir, m.browseDirs[m.browseCur])
			}
			m.rootDir = selected
			m.message = ""

			// If it's already a git repo, accept as-is — skip type and branch selection
			if git.IsGitRepo(m.rootDir) {
				m.repoDir = m.rootDir
				m.step = stepInputName
				m.nameInput.SetValue(filepath.Base(m.rootDir))
				m.nameInput.Focus()
				return m, textinput.Blink
			}

			m.step = stepSelectType
			m.typeCursor = 0
			return m, nil
		}
		return m, nil

	case stepSelectType:
		switch msg.String() {
		case "j", "down":
			if m.typeCursor < len(typeOptions)-1 {
				m.typeCursor++
			}
		case "k", "up":
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
			// Blank project
			m.repoDir = m.rootDir
			m.step = stepInputName
			m.nameInput.SetValue(filepath.Base(m.rootDir))
			m.nameInput.Focus()
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
			m.repoDir = filepath.Join(m.rootDir, git.RepoName(url))
			m.message = ""
			// Go to name — clone deferred to confirm
			m.step = stepInputName
			m.nameInput.SetValue(git.RepoName(url))
			m.nameInput.Focus()
			return m, textinput.Blink
		}
		var cmd tea.Cmd
		m.urlInput, cmd = m.urlInput.Update(msg)
		return m, cmd

	case stepSelectBranch:
		switch msg.String() {
		case "j", "down":
			if m.branchCursor < len(m.branches)-1 {
				m.branchCursor++
			}
		case "k", "up":
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
			if config.WorkspaceExists(name) {
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

	return func() tea.Msg {
		// Clone if git
		if projType == projectGit && gitURL != "" {
			if err := git.Clone(gitURL, dir); err != nil {
				return setupDoneMsg{err: fmt.Errorf("clone: %w", err)}
			}
		}

		// Create dir if blank and doesn't exist
		if projType == projectBlank {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				if err := os.MkdirAll(dir, 0755); err != nil {
					return setupDoneMsg{err: fmt.Errorf("mkdir: %w", err)}
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

func (m newWSModel) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("New Workspace"))
	b.WriteString("\n\n")

	switch m.step {
	case stepBrowseDir:
		b.WriteString(normalStyle.Render(fmt.Sprintf("  %s/", m.browseDir)))
		b.WriteString("\n\n")
		if len(m.browseDirs) == 0 {
			b.WriteString(inactiveStyle.Render("    (empty)"))
			b.WriteString("\n")
		} else {
			maxVisible := 15
			start := 0
			if m.browseCur >= maxVisible {
				start = m.browseCur - maxVisible + 1
			}
			end := start + maxVisible
			if end > len(m.browseDirs) {
				end = len(m.browseDirs)
			}
			for i := start; i < end; i++ {
				name := m.browseDirs[i] + "/"
				if i == m.browseCur {
					b.WriteString(selectedStyle.Render(fmt.Sprintf("  > %s", name)))
				} else {
					b.WriteString(normalStyle.Render(fmt.Sprintf("    %s", name)))
				}
				b.WriteString("\n")
			}
			if end < len(m.browseDirs) {
				b.WriteString(helpStyle.Render(fmt.Sprintf("    ... and %d more", len(m.browseDirs)-end)))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("  j/k: navigate  l: open dir  h: up  Enter: select this dir  Esc: cancel"))
		b.WriteString("\n")

	case stepSelectType:
		b.WriteString(normalStyle.Render(fmt.Sprintf("  Root: %s", m.rootDir)))
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
		b.WriteString(normalStyle.Render(fmt.Sprintf("  Root: %s", m.rootDir)))
		b.WriteString("\n\n")
		b.WriteString(normalStyle.Render("  Git URL:"))
		b.WriteString("\n  ")
		b.WriteString(m.urlInput.View())
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

	if m.step != stepConfirm && m.step != stepBrowseDir {
		b.WriteString("\n")
		b.WriteString(helpStyle.Render("  Esc: cancel"))
		b.WriteString("\n")
	}

	return b.String()
}
