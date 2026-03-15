package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/skarlsson/workshell/internal/config"
	"github.com/spf13/cobra"
)

var keybindingsCmd = &cobra.Command{
	Use:   "keybindings",
	Short: "Setup GNOME keybindings for workshell",
	Long:  "Registers custom GNOME keybindings for ws commands. Configurable via keybindings in config.yaml.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadGlobalConfig()
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}

		bindings := cfg.Keybindings
		if len(bindings) == 0 {
			bindings = config.DefaultKeybindings()
		}

		if err := applyKeybindings(bindings); err != nil {
			return err
		}

		fmt.Println("Keybindings configured:")
		for _, kb := range bindings {
			fmt.Printf("  %s → ws %s\n", kb.Binding, kb.Command)
		}
		return nil
	},
}

func applyKeybindings(bindings []config.Keybinding) error {
	if _, err := exec.LookPath("gsettings"); err != nil {
		return fmt.Errorf("gsettings not found — this command requires GNOME")
	}

	wsBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable path: %w", err)
	}

	schema := "org.gnome.settings-daemon.plugins.media-keys"
	keyPath := "/org/gnome/settings-daemon/plugins/media-keys/custom-keybindings"

	existing, err := gsettingsGet(schema, "custom-keybindings")
	if err != nil {
		return fmt.Errorf("reading existing keybindings: %w", err)
	}

	var newPaths []string
	for _, kb := range bindings {
		slot := 0
		for strings.Contains(existing, fmt.Sprintf("custom%d", slot)) || containsSlot(newPaths, slot) {
			slot++
		}

		slotPath := fmt.Sprintf("%s/custom%d/", keyPath, slot)
		subSchema := schema + ".custom-keybinding:" + slotPath

		if err := gsettingsSet(subSchema, "name", "ws "+kb.Command); err != nil {
			return fmt.Errorf("setting name for %s: %w", kb.Command, err)
		}
		if err := gsettingsSet(subSchema, "command", wsBin+" "+kb.Command); err != nil {
			return fmt.Errorf("setting command for %s: %w", kb.Command, err)
		}
		if err := gsettingsSet(subSchema, "binding", kb.Binding); err != nil {
			return fmt.Errorf("setting binding for %s: %w", kb.Command, err)
		}

		newPaths = append(newPaths, slotPath)
	}

	var newList string
	if existing == "@as []" {
		parts := make([]string, len(newPaths))
		for i, p := range newPaths {
			parts[i] = "'" + p + "'"
		}
		newList = "[" + strings.Join(parts, ", ") + "]"
	} else {
		trimmed := strings.TrimSuffix(existing, "]")
		parts := make([]string, len(newPaths))
		for i, p := range newPaths {
			parts[i] = "'" + p + "'"
		}
		newList = trimmed + ", " + strings.Join(parts, ", ") + "]"
	}
	return gsettingsSet(schema, "custom-keybindings", newList)
}

func containsSlot(paths []string, slot int) bool {
	needle := fmt.Sprintf("custom%d/", slot)
	for _, p := range paths {
		if strings.Contains(p, needle) {
			return true
		}
	}
	return false
}

func gsettingsGet(schema, key string) (string, error) {
	out, err := exec.Command("gsettings", "get", schema, key).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gsettingsSet(schema, key, value string) error {
	return exec.Command("gsettings", "set", schema, key, value).Run()
}

func init() {
	rootCmd.AddCommand(keybindingsCmd)
}
