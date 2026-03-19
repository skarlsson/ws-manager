package cmd

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/skarlsson/workshell/internal/config"
)

type editField int

const (
	editFieldAuth editField = iota
)

type editWSModel struct {
	ws        config.Workspace
	field     editField
	cursor    int
	cancelled bool
	done      bool
	message   string
}

func newEditWSModel(ws config.Workspace) editWSModel {
	cursor := 0
	if ws.ClaudeAuth == "anthropic" {
		cursor = 1
	}
	return editWSModel{
		ws:     ws,
		field:  editFieldAuth,
		cursor: cursor,
	}
}

func (m editWSModel) Init() tea.Cmd { return nil }

func (m editWSModel) Update(msg tea.Msg) (editWSModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.cancelled = true
			return m, nil
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor < 1 {
				m.cursor++
			}
		case "enter":
			if m.cursor == 0 {
				m.ws.ClaudeAuth = ""
			} else {
				m.ws.ClaudeAuth = "anthropic"
			}
			if err := config.SaveWorkspace(m.ws); err != nil {
				m.message = fmt.Sprintf("Save failed: %v", err)
				return m, nil
			}
			m.done = true
			return m, nil
		}
	}
	return m, nil
}

func (m editWSModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(fmt.Sprintf("Edit workspace: %s", m.ws.Name)))
	b.WriteString("\n\n")

	b.WriteString(normalStyle.Render("  Claude authentication:"))
	b.WriteString("\n\n")
	authOpts := []string{"Default (inherit environment)", "Anthropic (OAuth, strips corporate env)"}
	for i, opt := range authOpts {
		if i == m.cursor {
			b.WriteString(selectedStyle.Render(fmt.Sprintf("  > %s", opt)))
		} else {
			b.WriteString(normalStyle.Render(fmt.Sprintf("    %s", opt)))
		}
		b.WriteString("\n")
	}

	if m.message != "" {
		b.WriteString("\n")
		b.WriteString(errorStyle.Render("  " + m.message))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("  ↑/↓: select  Enter: save  Esc: cancel"))
	b.WriteString("\n")
	return b.String()
}
