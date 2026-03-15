package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/skarlsson/workshell/internal/config"
	"github.com/skarlsson/workshell/internal/deps"
)

type settingsStep int

const (
	stepSettingsMenu settingsStep = iota
	stepHostsList
	stepHostAdd
	stepHostEdit
	stepGlobalConfig
	stepDeps
)

type settingsModel struct {
	step       settingsStep
	menuCursor int

	// Hosts
	hosts      []config.HostConfig
	hostCursor int

	// Host add/edit inputs
	hostNameInput textinput.Model
	hostSSHInput  textinput.Model
	hostDirInput  textinput.Model
	hostAddStep   int // 0=name, 1=ssh, 2=dir
	editingHost   string

	// Global config
	globalCfg     config.GlobalConfig
	cfgCursor     int
	cfgEditing    bool
	cfgInput      textinput.Model

	// Dependencies
	depStatuses []deps.ToolStatus

	message   string
	done      bool
	cancelled bool
}

var settingsMenuItems = []string{"Hosts", "Global Config", "Dependencies"}

func newSettingsModel() settingsModel {
	nameTI := textinput.New()
	nameTI.Placeholder = "myhost"
	nameTI.Width = 40

	sshTI := textinput.New()
	sshTI.Placeholder = "user@host or ssh-config-name"
	sshTI.Width = 60

	dirTI := textinput.New()
	dirTI.Placeholder = "~/dev (optional)"
	dirTI.Width = 60

	cfgTI := textinput.New()
	cfgTI.Width = 60

	return settingsModel{
		step:          stepSettingsMenu,
		hostNameInput: nameTI,
		hostSSHInput:  sshTI,
		hostDirInput:  dirTI,
		cfgInput:      cfgTI,
	}
}

func (m settingsModel) Init() tea.Cmd {
	return nil
}

func (m settingsModel) Update(msg tea.Msg) (settingsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.step {
		case stepSettingsMenu:
			return m.updateMenu(msg)
		case stepHostsList:
			return m.updateHostsList(msg)
		case stepHostAdd, stepHostEdit:
			return m.updateHostAdd(msg)
		case stepGlobalConfig:
			return m.updateGlobalConfig(msg)
		case stepDeps:
			return m.updateDeps(msg)
		}
	}

	// Forward to active text inputs
	var cmd tea.Cmd
	switch m.step {
	case stepHostAdd, stepHostEdit:
		switch m.hostAddStep {
		case 0:
			m.hostNameInput, cmd = m.hostNameInput.Update(msg)
		case 1:
			m.hostSSHInput, cmd = m.hostSSHInput.Update(msg)
		case 2:
			m.hostDirInput, cmd = m.hostDirInput.Update(msg)
		}
	case stepGlobalConfig:
		if m.cfgEditing {
			m.cfgInput, cmd = m.cfgInput.Update(msg)
		}
	}
	return m, cmd
}

// Top-level settings menu
func (m settingsModel) updateMenu(msg tea.KeyMsg) (settingsModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.cancelled = true
	case "down":
		if m.menuCursor < len(settingsMenuItems)-1 {
			m.menuCursor++
		}
	case "up":
		if m.menuCursor > 0 {
			m.menuCursor--
		}
	case "enter":
		switch m.menuCursor {
		case 0: // Hosts
			m.hosts, _ = config.LoadHosts()
			m.hostCursor = 0
			m.step = stepHostsList
		case 1: // Global Config
			m.globalCfg, _ = config.LoadGlobalConfig()
			m.cfgCursor = 0
			m.cfgEditing = false
			m.step = stepGlobalConfig
		case 2: // Dependencies
			m.depStatuses = deps.CheckAll()
			m.step = stepDeps
		}
		m.message = ""
	}
	return m, nil
}

