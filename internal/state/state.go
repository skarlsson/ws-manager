package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type WorkspaceState struct {
	Name          string `yaml:"name"`
	KittyWinID    int    `yaml:"kitty_win_id,omitempty"`
	KittyPID      int    `yaml:"kitty_pid,omitempty"`
	ZellijSession string `yaml:"zellij_session,omitempty"`
	Active        bool   `yaml:"active"`
	HomeX         int    `yaml:"home_x"`        // original window X position
	HomeY         int    `yaml:"home_y"`        // original window Y position
	HomeCaptured  bool   `yaml:"home_captured"` // true once positions are captured
	Remote        bool   `yaml:"remote,omitempty"`
	Host          string `yaml:"host,omitempty"`
}

func stateDir() string {
	dir := os.Getenv("XDG_STATE_HOME")
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), ".local", "state")
	}
	return filepath.Join(dir, "ws-manager")
}

func statePath(name string) string {
	return filepath.Join(stateDir(), name+".yaml")
}

func Load(name string) (WorkspaceState, error) {
	var s WorkspaceState
	data, err := os.ReadFile(statePath(name))
	if err != nil {
		if os.IsNotExist(err) {
			return WorkspaceState{Name: name}, nil
		}
		return s, fmt.Errorf("reading state for %q: %w", name, err)
	}
	if err := yaml.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("parsing state for %q: %w", name, err)
	}
	return s, nil
}

func Save(s WorkspaceState) error {
	if err := os.MkdirAll(stateDir(), 0755); err != nil {
		return fmt.Errorf("creating state dir: %w", err)
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	return os.WriteFile(statePath(s.Name), data, 0644)
}

func Remove(name string) error {
	return os.Remove(statePath(name))
}

func focusedPath() string {
	return filepath.Join(stateDir(), ".focused")
}

func LoadFocused() string {
	data, err := os.ReadFile(focusedPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func SaveFocused(name string) {
	os.MkdirAll(stateDir(), 0755)
	os.WriteFile(focusedPath(), []byte(name), 0644)
}

type FocusPosition struct {
	X          int  `yaml:"x"`
	Y          int  `yaml:"y"`
	Captured   bool `yaml:"captured"`
	OffsetX    int  `yaml:"offset_x"`    // getwindowgeometry vs windowmove offset
	OffsetY    int  `yaml:"offset_y"`
	Calibrated bool `yaml:"calibrated"`
}

func focusPosPath() string {
	return filepath.Join(stateDir(), ".focus-position")
}

func LoadFocusPosition() FocusPosition {
	var fp FocusPosition
	data, err := os.ReadFile(focusPosPath())
	if err != nil {
		return fp
	}
	yaml.Unmarshal(data, &fp)
	return fp
}

func SaveFocusPosition(x, y int) {
	fp := LoadFocusPosition()
	fp.X = x
	fp.Y = y
	fp.Captured = true
	os.MkdirAll(stateDir(), 0755)
	data, _ := yaml.Marshal(fp)
	os.WriteFile(focusPosPath(), data, 0644)
}

func SaveFocusOffset(dx, dy int) {
	fp := LoadFocusPosition()
	fp.OffsetX = dx
	fp.OffsetY = dy
	fp.Calibrated = true
	os.MkdirAll(stateDir(), 0755)
	data, _ := yaml.Marshal(fp)
	os.WriteFile(focusPosPath(), data, 0644)
}

func rotateIndexPath() string {
	return filepath.Join(stateDir(), ".rotate-index")
}

func LoadRotateIndex() int {
	data, err := os.ReadFile(rotateIndexPath())
	if err != nil {
		return -1
	}
	var idx int
	fmt.Sscanf(string(data), "%d", &idx)
	return idx
}

func SaveRotateIndex(idx int) {
	os.MkdirAll(stateDir(), 0755)
	os.WriteFile(rotateIndexPath(), []byte(fmt.Sprintf("%d", idx)), 0644)
}

// ListActive returns workspace states that are active and have a matching
// workspace config file. Stale state files without configs are automatically
// cleaned up.
func ListActive() ([]WorkspaceState, error) {
	entries, err := os.ReadDir(stateDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var states []WorkspaceState
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		if !workspaceConfigExists(name) {
			_ = Remove(name)
			continue
		}
		s, err := Load(name)
		if err != nil {
			continue
		}
		if s.Active {
			states = append(states, s)
		}
	}
	return states, nil
}

func workspaceConfigExists(name string) bool {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	path := filepath.Join(dir, "ws-manager", "workspaces", name+".yaml")
	_, err := os.Stat(path)
	return err == nil
}
