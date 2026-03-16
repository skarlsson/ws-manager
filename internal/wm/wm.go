package wm

import (
	"os"
	"sync"
)

// Manager abstracts window management operations across display servers.
type Manager interface {
	Move(wsName string, x, y int) error
	Activate(wsName string) error
	Minimize(wsName string) error
	GetPosition(wsName string) (x, y int, err error)
	IsMaximized(wsName string) bool
	Maximize(wsName string) error
	Unmaximize(wsName string) error
	SetTitle(wsName string, title string) error
}

// IsWayland returns true if the current session is running under Wayland.
func IsWayland() bool {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return true
	}
	return os.Getenv("XDG_SESSION_TYPE") == "wayland"
}

// IsHeadless returns true when there's no display server at all (remote/SSH sessions).
func IsHeadless() bool {
	return os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == ""
}

var (
	defaultMgr  Manager
	defaultOnce sync.Once
)

// Default returns the auto-detected window manager backend.
func Default() Manager {
	defaultOnce.Do(func() {
		if IsHeadless() {
			defaultMgr = &nopManager{}
		} else if IsWayland() {
			defaultMgr = &gnomeManager{}
		} else {
			defaultMgr = &x11Manager{}
		}
	})
	return defaultMgr
}
