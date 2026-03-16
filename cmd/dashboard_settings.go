package cmd

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/skarlsson/workshell/internal/config"
	"github.com/skarlsson/workshell/internal/deps"
	"github.com/skarlsson/workshell/internal/monitor"
)

type monitorOption struct {
	connector string
	label     string
	primary   bool
}

type settingsStep int

const (
	stepSettingsMenu settingsStep = iota
	stepHostsList
	stepHostAdd
	stepHostEdit
	stepGlobalConfig
	stepMonitorSelect
	stepKeybindings
	stepKeybindingAdd
	stepKeybindingEdit
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

	// Monitor selection
	monitors      []monitorOption
	monitorCursor int

	// Keybindings
	keybindings   []config.Keybinding
	kbCursor      int
	kbBindingInput textinput.Model
	kbCommandInput textinput.Model
	kbAddStep     int // 0=binding, 1=command
	kbEditingIdx  int // -1 for new

	// Dependencies
	depStatuses []deps.ToolStatus

	message   string
	done      bool
	cancelled bool
}

var settingsMenuItems = []string{"Hosts", "Global Config", "Keybindings", "Dependencies"}

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

	kbBindTI := textinput.New()
	kbBindTI.Placeholder = "<super>r"
	kbBindTI.Width = 40

	kbCmdTI := textinput.New()
	kbCmdTI.Placeholder = "rotate"
	kbCmdTI.Width = 40

	return settingsModel{
		step:           stepSettingsMenu,
		hostNameInput:  nameTI,
		hostSSHInput:   sshTI,
		hostDirInput:   dirTI,
		cfgInput:       cfgTI,
		kbBindingInput: kbBindTI,
		kbCommandInput: kbCmdTI,
		kbEditingIdx:   -1,
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
		case stepMonitorSelect:
			return m.updateMonitorSelect(msg)
		case stepKeybindings:
			return m.updateKeybindings(msg)
		case stepKeybindingAdd, stepKeybindingEdit:
			return m.updateKeybindingAdd(msg)
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
	case stepKeybindingAdd, stepKeybindingEdit:
		switch m.kbAddStep {
		case 0:
			m.kbBindingInput, cmd = m.kbBindingInput.Update(msg)
		case 1:
			m.kbCommandInput, cmd = m.kbCommandInput.Update(msg)
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
		case 2: // Keybindings
			m.globalCfg, _ = config.LoadGlobalConfig()
			m.keybindings = m.globalCfg.Keybindings
			if len(m.keybindings) == 0 {
				m.keybindings = config.DefaultKeybindings()
			}
			m.kbCursor = 0
			m.step = stepKeybindings
		case 3: // Dependencies
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
			case 1: // WorkspaceBaseDir
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
		if m.cfgCursor < 1 {
			m.cfgCursor++
		}
	case "up":
		if m.cfgCursor > 0 {
			m.cfgCursor--
		}
	case "enter":
		switch m.cfgCursor {
		case 0: // WorkMonitor — monitor picker
			m.monitors = nil
			m.monitorCursor = 0
			// "(none)" option first
			m.monitors = append(m.monitors, monitorOption{connector: "", label: "(none)", primary: false})
			if mons, err := monitor.ListMonitors(); err == nil {
				for i, mon := range mons {
					label := mon.Connector
					if mon.Primary {
						label += " (primary)"
					}
					label += fmt.Sprintf("  [%d,%d]", mon.X, mon.Y)
					opt := monitorOption{connector: mon.Connector, label: label, primary: mon.Primary}
					m.monitors = append(m.monitors, opt)
					// Default cursor to current config value, or primary
					if mon.Connector == m.globalCfg.WorkMonitor {
						m.monitorCursor = i + 1
					} else if mon.Primary && m.globalCfg.WorkMonitor == "" {
						m.monitorCursor = i + 1
					}
				}
			} else {
				m.message = fmt.Sprintf("Could not detect monitors: %v", err)
				return m, nil
			}
			m.step = stepMonitorSelect
			m.message = ""
		case 1: // WorkspaceBaseDir — text input
			m.cfgEditing = true
			m.cfgInput.SetValue(m.globalCfg.WorkspaceBaseDir)
			m.cfgInput.Focus()
			m.message = ""
			return m, textinput.Blink
		}
	}
	return m, nil
}