// Hosts list view
func (m settingsModel) updateHostsList(msg tea.KeyMsg) (settingsModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.step = stepSettingsMenu
		m.message = ""
	case "down":
		if m.hostCursor < len(m.hosts) {
			m.hostCursor++
		}
	case "up":
		if m.hostCursor > 0 {
			m.hostCursor--
		}
	case "a":
		m.hostAddStep = 0
		m.hostNameInput.SetValue("")
		m.hostSSHInput.SetValue("")
		m.hostDirInput.SetValue("")
		m.hostNameInput.Focus()
		m.editingHost = ""
		m.step = stepHostAdd
		m.message = ""
		return m, textinput.Blink
	case "enter":
		if m.hostCursor < len(m.hosts) {
			h := m.hosts[m.hostCursor]
			m.hostAddStep = 0
			m.hostNameInput.SetValue(h.Name)
			m.hostSSHInput.SetValue(h.SSH)
			m.hostDirInput.SetValue(h.WorkspaceDir)
			m.hostNameInput.Focus()
			m.editingHost = h.Name
			m.step = stepHostEdit
			m.message = ""
			return m, textinput.Blink
		}
	case "d":
		if m.hostCursor < len(m.hosts) {
			h := m.hosts[m.hostCursor]
			if err := config.RemoveHost(h.Name); err != nil {
				m.message = fmt.Sprintf("Remove failed: %v", err)
			} else {
				m.message = fmt.Sprintf("Removed %s", h.Name)
				m.hosts, _ = config.LoadHosts()
				if m.hostCursor >= len(m.hosts) && len(m.hosts) > 0 {
					m.hostCursor = len(m.hosts) - 1
				}
			}
		}
	}
	return m, nil
}

// Host add/edit input flow
func (m settingsModel) updateHostAdd(msg tea.KeyMsg) (settingsModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.hosts, _ = config.LoadHosts()
		m.hostCursor = 0
		m.step = stepHostsList
		m.message = ""
		return m, nil
	case "enter":
		switch m.hostAddStep {
		case 0:
			name := strings.TrimSpace(m.hostNameInput.Value())
			if name == "" {
				m.message = "Host name is required"
				return m, nil
			}
			m.hostAddStep = 1
			m.hostSSHInput.Focus()
			m.message = ""
			return m, textinput.Blink
		case 1:
			ssh := strings.TrimSpace(m.hostSSHInput.Value())
			if ssh == "" {
				m.message = "SSH target is required"
				return m, nil
			}
			m.hostAddStep = 2
			m.hostDirInput.Focus()
			m.message = ""
			return m, textinput.Blink
		case 2:
			name := strings.TrimSpace(m.hostNameInput.Value())
			sshTarget := strings.TrimSpace(m.hostSSHInput.Value())
			dir := strings.TrimSpace(m.hostDirInput.Value())

			h := config.HostConfig{
				Name:         name,
				SSH:          sshTarget,
				WorkspaceDir: dir,
			}

			if m.step == stepHostEdit && m.editingHost != "" {
				_ = config.RemoveHost(m.editingHost)
			}

			if err := config.AddHost(h); err != nil {
				m.message = fmt.Sprintf("Save failed: %v", err)
				return m, nil
			}

			m.hosts, _ = config.LoadHosts()
			m.hostCursor = 0
			m.step = stepHostsList
			m.message = fmt.Sprintf("Saved host %s", name)
			return m, nil
		}
	}

	var cmd tea.Cmd
	switch m.hostAddStep {
	case 0:
		m.hostNameInput, cmd = m.hostNameInput.Update(msg)
	case 1:
		m.hostSSHInput, cmd = m.hostSSHInput.Update(msg)
	case 2:
		m.hostDirInput, cmd = m.hostDirInput.Update(msg)
	}
	return m, cmd
}

