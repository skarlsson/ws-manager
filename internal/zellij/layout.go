package zellij

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/skarlsson/workshell/internal/config"
)

const defaultLayoutTemplate = `layout {
    pane size=1 borderless=true {
        plugin location="tab-bar"
    }
    pane split_direction="Vertical" {
        pane size="60%" command="claude"
        pane split_direction="Horizontal" {
            pane name="build"
            pane name="shell"
        }
    }
    pane size=1 borderless=true {
        plugin location="status-bar"
    }
}
`

const noclaudeLayoutTemplate = `layout {
    pane size=1 borderless=true {
        plugin location="tab-bar"
    }
    pane split_direction="Vertical" {
        pane name="editor"
        pane split_direction="Horizontal" {
            pane name="build"
            pane name="shell"
        }
    }
    pane size=1 borderless=true {
        plugin location="status-bar"
    }
}
`

func GenerateLayout(ws config.Workspace) (string, error) {
	layoutDir := config.LayoutsDir()
	if err := os.MkdirAll(layoutDir, 0755); err != nil {
		return "", fmt.Errorf("creating layouts dir: %w", err)
	}

	layoutPath := filepath.Join(layoutDir, ws.Name+".kdl")

	var content string
	if ws.AutoClaude {
		content = defaultLayoutTemplate
	} else {
		content = noclaudeLayoutTemplate
	}

	if err := os.WriteFile(layoutPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("writing layout: %w", err)
	}
	return layoutPath, nil
}

func LayoutPath(ws config.Workspace) string {
	// Check for workspace-specific layout first
	specific := filepath.Join(config.LayoutsDir(), ws.Name+".kdl")
	if _, err := os.Stat(specific); err == nil {
		return specific
	}
	// Check for named layout
	named := filepath.Join(config.LayoutsDir(), ws.Layout+".kdl")
	if _, err := os.Stat(named); err == nil {
		return named
	}
	return ""
}

func SessionName(wsName string) string {
	return "ws-" + strings.ReplaceAll(wsName, " ", "-")
}
