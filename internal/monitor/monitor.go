package monitor

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/skarlsson/workshell/internal/deps"
)

type MonitorInfo struct {
	Connector string
	X         int
	Y         int
	Width     int
	Height    int
	Primary   bool
}

// ListMonitors queries Mutter DisplayConfig via gdbus for monitor layout.
func ListMonitors() ([]MonitorInfo, error) {
	if !deps.HasTool("gdbus") {
		return nil, fmt.Errorf("gdbus not found in PATH (required for monitor detection on GNOME/Mutter)")
	}
	out, err := exec.Command("gdbus", "call",
		"--session",
		"--dest", "org.gnome.Mutter.DisplayConfig",
		"--object-path", "/org/gnome/Mutter/DisplayConfig",
		"--method", "org.gnome.Mutter.DisplayConfig.GetCurrentState",
	).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("querying display config: %w\n%s", err, string(out))
	}
	return parseDisplayConfig(string(out))
}

// parseDisplayConfig extracts monitor info from Mutter's GetCurrentState output.
// The logical monitors section looks like: [(x, y, scale, transform, primary, [connectors], {}), ...]
func parseDisplayConfig(output string) ([]MonitorInfo, error) {
	// Find the logical monitors array - it's the third top-level element
	// Format: (serial, [physical_monitors], [logical_monitors], {properties})
	// Each logical monitor: (x, y, scale, transform, primary, [(connector, vendor, product, serial)], {})

	var monitors []MonitorInfo

	// Find logical monitors section by looking for the pattern after physical monitors
	// Logical monitors start after "], [(" and each is "(x, y, scale, transform, primary/false, [(...)]"
	// Simple approach: find all "(x, y, scale, transform, true/false, [(" patterns

	// Split by logical monitor entries - they contain "true" or "false" for primary flag
	// and are followed by connector info in [(connector, ...)]
	idx := 0
	for {
		// Find next logical monitor entry: a number pair followed by scale and primary flag
		pos := strings.Index(output[idx:], "true, [(")
		posFalse := strings.Index(output[idx:], "false, [(")

		var nextPos int
		var isPrimary bool
		if pos >= 0 && (posFalse < 0 || pos < posFalse) {
			nextPos = idx + pos
			isPrimary = true
		} else if posFalse >= 0 {
			nextPos = idx + posFalse
			isPrimary = false
		} else {
			break
		}

		// Extract connector from [('DP-0', ...)]
		connStart := strings.Index(output[nextPos:], "[('")
		if connStart < 0 {
			connStart = strings.Index(output[nextPos:], "[(\"")
		}
		if connStart >= 0 {
			connStart += nextPos + 3 // skip [('
			connEnd := strings.IndexAny(output[connStart:], "'\"")
			if connEnd >= 0 {
				connector := output[connStart : connStart+connEnd]

				// Walk backwards from the primary flag to find x, y
				// Pattern before "true/false": "(x, y, scale, transform, "
				prefix := output[:nextPos]
				// Find the opening paren for this logical monitor
				parenPos := strings.LastIndex(prefix, "(")
				if parenPos >= 0 {
					between := output[parenPos+1 : nextPos]
					parts := strings.Split(between, ",")
					if len(parts) >= 4 {
						x, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
						y, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
						monitors = append(monitors, MonitorInfo{
							Connector: connector,
							X:         x,
							Y:         y,
							Primary:   isPrimary,
						})
					}
				}
			}
		}

		if isPrimary {
			idx = nextPos + 8 // skip "true, [("
		} else {
			idx = nextPos + 9 // skip "false, [("
		}
	}

	if len(monitors) == 0 {
		return nil, fmt.Errorf("no monitors found in display config")
	}
	return monitors, nil
}

// GetMonitor returns the MonitorInfo for a given connector name.
func GetMonitor(connector string) (MonitorInfo, error) {
	monitors, err := ListMonitors()
	if err != nil {
		return MonitorInfo{}, err
	}
	for _, m := range monitors {
		if m.Connector == connector {
			return m, nil
		}
	}
	return MonitorInfo{}, fmt.Errorf("monitor %q not found", connector)
}