// Global config editing
func (m settingsModel) updateGlobalConfig(msg tea.KeyMsg) (settingsModel, tea.Cmd) {
	if m.cfgEditing {
		switch msg.String() {
		case "esc":
			m.cfgEditing = false
			m.message = ""
			return m, nil
		case "enter":
			val := strings.TrimSpace(m.cfgInput.Value())
			switch m.cfgCursor {
			case 0: // FocusMode — toggle handled below, shouldn't reach here
			case 1: // WorkMonitor
				m.globalCfg.WorkMonitor = val
			case 2: // WorkspaceBaseDir
				m.globalCfg.WorkspaceBaseDir = val
			}
			if err := config.SaveGlobalConfig(m.globalCfg); err != nil {
				m.message = fmt.Sprintf("Save failed: %v", err)
			} else {
				m.message = "Saved"
			}
			m.cfgEditing = false
			return m, nil
		}
		var cmd tea.Cmd
		m.cfgInput, cmd = m.cfgInput.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "esc":
		m.step = stepSettingsMenu
		m.message = ""
	case "down":
		if m.cfgCursor < 2 {
			m.cfgCursor++
		}
	case "up":
		if m.cfgCursor > 0 {
			m.cfgCursor--
		}
	case "enter":
		switch m.cfgCursor {
		case 0: // FocusMode — toggle
			if m.globalCfg.FocusMode == "multi" {
				m.globalCfg.FocusMode = "single"
			} else {
				m.globalCfg.FocusMode = "multi"
			}
			if err := config.SaveGlobalConfig(m.globalCfg); err != nil {
				m.message = fmt.Sprintf("Save failed: %v", err)
			} else {
				m.message = "Saved"
			}
		case 1: // WorkMonitor — text input
			m.cfgEditing = true
			m.cfgInput.SetValue(m.globalCfg.WorkMonitor)
			m.cfgInput.Focus()
			m.message = ""
			return m, textinput.Blink
		case 2: // WorkspaceBaseDir — text input
			m.cfgEditing = true
			m.cfgInput.SetValue(m.globalCfg.WorkspaceBaseDir)
			m.cfgInput.Focus()
			m.message = ""
			return m, textinput.Blink
		}
	}
	return m, nil
}

// Dependencies view
func (m settingsModel) updateDeps(msg tea.KeyMsg) (settingsModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.step = stepSettingsMenu
		m.message = ""
	case "i":
		installed := deps.InstallMissing(false)
		if len(installed) > 0 {
			m.message = fmt.Sprintf("Installed: %s", strings.Join(installed, ", "))
		} else {
			m.message = "Nothing to install"
		}
		m.depStatuses = deps.CheckAll()
	}
	return m, nil
}

