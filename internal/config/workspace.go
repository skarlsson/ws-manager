package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Workspace struct {
	Name          string   `yaml:"name"`
	Dir           string   `yaml:"dir"`
	DefaultBranch string   `yaml:"default_branch"`
	CurrentTask   string   `yaml:"current_task,omitempty"`
	Layout        string   `yaml:"layout"`
	AutoClaude    bool     `yaml:"auto_claude"`
	SetupCommands []string `yaml:"setup_commands,omitempty"`
	Skills        []string `yaml:"skills,omitempty"`
	Host          string   `yaml:"host,omitempty"` // references hosts.yaml entry for remote workspaces
}

func (w Workspace) IsRemote() bool { return w.Host != "" }

func DefaultWorkspace(name, dir string) Workspace {
	return Workspace{
		Name:          name,
		Dir:           dir,
		DefaultBranch: "main",
		Layout:        "default",
		AutoClaude:    true,
	}
}

func WorkspacePath(name string) string {
	return filepath.Join(WorkspacesDir(), name+".yaml")
}

func LoadWorkspace(name string) (Workspace, error) {
	var ws Workspace
	data, err := os.ReadFile(WorkspacePath(name))
	if err != nil {
		return ws, fmt.Errorf("reading workspace %q: %w", name, err)
	}
	if err := yaml.Unmarshal(data, &ws); err != nil {
		return ws, fmt.Errorf("parsing workspace %q: %w", name, err)
	}
	return ws, nil
}

func SaveWorkspace(ws Workspace) error {
	if err := os.MkdirAll(WorkspacesDir(), 0755); err != nil {
		return fmt.Errorf("creating workspaces dir: %w", err)
	}
	data, err := yaml.Marshal(ws)
	if err != nil {
		return fmt.Errorf("marshaling workspace: %w", err)
	}
	return os.WriteFile(WorkspacePath(ws.Name), data, 0644)
}

func ListWorkspaces() ([]Workspace, error) {
	entries, err := os.ReadDir(WorkspacesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading workspaces dir: %w", err)
	}
	var workspaces []Workspace
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		ws, err := LoadWorkspace(name)
		if err != nil {
			continue
		}
		workspaces = append(workspaces, ws)
	}
	return workspaces, nil
}

func (w Workspace) Validate() error {
	if w.Name == "" {
		return fmt.Errorf("workspace name is required")
	}
	if w.Dir == "" {
		return fmt.Errorf("workspace dir is required")
	}
	if w.Layout == "" {
		return fmt.Errorf("workspace layout is required")
	}
	if w.Host != "" && !HostExists(w.Host) {
		return fmt.Errorf("host %q not found in hosts.yaml", w.Host)
	}
	return nil
}

func DeleteWorkspace(name string) error {
	return os.Remove(WorkspacePath(name))
}

func WorkspaceExists(name string) bool {
	_, err := os.Stat(WorkspacePath(name))
	return err == nil
}
