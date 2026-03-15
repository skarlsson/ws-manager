package deps

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type ToolStatus struct {
	Name     string
	Required bool
	Found    bool
	Path     string
	Note     string
	Remote   bool // needed on remote side
}

type toolDef struct {
	Name     string
	Required bool
	Note     string
	Remote   bool // needed when running as remote server
}

var tools = []toolDef{
	{"kitty", true, "terminal emulator", false},
	{"zellij", true, "terminal multiplexer", true},
	{"claude", true, "AI coding assistant", true},
	{"git", true, "branch/task management", true},
	{"kitty-terminfo", true, "terminfo for xterm-kitty", true},
	{"xdotool", false, "window move/focus/minimize", false},
	{"xprop", false, "window title updates", false},
	{"gdbus", false, "monitor detection (GNOME/Mutter)", false},
}

// CheckAll returns the status of all required and optional tools.
func CheckAll() []ToolStatus {
	return checkTools(false)
}

// CheckRemote returns the status of tools needed on a remote server.
func CheckRemote() []ToolStatus {
	return checkTools(true)
}

func checkTools(remoteOnly bool) []ToolStatus {
	var results []ToolStatus
	for _, t := range tools {
		if remoteOnly && !t.Remote {
			continue
		}
		ts := ToolStatus{
			Name:     t.Name,
			Required: t.Required,
			Note:     t.Note,
			Remote:   t.Remote,
		}
		if t.Name == "kitty-terminfo" {
			ts.Found = hasKittyTerminfo()
			if ts.Found {
				ts.Path = "(terminfo db)"
			}
		} else if path, err := exec.LookPath(t.Name); err == nil {
			ts.Found = true
			ts.Path = path
		}
		results = append(results, ts)
	}
	return results
}

func hasKittyTerminfo() bool {
	cmd := exec.Command("infocmp", "xterm-kitty")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// CheckRequired verifies that all required tools (kitty, zellij, git) are in PATH.
func CheckRequired() error {
	var missing []string
	for _, t := range tools {
		if !t.Required {
			continue
		}
		if _, err := exec.LookPath(t.Name); err != nil {
			missing = append(missing, t.Name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("required tools not found in PATH: %s\nRun 'ws doctor' for details or 'ws deps install' to install", strings.Join(missing, ", "))
	}
	return nil
}

// HasTool checks if a single tool is available in PATH (or installed for special cases).
func HasTool(name string) bool {
	if name == "kitty-terminfo" {
		return hasKittyTerminfo()
	}
	_, err := exec.LookPath(name)
	return err == nil
}

// Install attempts to install a missing tool. Returns nil if already installed.
func Install(name string) error {
	if HasTool(name) {
		return nil
	}

	switch name {
	case "zellij":
		return installZellij()
	case "claude":
		return installClaude()
	case "kitty-terminfo":
		return installKittyTerminfo()
	default:
		return fmt.Errorf("%s: no auto-install available — install via your package manager", name)
	}
}

func installZellij() error {
	arch := runtime.GOARCH
	var zellijArch string
	switch arch {
	case "amd64":
		zellijArch = "x86_64-unknown-linux-musl"
	case "arm64":
		zellijArch = "aarch64-unknown-linux-musl"
	default:
		return fmt.Errorf("unsupported architecture for zellij: %s", arch)
	}

	binDir := localBinDir()
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("creating %s: %w", binDir, err)
	}

	url := fmt.Sprintf("https://github.com/zellij-org/zellij/releases/latest/download/zellij-%s.tar.gz", zellijArch)

	// Download to temp file then extract — avoids shell interpolation issues
	tmpFile, err := os.CreateTemp("", "zellij-*.tar.gz")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	dl := exec.Command("curl", "-fsSL", "-L", "-o", tmpPath, url)
	dl.Stdout = os.Stdout
	dl.Stderr = os.Stderr
	if err := dl.Run(); err != nil {
		return fmt.Errorf("downloading zellij: %w", err)
	}

	extract := exec.Command("tar", "xzf", tmpPath, "-C", binDir)
	extract.Stdout = os.Stdout
	extract.Stderr = os.Stderr
	if err := extract.Run(); err != nil {
		return fmt.Errorf("extracting zellij: %w", err)
	}

	return nil
}

func installClaude() error {
	cmd := exec.Command("bash", "-c", "curl -fsSL https://claude.ai/install.sh | bash")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installing claude: %w", err)
	}
	return nil
}

func installKittyTerminfo() error {
	// Download kitty's terminfo and compile it into ~/.terminfo
	cmd := exec.Command("bash", "-c",
		`mkdir -p ~/.terminfo && curl -fsSL https://raw.githubusercontent.com/kovidgoyal/kitty/master/terminfo/kitty.terminfo | tic -x -o ~/.terminfo /dev/stdin`)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installing kitty terminfo: %w", err)
	}
	return nil
}

func localBinDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

// InstallMissing installs all missing required tools that support auto-install.
// If remoteOnly is true, only installs tools needed on remote servers.
func InstallMissing(remoteOnly bool) []string {
	var installed []string
	for _, t := range tools {
		if remoteOnly && !t.Remote {
			continue
		}
		if !t.Required || HasTool(t.Name) {
			continue
		}
		if err := Install(t.Name); err != nil {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", t.Name, err)
		} else {
			installed = append(installed, t.Name)
		}
	}
	return installed
}
