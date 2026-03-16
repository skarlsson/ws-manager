package wm

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/skarlsson/workshell/internal/deps"
	"github.com/skarlsson/workshell/internal/kitty"
)

type x11Manager struct{}

func (m *x11Manager) Move(wsName string, x, y int) error {
	winID, err := kitty.PlatformWindowID(wsName)
	if err != nil {
		return err
	}
	if !deps.HasTool("xdotool") {
		return fmt.Errorf("xdotool not found in PATH (required for window management)")
	}
	out, err := exec.Command("xdotool", "windowmove", "--sync", strconv.Itoa(winID),
		strconv.Itoa(x), strconv.Itoa(y)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("xdotool windowmove: %w\n%s", err, string(out))
	}
	return nil
}

func (m *x11Manager) Activate(wsName string) error {
	winID, err := kitty.PlatformWindowID(wsName)
	if err != nil {
		return err
	}
	if !deps.HasTool("xdotool") {
		return fmt.Errorf("xdotool not found in PATH (required for window activation)")
	}
	out, err := exec.Command("xdotool", "windowactivate", strconv.Itoa(winID)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("xdotool windowactivate: %w\n%s", err, string(out))
	}
	return nil
}

func (m *x11Manager) Minimize(wsName string) error {
	winID, err := kitty.PlatformWindowID(wsName)
	if err != nil {
		return err
	}
	if !deps.HasTool("xdotool") {
		return fmt.Errorf("xdotool not found in PATH (required for window management)")
	}
	out, err := exec.Command("xdotool", "windowminimize", strconv.Itoa(winID)).CombinedOutput()
	if err != nil {
		return fmt.Errorf("xdotool windowminimize: %w\n%s", err, string(out))
	}
	return nil
}

func (m *x11Manager) GetPosition(wsName string) (int, int, error) {
	winID, err := kitty.PlatformWindowID(wsName)
	if err != nil {
		return 0, 0, err
	}
	if !deps.HasTool("xdotool") {
		return 0, 0, fmt.Errorf("xdotool not found in PATH (required for window management)")
	}
	out, err := exec.Command("xdotool", "getwindowgeometry", "--shell", strconv.Itoa(winID)).CombinedOutput()
	if err != nil {
		return 0, 0, fmt.Errorf("xdotool getwindowgeometry: %w\n%s", err, string(out))
	}
	var x, y int
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "X=") {
			x, _ = strconv.Atoi(strings.TrimPrefix(line, "X="))
		} else if strings.HasPrefix(line, "Y=") {
			y, _ = strconv.Atoi(strings.TrimPrefix(line, "Y="))
		}
	}
	return x, y, nil
}

func (m *x11Manager) IsMaximized(wsName string) bool {
	winID, err := kitty.PlatformWindowID(wsName)
	if err != nil {
		return false
	}
	if !deps.HasTool("xprop") {
		return false
	}
	out, err := exec.Command("xprop", "-id", strconv.Itoa(winID), "_NET_WM_STATE").CombinedOutput()
	if err != nil {
		return false
	}
	s := string(out)
	return strings.Contains(s, "_NET_WM_STATE_MAXIMIZED_HORZ") && strings.Contains(s, "_NET_WM_STATE_MAXIMIZED_VERT")
}

func (m *x11Manager) Maximize(wsName string) error {
	winID, err := kitty.PlatformWindowID(wsName)
	if err != nil {
		return err
	}
	if !deps.HasTool("xdotool") {
		return fmt.Errorf("xdotool not found in PATH")
	}
	out, err := exec.Command("xdotool", "windowsize", "--sync", strconv.Itoa(winID), "100%", "100%").CombinedOutput()
	if err != nil {
		return fmt.Errorf("xdotool windowsize: %w\n%s", err, string(out))
	}
	return nil
}

func (m *x11Manager) Unmaximize(wsName string) error {
	// wmctrl would be ideal, but xdotool doesn't have a direct unmaximize
	// For X11, moving the window effectively unmaximizes it
	return nil
}

func (m *x11Manager) SetTitle(wsName, title string) error {
	if !deps.HasTool("xprop") {
		return nil
	}
	winID, err := kitty.PlatformWindowID(wsName)
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

