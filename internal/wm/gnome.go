package wm

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/skarlsson/workshell/internal/kitty"
	"github.com/skarlsson/workshell/internal/state"
)

const (
	wcDest   = "org.gnome.Shell"
	wcPath   = "/org/gnome/Shell/Extensions/Windows"
	wcIface  = "org.gnome.Shell.Extensions.Windows"
)

type gnomeManager struct{}

type wcWindow struct {
	ID    int `json:"id"`
	PID   int `json:"pid"`
	X     int `json:"x"`
	Y     int `json:"y"`
	Focus bool `json:"focus"`
}

// windowCall invokes a window-calls extension D-Bus method.
func windowCall(method string, args ...string) (string, error) {
	cmdArgs := []string{"call", "--session",
		"--dest", wcDest,
		"--object-path", wcPath,
		"--method", wcIface + "." + method,
	}
	cmdArgs = append(cmdArgs, args...)
	out, err := exec.Command("gdbus", cmdArgs...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("window-calls %s: %w\n%s", method, err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

// findWindowID finds the Mutter window ID for a workspace's kitty process.
func findWindowID(wsName string) (int, error) {
	st, err := state.Load(wsName)
	if err != nil {
		return 0, fmt.Errorf("loading state for %q: %w", wsName, err)
	}
	if st.KittyPID <= 0 {
		return 0, fmt.Errorf("no kitty PID for workspace %q", wsName)
	}

	out, err := windowCall("List")
	if err != nil {
		return 0, err
	}

	// gdbus wraps the result: ('json_string',)
	jsonStr := extractGdbusString(out)

	var windows []wcWindow
	if err := json.Unmarshal([]byte(jsonStr), &windows); err != nil {
		return 0, fmt.Errorf("parsing window list: %w", err)
	}

	for _, w := range windows {
		if w.PID == st.KittyPID {
			return w.ID, nil
		}
	}
	return 0, fmt.Errorf("no window found for PID %d (workspace %q)", st.KittyPID, wsName)
}

// extractGdbusString extracts the string value from gdbus output like ('...value...',)
func extractGdbusString(out string) string {
	if i := strings.Index(out, "'"); i >= 0 {
		if j := strings.LastIndex(out, "'"); j > i {
			return out[i+1 : j]
		}
	}
	return out
}

func (m *gnomeManager) Move(wsName string, x, y int) error {
	wid, err := findWindowID(wsName)
	if err != nil {
		return err
	}
	_, err = windowCall("Move", fmt.Sprintf("uint32 %d", wid), fmt.Sprintf("int32 %d", x), fmt.Sprintf("int32 %d", y))
	return err
}

func (m *gnomeManager) Activate(wsName string) error {
	wid, err := findWindowID(wsName)
	if err != nil {
		return err
	}
	_, err = windowCall("Activate", fmt.Sprintf("uint32 %d", wid))
	return err
}

func (m *gnomeManager) Minimize(wsName string) error {
	wid, err := findWindowID(wsName)
	if err != nil {
		return err
	}
	_, err = windowCall("Minimize", fmt.Sprintf("uint32 %d", wid))
	return err
}

func (m *gnomeManager) GetPosition(wsName string) (int, int, error) {
	wid, err := findWindowID(wsName)
	if err != nil {
		return 0, 0, err
	}
	out, err := windowCall("GetFrameRect", fmt.Sprintf("uint32 %d", wid))
	if err != nil {
		return 0, 0, err
	}

	jsonStr := extractGdbusString(out)

	var rect struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &rect); err != nil {
		// Fallback: try parsing "x,y" format
		parts := strings.SplitN(jsonStr, ",", 2)
		if len(parts) == 2 {
			rect.X, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
			rect.Y, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
		} else {
			return 0, 0, fmt.Errorf("parsing frame rect: %w", err)
		}
	}
	return rect.X, rect.Y, nil
}

func (m *gnomeManager) IsMaximized(wsName string) bool {
	wid, err := findWindowID(wsName)
	if err != nil {
		return false
	}
	out, err := windowCall("Details", fmt.Sprintf("uint32 %d", wid))
	if err != nil {
		return false
	}
	jsonStr := extractGdbusString(out)
	var details struct {
		Maximized int `json:"maximized"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &details); err != nil {
		return false
	}
	return details.Maximized > 0
}

func (m *gnomeManager) Maximize(wsName string) error {
	wid, err := findWindowID(wsName)
	if err != nil {
		return err
	}
	_, err = windowCall("Maximize", fmt.Sprintf("uint32 %d", wid))
	return err
}

func (m *gnomeManager) Unmaximize(wsName string) error {
	wid, err := findWindowID(wsName)
	if err != nil {
		return err
	}
	_, err = windowCall("Unmaximize", fmt.Sprintf("uint32 %d", wid))
	return err
}

func (m *gnomeManager) SetTitle(wsName, title string) error {
	socket := kitty.SocketPath(wsName)
	_, err := kitty.RemoteCmd(socket, "set-window-title", "--match", "all", title)
	return err
}