// Monitor selection
func (m settingsModel) updateMonitorSelect(msg tea.KeyMsg) (settingsModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.step = stepGlobalConfig
		m.message = ""
	case "down":
		if m.monitorCursor < len(m.monitors)-1 {
			m.monitorCursor++
		}
	case "up":
		if m.monitorCursor > 0 {
			m.monitorCursor--
		}
	case "enter":
		selected := m.monitors[m.monitorCursor]
		m.globalCfg.WorkMonitor = selected.connector
		if err := config.SaveGlobalConfig(m.globalCfg); err != nil {
			m.message = fmt.Sprintf("Save failed: %v", err)
		} else {
			if selected.connector == "" {
				m.message = "Work monitor cleared"
			} else {
				m.message = fmt.Sprintf("Work monitor set to %s", selected.connector)
			}
		}
		m.step = stepGlobalConfig
	}
	return m, nil
}

// Keybindings list view
func (m settingsModel) updateKeybindings(msg tea.KeyMsg) (settingsModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.step = stepSettingsMenu
		m.message = ""
	case "down":
		// +1 for the "Add new..." option
		if m.kbCursor < len(m.keybindings) {
			m.kbCursor++
		}
	case "up":
		if m.kbCursor > 0 {
			m.kbCursor--
		}
	case "a":
		m.kbAddStep = 0
		m.kbBindingInput.SetValue("")
		m.kbCommandInput.SetValue("")
		m.kbBindingInput.Focus()
		m.kbEditingIdx = -1
		m.step = stepKeybindingAdd
		m.message = ""
		return m, textinput.Blink
	case "enter":
		if m.kbCursor < len(m.keybindings) {
			kb := m.keybindings[m.kbCursor]
			m.kbAddStep = 0
			m.kbBindingInput.SetValue(kb.Binding)
			m.kbCommandInput.SetValue(kb.Command)
			m.kbBindingInput.Focus()
			m.kbEditingIdx = m.kbCursor
			m.step = stepKeybindingEdit
			m.message = ""
			return m, textinput.Blink
		} else {
			// "Add new..." selected
			m.kbAddStep = 0
			m.kbBindingInput.SetValue("")
			m.kbCommandInput.SetValue("")
			m.kbBindingInput.Focus()
			m.kbEditingIdx = -1
			m.step = stepKeybindingAdd
			m.message = ""
			return m, textinput.Blink
		}
	case "d":
		if m.kbCursor < len(m.keybindings) {
			m.keybindings = append(m.keybindings[:m.kbCursor], m.keybindings[m.kbCursor+1:]...)
			if err := m.saveKeybindings(); err != nil {
				m.message = fmt.Sprintf("Save failed: %v", err)
			} else {
				m.message = "Removed"
			}
			if m.kbCursor >= len(m.keybindings) && len(m.keybindings) > 0 {
				m.kbCursor = len(m.keybindings) - 1
			}
		}
	case "A":
		if err := applyKeybindings(m.keybindings); err != nil {
			m.message = fmt.Sprintf("Apply failed: %v", err)
		} else {
			m.message = fmt.Sprintf("Applied %d keybindings via gsettings", len(m.keybindings))
		}
	}
	return m, nil
}

// Keybinding add/edit input flow
func (m settingsModel) updateKeybindingAdd(msg tea.KeyMsg) (settingsModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.kbCursor = 0
		m.step = stepKeybindings
		m.message = ""
		return m, nil
	case "enter":
		switch m.kbAddStep {
		case 0:
			binding := strings.TrimSpace(m.kbBindingInput.Value())
			if binding == "" {
				m.message = "Binding is required (e.g. <super>r)"
				return m, nil
			}
			m.kbAddStep = 1
			m.kbCommandInput.Focus()
			m.message = ""
			return m, textinput.Blink
		case 1:
			command := strings.TrimSpace(m.kbCommandInput.Value())
			if command == "" {
				m.message = "Command is required (e.g. rotate)"
				return m, nil
			}
			binding := strings.TrimSpace(m.kbBindingInput.Value())
			kb := config.Keybinding{Binding: binding, Command: command}

			if m.kbEditingIdx >= 0 && m.kbEditingIdx < len(m.keybindings) {
				m.keybindings[m.kbEditingIdx] = kb
			} else {
				m.keybindings = append(m.keybindings, kb)
			}

			if err := m.saveKeybindings(); err != nil {
				m.message = fmt.Sprintf("Save failed: %v", err)
				return m, nil
			}

			m.kbCursor = 0
			m.step = stepKeybindings
			m.message = "Saved"
			return m, nil
		}
	}

	var cmd tea.Cmd
	switch m.kbAddStep {
	case 0:
		m.kbBindingInput, cmd = m.kbBindingInput.Update(msg)
	case 1:
		m.kbCommandInput, cmd = m.kbCommandInput.Update(msg)
	}
	return m, cmd
}

