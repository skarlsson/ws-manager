package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Keybinding struct {
	Binding string `yaml:"binding"` // GNOME keybinding string, e.g. "<super>r"
	Command string `yaml:"command"` // ws subcommand name, e.g. "rotate"
}

type GlobalConfig struct {
	DefaultLayout    string            `yaml:"default_layout"`
	DefaultShell     string            `yaml:"default_shell"`
	MonitorMapping   map[string]string `yaml:"monitor_mapping,omitempty"`
	WorkMonitor      string            `yaml:"work_monitor"`
	WorkspaceBaseDir string            `yaml:"workspace_base_dir,omitempty"`
	Keybindings      []Keybinding      `yaml:"keybindings,omitempty"`
}

func DefaultKeybindings() []Keybinding {
	return []Keybinding{
		{Binding: "<super>r", Command: "rotate"},
		{Binding: "<super>u", Command: "unfocus"},
	}
}

func DefaultGlobalConfig() GlobalConfig {
	home, _ := os.UserHomeDir()
	return GlobalConfig{
		DefaultLayout:    "default",
		DefaultShell:     "bash",
		WorkspaceBaseDir: filepath.Join(home, "dev"),
	}
}

func ConfigDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "workshell")
}

func WorkspacesDir() string {
	return filepath.Join(ConfigDir(), "workspaces")
}

func LayoutsDir() string {
	return filepath.Join(ConfigDir(), "layouts")
}

func GlobalConfigPath() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

func LoadGlobalConfig() (GlobalConfig, error) {
	cfg := DefaultGlobalConfig()
	data, err := os.ReadFile(GlobalConfigPath())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("reading global config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parsing global config: %w", err)
	}
	return cfg, nil
}

func SaveGlobalConfig(cfg GlobalConfig) error {
	if err := os.MkdirAll(ConfigDir(), 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling global config: %w", err)
	}
	return os.WriteFile(GlobalConfigPath(), data, 0644)
}

func EnsureDirs() error {
	dirs := []string{ConfigDir(), WorkspacesDir(), LayoutsDir()}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", d, err)
		}
	}
	return nil
}
