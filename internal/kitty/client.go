package kitty

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"time"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/skarlsson/ws-manager/internal/deps"
)

func SocketPath(wsName string) string {
	return fmt.Sprintf("/tmp/kitty-ws-%s", wsName)
}

type kittyWindow struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	PID   int    `json:"pid"`
}

type kittyTab struct {
	ID      int           `json:"id"`
	Windows []kittyWindow `json:"windows"`
}

type kittyOSWindow struct {
	ID             int        `json:"id"`
	PlatformWinID  int        `json:"platform_window_id"`
	Tabs           []kittyTab `json:"tabs"`
}

func remoteCmd(socket string, args ...string) (string, error) {
	fullArgs := append([]string{"@", "--to", "unix:" + socket}, args...)
	cmd := exec.Command("kitty", fullArgs...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("kitty @ %s: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

// SocketAlive checks if a kitty instance is running by connecting to its Unix socket.
// More reliable than PID-based checks — the socket only accepts connections when
// kitty is actually alive and listening.
func SocketAlive(wsName string) bool {
	socket := SocketPath(wsName)
	conn, err := net.DialTimeout("unix", socket, 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// IsAlive checks if a kitty instance is running, using socket check first (reliable),
// falling back to PID check.
func IsAlive(wsName string, pid int) bool {
	if SocketAlive(wsName) {
		return true
	}
	return IsRunning(pid)
}

// Launch starts a new kitty instance for a workspace.
// Returns the PID of the kitty process.
func Launch(wsName, cwd, title string) (int, error) {
	socket := SocketPath(wsName)
	// Remove stale socket from previous instance
	os.Remove(socket)

	cmd := exec.Command("kitty",
		"--listen-on", "unix:"+socket,
		"--directory", cwd,
		"--title", title,
		"--override", "allow_remote_control=yes",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	// Clean environment: remove vars that prevent tools like claude from starting
	env := os.Environ()
	cleanEnv := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "CLAUDECODE=") {
			continue
		}
		cleanEnv = append(cleanEnv, e)
	}
	cmd.Env = cleanEnv

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting kitty: %w", err)
	}

	pid := cmd.Process.Pid
	// Release the process so init adopts it — no zombies
	cmd.Process.Release()

	return pid, nil
}

// PlatformWindowID returns the X11/XWayland window ID for a workspace's kitty instance.
func PlatformWindowID(wsName string) (int, error) {
	socket := SocketPath(wsName)
	out, err := remoteCmd(socket, "ls")
	if err != nil {
		return 0, err
	}
	var osWindows []kittyOSWindow
	if err := json.Unmarshal([]byte(out), &osWindows); err != nil {
		return 0, fmt.Errorf("parsing kitty ls: %w", err)
	}
	if len(osWindows) == 0 {
		return 0, fmt.Errorf("no kitty OS windows found for %q", wsName)
	}
	return osWindows[0].PlatformWinID, nil
}

// Activate raises and focuses a workspace's kitty window using xdotool.
func Activate(wsName string) error {
	if !deps.HasTool("xdotool") {
		return fmt.Errorf("xdotool not found in PATH (required for window activation)")
	}
	winID, err := PlatformWindowID(wsName)
	if err != nil {
		return err
	}
	cmd := exec.Command("xdotool", "windowactivate", strconv.Itoa(winID))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("xdotool windowactivate: %w\n%s", err, string(out))
	}
	return nil
}

func SendText(socket string, text string) error {
	_, err := remoteCmd(socket, "send-text", text)
	return err
}

// SetTitle sets the OS window title for a workspace's kitty instance
// by setting _NET_WM_NAME via xprop (zellij intercepts escape sequences).
// Silently skips if xprop is not available (cosmetic feature).
func SetTitle(wsName, title string) error {
	if !deps.HasTool("xprop") {
		return nil
	}
	winID, err := PlatformWindowID(wsName)
	if err != nil {
		return err
	}
	id := strconv.Itoa(winID)
	out, err := exec.Command("xprop", "-id", id, "-f", "_NET_WM_NAME", "8u", "-set", "_NET_WM_NAME", title).CombinedOutput()
	if err != nil {
		return fmt.Errorf("xprop set _NET_WM_NAME: %w\n%s", err, string(out))
	}
	return nil
}

// LaunchRemote starts a new kitty instance for a remote workspace (no --directory).
// Returns the PID of the kitty process.
func LaunchRemote(wsName, title string) (int, error) {
	socket := SocketPath(wsName)
	// Remove stale socket from previous instance
	os.Remove(socket)

	cmd := exec.Command("kitty",
		"--listen-on", "unix:"+socket,
		"--title", title,
		"--override", "allow_remote_control=yes",
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	env := os.Environ()
	cleanEnv := make([]string, 0, len(env))
	for _, e := range env {
		if strings.HasPrefix(e, "CLAUDECODE=") {
			continue
		}
		cleanEnv = append(cleanEnv, e)
	}
	cmd.Env = cleanEnv

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("starting kitty: %w", err)
	}

	pid := cmd.Process.Pid
	cmd.Process.Release()

	return pid, nil
}

// KillProcess sends SIGTERM to a kitty process by PID.
func KillProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}

// IsRunning checks if a process with the given PID is still alive.
func IsRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	// Check /proc status to detect zombies — kill -0 succeeds on zombies
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "State:") {
			// Z = zombie, X = dead
			return !strings.Contains(line, "Z") && !strings.Contains(line, "X")
		}
	}
	return false
}