func (m *settingsModel) saveKeybindings() error {
	cfg, err := config.LoadGlobalConfig()
	if err != nil {
		return err
	}
	cfg.Keybindings = m.keybindings
	return config.SaveGlobalConfig(cfg)
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

	case stepMonitorSelect:
		b.WriteString(titleStyle.Render("Select Work Monitor"))
		b.WriteString("\n\n")
		for i, opt := range m.monitors {
			prefix := "    "
			if opt.connector == m.globalCfg.WorkMonitor {
				prefix = "  * "
			}
			line := fmt.Sprintf("%s%s", prefix, opt.label)
			if i == m.monitorCursor {
				b.WriteString(selectedStyle.Render(fmt.Sprintf("  > %s", opt.label)))
			} else {
				b.WriteString(normalStyle.Render(line))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
		if m.message != "" {
			b.WriteString("  " + warnStyle.Render(m.message) + "\n\n")
		}
		b.WriteString(helpStyle.Render("  ↑/↓: navigate  Enter: select  Esc: back"))
		b.WriteString("\n")

	case stepKeybindings:
		b.WriteString(titleStyle.Render("Keybindings"))
		b.WriteString("\n\n")
		if len(m.keybindings) == 0 {
			b.WriteString(inactiveStyle.Render("  No keybindings configured."))
			b.WriteString("\n")
		} else {
			for i, kb := range m.keybindings {
				label := fmt.Sprintf("%-16s → ws %s", kb.Binding, kb.Command)
				if i == m.kbCursor {
					b.WriteString(selectedStyle.Render(fmt.Sprintf("  > %s", label)))
				} else {
					b.WriteString(normalStyle.Render(fmt.Sprintf("    %s", label)))
				}
				b.WriteString("\n")
			}
		}
		addLabel := "Add new keybinding..."
		if m.kbCursor == len(m.keybindings) {
			b.WriteString(selectedStyle.Render(fmt.Sprintf("  > %s", addLabel)))
		} else {
			b.WriteString(normalStyle.Render(fmt.Sprintf("    %s", addLabel)))
		}
		b.WriteString("\n\n")
		if m.message != "" {
			b.WriteString("  " + warnStyle.Render(m.message) + "\n\n")
		}
		b.WriteString(helpStyle.Render("  ↑/↓: navigate  Enter: edit  a: add  d: remove  A: apply to GNOME  Esc: back"))
		b.WriteString("\n")

	case stepKeybindingAdd, stepKeybindingEdit:
		if m.step == stepKeybindingEdit {
			b.WriteString(titleStyle.Render("Edit Keybinding"))
		} else {
			b.WriteString(titleStyle.Render("Add Keybinding"))
		}
		b.WriteString("\n\n")
		if m.kbAddStep >= 0 {
			label := "  Binding: "
			if m.kbAddStep == 0 {
				b.WriteString(normalStyle.Render(label))
				b.WriteString(m.kbBindingInput.View())
			} else {
				b.WriteString(normalStyle.Render(label + m.kbBindingInput.Value()))
			}
			b.WriteString("\n")
		}
		if m.kbAddStep >= 1 {
			label := "  Command: "
			b.WriteString(normalStyle.Render(label))
			b.WriteString(m.kbCommandInput.View())
			b.WriteString("\n")
		}
		b.WriteString("\n")
		if m.message != "" {
			b.WriteString("  " + warnStyle.Render(m.message) + "\n\n")
		}
		b.WriteString(helpStyle.Render("  Enter: next  Esc: cancel"))
		b.WriteString("\n")

	case stepDeps:
		b.WriteString(titleStyle.Render("Dependencies"))
		b.WriteString(normalStyle.Render(fmt.Sprintf("  (session: %s)", deps.SessionType())))
		b.WriteString("\n\n")
		type catDef struct {
			key, label string
		}
		for _, cat := range []catDef{{"core", "Core"}, {"x11", "X11"}, {"wayland", "Wayland"}} {
			var items []deps.ToolStatus
			for _, ts := range m.depStatuses {
				if ts.Category == cat.key {
					items = append(items, ts)
				}
			}
			if len(items) == 0 {
				continue
			}
			b.WriteString(normalStyle.Render(fmt.Sprintf("  %s:", cat.label)))
			b.WriteString("\n")
			for _, ts := range items {
				if ts.Found {
					path := ts.Path
					if path == "" {
						path = "found"
					}
					b.WriteString(successStyle.Render(fmt.Sprintf("    ✓ %-18s %s", ts.Name, path)))
				} else {
					if ts.Required {
						b.WriteString(errorStyle.Render(fmt.Sprintf("    ✗ %-18s (not found)", ts.Name)))
					} else {
						b.WriteString(inactiveStyle.Render(fmt.Sprintf("    ✗ %-18s (not found)", ts.Name)))
					}
				}
				b.WriteString("\n")
			}
			b.WriteString("\n")
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