func (m settingsModel) View() string {
	var b strings.Builder

	switch m.step {
	case stepSettingsMenu:
		b.WriteString(titleStyle.Render("Settings"))
		b.WriteString("\n\n")
		for i, item := range settingsMenuItems {
			if i == m.menuCursor {
				b.WriteString(selectedStyle.Render(fmt.Sprintf("  > %s", item)))
			} else {
				b.WriteString(normalStyle.Render(fmt.Sprintf("    %s", item)))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		if m.message != "" {
			b.WriteString("  " + warnStyle.Render(m.message) + "\n\n")
		}
		b.WriteString(helpStyle.Render("  ↑/↓: navigate  Enter: select  Esc: back"))
		b.WriteString("\n")

	case stepHostsList:
		b.WriteString(titleStyle.Render("Hosts"))
		b.WriteString("\n\n")
		if len(m.hosts) == 0 {
			b.WriteString(inactiveStyle.Render("  No hosts configured."))
			b.WriteString("\n")
		} else {
			for i, h := range m.hosts {
				label := fmt.Sprintf("%s  (%s)", h.Name, h.SSH)
				if h.WorkspaceDir != "" {
					label += fmt.Sprintf("  %s", h.WorkspaceDir)
				}
				if i == m.hostCursor {
					b.WriteString(selectedStyle.Render(fmt.Sprintf("  > %s", label)))
				} else {
					b.WriteString(normalStyle.Render(fmt.Sprintf("    %s", label)))
				}
				b.WriteString("\n")
			}
		}
		// "Add new host..." option
		addLabel := "Add new host..."
		if m.hostCursor == len(m.hosts) {
			b.WriteString(selectedStyle.Render(fmt.Sprintf("  > %s", addLabel)))
		} else {
			b.WriteString(normalStyle.Render(fmt.Sprintf("    %s", addLabel)))
		}
		b.WriteString("\n\n")
		if m.message != "" {
			b.WriteString("  " + warnStyle.Render(m.message) + "\n\n")
		}
		b.WriteString(helpStyle.Render("  ↑/↓: navigate  Enter: edit  a: add  d: remove  Esc: back"))
		b.WriteString("\n")

	case stepHostAdd, stepHostEdit:
		if m.step == stepHostEdit {
			b.WriteString(titleStyle.Render("Edit Host"))
		} else {
			b.WriteString(titleStyle.Render("Add Host"))
		}
		b.WriteString("\n\n")
		// Show completed fields
		if m.hostAddStep >= 0 {
			label := "  Name: "
			if m.hostAddStep == 0 {
				b.WriteString(normalStyle.Render(label))
				b.WriteString(m.hostNameInput.View())
			} else {
				b.WriteString(normalStyle.Render(label + m.hostNameInput.Value()))
			}
			b.WriteString("\n")
		}
		if m.hostAddStep >= 1 {
			label := "  SSH:  "
			if m.hostAddStep == 1 {
				b.WriteString(normalStyle.Render(label))
				b.WriteString(m.hostSSHInput.View())
			} else {
				b.WriteString(normalStyle.Render(label + m.hostSSHInput.Value()))
			}
			b.WriteString("\n")
		}
		if m.hostAddStep >= 2 {
			label := "  Dir:  "
			b.WriteString(normalStyle.Render(label))
			b.WriteString(m.hostDirInput.View())
			b.WriteString("\n")
		}
		b.WriteString("\n")
		if m.message != "" {
			b.WriteString("  " + warnStyle.Render(m.message) + "\n\n")
		}
		b.WriteString(helpStyle.Render("  Enter: next  Esc: cancel"))
		b.WriteString("\n")

	case stepGlobalConfig:
		b.WriteString(titleStyle.Render("Global Config"))
		b.WriteString("\n\n")
		type cfgField struct {
			label string
			value string
		}
		fields := []cfgField{
			{"Focus mode", fmt.Sprintf("[%s]", m.globalCfg.FocusMode)},
			{"Work monitor", m.globalCfg.WorkMonitor},
			{"Workspace base dir", m.globalCfg.WorkspaceBaseDir},
		}
		for i, f := range fields {
			val := f.value
			if val == "" {
				val = "(not set)"
			}
			if m.cfgEditing && i == m.cfgCursor {
				b.WriteString(normalStyle.Render(fmt.Sprintf("  %s: ", f.label)))
				b.WriteString(m.cfgInput.View())
			} else if i == m.cfgCursor {
				b.WriteString(selectedStyle.Render(fmt.Sprintf("  > %-20s %s", f.label+":", val)))
			} else {
				b.WriteString(normalStyle.Render(fmt.Sprintf("    %-20s %s", f.label+":", val)))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		if m.message != "" {
			b.WriteString("  " + warnStyle.Render(m.message) + "\n\n")
		}
		if m.cfgEditing {
			b.WriteString(helpStyle.Render("  Enter: save  Esc: cancel"))
		} else {
			b.WriteString(helpStyle.Render("  ↑/↓: navigate  Enter: edit/toggle  Esc: back"))
		}
		b.WriteString("\n")

	case stepDeps:
		b.WriteString(titleStyle.Render("Dependencies"))
		b.WriteString("\n\n")
		var required, optional []deps.ToolStatus
		for _, ts := range m.depStatuses {
			if ts.Required {
				required = append(required, ts)
			} else {
				optional = append(optional, ts)
			}
		}
		if len(required) > 0 {
			b.WriteString(normalStyle.Render("  Required:"))
			b.WriteString("\n")
			for _, ts := range required {
				if ts.Found {
					path := ts.Path
					if path == "" {
						path = "found"
					}
					b.WriteString(successStyle.Render(fmt.Sprintf("    ✓ %-16s %s", ts.Name, path)))
				} else {
					b.WriteString(errorStyle.Render(fmt.Sprintf("    ✗ %-16s (not found)", ts.Name)))
				}
				b.WriteString("\n")
			}
		}
		if len(optional) > 0 {
			b.WriteString("\n")
			b.WriteString(normalStyle.Render("  Optional:"))
			b.WriteString("\n")
			for _, ts := range optional {
				if ts.Found {
					path := ts.Path
					if path == "" {
						path = "found"
					}
					b.WriteString(successStyle.Render(fmt.Sprintf("    ✓ %-16s %s", ts.Name, path)))
				} else {
					b.WriteString(inactiveStyle.Render(fmt.Sprintf("    ✗ %-16s (not found)", ts.Name)))
				}
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
		if m.message != "" {
			b.WriteString("  " + warnStyle.Render(m.message) + "\n\n")
		}
		b.WriteString(helpStyle.Render("  i: install missing  Esc: back"))
		b.WriteString("\n")
	}

	return b.String()
}
