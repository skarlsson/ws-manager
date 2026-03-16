package deps

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

//go:embed embed/window-calls.zip
var windowCallsZip []byte

type ToolStatus struct {
	Name     string
	Required bool
	Found    bool
	Path     string
	Note     string
	Remote   bool   // needed on remote side
	Category string // "core", "x11", "wayland"
}

type toolDef struct {
	Name        string
	Required    bool
	Note        string
	Remote      bool // needed when running as remote server
	X11Only     bool // only required on X11 sessions
	WaylandOnly bool // only required on Wayland sessions
}

var tools = []toolDef{
	{"kitty", true, "terminal emulator", false, false, false},
	{"zellij", true, "terminal multiplexer", true, false, false},
	{"claude", true, "AI coding assistant", true, false, false},
	{"git", true, "branch/task management", true, false, false},
	{"kitty-terminfo", true, "terminfo for xterm-kitty", true, false, false},
	{"xdotool", true, "window move/focus/minimize (X11)", false, true, false},
	{"xprop", true, "window title updates (X11)", false, true, false},
	{"gdbus", true, "monitor/window management (GNOME/Wayland)", false, false, false},
	{"window-calls", true, "window management extension (Wayland)", false, false, true},
}

// SessionType returns a human-readable string for the detected display session.
func SessionType() string {
	if isHeadless() {
		return "headless"
	}
	if isWayland() {
		return "wayland"
	}
	return "x11"
}

// CheckAll returns the status of all required and optional tools.
func CheckAll() []ToolStatus {
	return checkTools(false)
}

// CheckRemote returns the status of tools needed on a remote server.
func CheckRemote() []ToolStatus {
	return checkTools(true)
}

func isWayland() bool {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return true
	}
	return os.Getenv("XDG_SESSION_TYPE") == "wayland"
}

func isHeadless() bool {
	return os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
}

