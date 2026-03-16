package wm

// nopManager is a no-op backend for headless/remote systems with no display server.
type nopManager struct{}

func (m *nopManager) Move(string, int, int) error              { return nil }
func (m *nopManager) Activate(string) error                    { return nil }
func (m *nopManager) Minimize(string) error                    { return nil }
func (m *nopManager) GetPosition(string) (int, int, error)     { return 0, 0, nil }
func (m *nopManager) IsMaximized(string) bool                  { return false }
func (m *nopManager) Maximize(string) error                    { return nil }
func (m *nopManager) Unmaximize(string) error                  { return nil }
func (m *nopManager) SetTitle(string, string) error            { return nil }
