package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type HostConfig struct {
	Name         string `yaml:"name"`
	SSH          string `yaml:"ssh"`           // SSH target: "dev-server" or "user@host"
	WorkspaceDir string `yaml:"workspace_dir"` // default workspace root on remote
}

type hostsFile struct {
	Hosts []HostConfig `yaml:"hosts"`
}

func HostsPath() string {
	return filepath.Join(ConfigDir(), "hosts.yaml")
}

func LoadHosts() ([]HostConfig, error) {
	data, err := os.ReadFile(HostsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading hosts config: %w", err)
	}
	var f hostsFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing hosts config: %w", err)
	}
	return f.Hosts, nil
}

func LoadHost(name string) (HostConfig, error) {
	hosts, err := LoadHosts()
	if err != nil {
		return HostConfig{}, err
	}
	for _, h := range hosts {
		if h.Name == name {
			return h, nil
		}
	}
	return HostConfig{}, fmt.Errorf("host %q not found", name)
}

func HostExists(name string) bool {
	_, err := LoadHost(name)
	return err == nil
}

func saveHosts(hosts []HostConfig) error {
	if err := os.MkdirAll(ConfigDir(), 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	f := hostsFile{Hosts: hosts}
	data, err := yaml.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshaling hosts: %w", err)
	}
	return os.WriteFile(HostsPath(), data, 0644)
}

func AddHost(h HostConfig) error {
	if HostExists(h.Name) {
		return fmt.Errorf("host %q already exists", h.Name)
	}
	hosts, _ := LoadHosts()
	hosts = append(hosts, h)
	return saveHosts(hosts)
}

func RemoveHost(name string) error {
	hosts, err := LoadHosts()
	if err != nil {
		return err
	}
	found := false
	filtered := make([]HostConfig, 0, len(hosts))
	for _, h := range hosts {
		if h.Name == name {
			found = true
			continue
		}
		filtered = append(filtered, h)
	}
	if !found {
		return fmt.Errorf("host %q not found", name)
	}
	return saveHosts(filtered)
}