func checkTools(remoteOnly bool) []ToolStatus {
	wayland := isWayland()
	headless := isHeadless()
	var results []ToolStatus
	for _, t := range tools {
		if remoteOnly && !t.Remote {
			continue
		}
		if headless && (t.X11Only || t.WaylandOnly) {
			continue
		}
		if t.X11Only && wayland {
			continue
		}
		if t.WaylandOnly && !wayland {
			continue
		}
		cat := "core"
		if t.X11Only {
			cat = "x11"
		} else if t.WaylandOnly {
			cat = "wayland"
		}
		ts := ToolStatus{
			Name:     t.Name,
			Required: t.Required,
			Note:     t.Note,
			Remote:   t.Remote,
			Category: cat,
		}
		if t.Name == "kitty-terminfo" {
			ts.Found = hasKittyTerminfo()
			if ts.Found {
				ts.Path = "(terminfo db)"
			}
		} else if t.Name == "window-calls" {
			ts.Found = hasWindowCalls()
			if ts.Found {
				ts.Path = "(gnome extension)"
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
	wayland := isWayland()
	headless := isHeadless()
	var missing []string
	for _, t := range tools {
		if !t.Required {
			continue
		}
		if headless && (t.X11Only || t.WaylandOnly) {
			continue
		}
		if t.X11Only && wayland {
			continue
		}
		if t.WaylandOnly && !wayland {
			continue
		}
		found := false
		switch t.Name {
		case "kitty-terminfo":
			found = hasKittyTerminfo()
		case "window-calls":
			found = hasWindowCalls()
		default:
			_, err := exec.LookPath(t.Name)
			found = err == nil
		}
		if !found {
			missing = append(missing, t.Name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("required tools not found in PATH: %s\nRun 'ws doctor' for details or 'ws deps install' to install", strings.Join(missing, ", "))
	}
	return nil
}

const windowCallsUUID = "window-calls@domandoman.xyz"

func hasWindowCalls() bool {
	out, err := exec.Command("gnome-extensions", "info", windowCallsUUID).CombinedOutput()
	if err != nil {
		return false
	}
	// Check it's enabled, not just installed
	return strings.Contains(string(out), "State: ACTIVE") ||
		strings.Contains(string(out), "State: ENABLED")
}

// HasTool checks if a single tool is available in PATH (or installed for special cases).
func HasTool(name string) bool {
	if name == "kitty-terminfo" {
		return hasKittyTerminfo()
	}
	if name == "window-calls" {
		return hasWindowCalls()
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
	case "xdotool", "gdbus":
		return installPackage(name)
	case "xprop":
		return installPackage("xprop")
	case "window-calls":
		return installWindowCalls()
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

// packageNames maps tool names to package names per package manager.
var packageNames = map[string]map[string]string{
	"xdotool": {"apt-get": "xdotool", "dnf": "xdotool", "pacman": "xdotool"},
	"xprop":   {"apt-get": "x11-utils", "dnf": "xorg-x11-utils", "pacman": "xorg-xprop"},
	"gdbus":   {"apt-get": "libglib2.0-bin", "dnf": "glib2", "pacman": "glib2"},
}

func detectPackageManager() string {
	for _, pm := range []string{"apt-get", "dnf", "pacman"} {
		if _, err := exec.LookPath(pm); err == nil {
			return pm
		}
	}
	return ""
}

func installPackage(name string) error {
	pm := detectPackageManager()
	if pm == "" {
		return fmt.Errorf("%s: no supported package manager found (apt-get, dnf, pacman)", name)
	}

	pkgMap, ok := packageNames[name]
	if !ok {
		return fmt.Errorf("%s: no package mapping available", name)
	}
	pkg, ok := pkgMap[pm]
	if !ok {
		return fmt.Errorf("%s: no package name for %s", name, pm)
	}

	var cmd *exec.Cmd
	switch pm {
	case "apt-get":
		cmd = exec.Command("sudo", "apt-get", "install", "-y", pkg)
	case "dnf":
		cmd = exec.Command("sudo", "dnf", "install", "-y", pkg)
	case "pacman":
		cmd = exec.Command("sudo", "pacman", "-S", "--noconfirm", pkg)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installing %s (%s): %w", name, pkg, err)
	}
	return nil
}

func installWindowCalls() error {
	if hasWindowCalls() {
		return nil
	}

	// Write embedded zip to temp file
	tmpFile, err := os.CreateTemp("", "window-calls-*.zip")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(windowCallsZip); err != nil {
		tmpFile.Close()
		return fmt.Errorf("writing extension zip: %w", err)
	}
	tmpFile.Close()

	install := exec.Command("gnome-extensions", "install", "--force", tmpPath)
	install.Stdout = os.Stdout
	install.Stderr = os.Stderr
	if err := install.Run(); err != nil {
		return fmt.Errorf("installing window-calls extension: %w", err)
	}

	// Enable via D-Bus (works immediately, unlike gnome-extensions CLI which
	// requires a shell restart to discover newly installed extensions)
	enable := exec.Command("gdbus", "call", "--session",
		"--dest", "org.gnome.Shell.Extensions",
		"--object-path", "/org/gnome/Shell/Extensions",
		"--method", "org.gnome.Shell.Extensions.EnableExtension",
		windowCallsUUID)
	out, err := enable.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "true") {
		fmt.Printf("Note: you may need to log out and back in, then run:\n  gnome-extensions enable %s\n", windowCallsUUID)
		return fmt.Errorf("enabling window-calls extension: %w\n%s", err, string(out))
	}

	return nil
}

func localBinDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

// InstallMissing installs all missing tools that support auto-install.
// If remoteOnly is true, only installs tools needed on remote servers.
func InstallMissing(remoteOnly bool) []string {
	wayland := isWayland()
	headless := isHeadless()
	var installed []string
	for _, t := range tools {
		if remoteOnly && !t.Remote {
			continue
		}
		if headless && (t.X11Only || t.WaylandOnly) {
			continue
		}
		if t.X11Only && wayland {
			continue
		}
		if t.WaylandOnly && !wayland {
			continue
		}
		if HasTool(t.Name) {
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
