package state

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestDirs(t *testing.T) (stateDir, configDir string) {
	t.Helper()
	tmp := t.TempDir()
	sd := filepath.Join(tmp, "state")
	cd := filepath.Join(tmp, "config", "workshell", "workspaces")
	os.MkdirAll(sd, 0755)
	os.MkdirAll(cd, 0755)
	t.Setenv("XDG_STATE_HOME", filepath.Join(tmp, "state", ".."))
	// stateDir() uses XDG_STATE_HOME + "workshell", so set it one level up
	t.Setenv("XDG_STATE_HOME", tmp)
	// We need stateDir() to return sd — but stateDir appends "workshell"
	// So create the workshell subdir
	wsStateDir := filepath.Join(tmp, "workshell")
	os.MkdirAll(wsStateDir, 0755)
	t.Setenv("XDG_STATE_HOME", tmp)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmp, "config"))
	return wsStateDir, cd
}

func TestSaveAndLoad(t *testing.T) {
	setupTestDirs(t)

	ws := WorkspaceState{
		Name:     "test-ws",
		KittyPID: 12345,
		Active:   true,
		HomeX:    100,
		HomeY:    200,
	}

	if err := Save(ws); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load("test-ws")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Name != ws.Name {
		t.Errorf("expected name %q, got %q", ws.Name, loaded.Name)
	}
	if loaded.KittyPID != ws.KittyPID {
		t.Errorf("expected PID %d, got %d", ws.KittyPID, loaded.KittyPID)
	}
	if loaded.HomeX != 100 || loaded.HomeY != 200 {
		t.Errorf("expected home (100, 200), got (%d, %d)", loaded.HomeX, loaded.HomeY)
	}
	if !loaded.Active {
		t.Error("expected active=true")
	}
}

func TestLoadNonExistent(t *testing.T) {
	setupTestDirs(t)

	loaded, err := Load("does-not-exist")
	if err != nil {
		t.Fatalf("expected no error for missing state, got: %v", err)
	}
	if loaded.Name != "does-not-exist" {
		t.Errorf("expected name %q, got %q", "does-not-exist", loaded.Name)
	}
	if loaded.Active {
		t.Error("expected active=false for non-existent state")
	}
}

func TestSaveAndRemove(t *testing.T) {
	setupTestDirs(t)

	ws := WorkspaceState{Name: "removeme", Active: true}
	Save(ws)

	if err := Remove("removeme"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	loaded, err := Load("removeme")
	if err != nil {
		t.Fatalf("Load after remove failed: %v", err)
	}
	if loaded.Active {
		t.Error("expected inactive after removal")
	}
}

func TestHomeCapturedFlag(t *testing.T) {
	setupTestDirs(t)

	ws := WorkspaceState{
		Name:         "captured",
		Active:       true,
		HomeX:        0,
		HomeY:        0,
		HomeCaptured: true,
	}
	Save(ws)

	loaded, err := Load("captured")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !loaded.HomeCaptured {
		t.Error("expected HomeCaptured=true")
	}
	if loaded.HomeX != 0 || loaded.HomeY != 0 {
		t.Errorf("expected (0,0), got (%d,%d)", loaded.HomeX, loaded.HomeY)
	}
}

func TestFocusedState(t *testing.T) {
	setupTestDirs(t)

	if f := LoadFocused(); f != "" {
		t.Errorf("expected empty focused, got %q", f)
	}

	SaveFocused("my-ws")
	if f := LoadFocused(); f != "my-ws" {
		t.Errorf("expected focused %q, got %q", "my-ws", f)
	}

	SaveFocused("")
	if f := LoadFocused(); f != "" {
		t.Errorf("expected empty focused after clear, got %q", f)
	}
}

func TestRotateIndex(t *testing.T) {
	setupTestDirs(t)

	if idx := LoadRotateIndex(); idx != -1 {
		t.Errorf("expected -1 for missing index, got %d", idx)
	}

	SaveRotateIndex(3)
	if idx := LoadRotateIndex(); idx != 3 {
		t.Errorf("expected 3, got %d", idx)
	}

	SaveRotateIndex(0)
	if idx := LoadRotateIndex(); idx != 0 {
		t.Errorf("expected 0, got %d", idx)
	}
}

func TestListActive_CleansStaleState(t *testing.T) {
	sd, cd := setupTestDirs(t)

	// Create a workspace config for "real-ws" but not for "stale-ws"
	os.WriteFile(filepath.Join(cd, "real-ws.yaml"), []byte("name: real-ws\ndir: /tmp\n"), 0644)

	// Create state files for both
	realState := WorkspaceState{Name: "real-ws", Active: true, KittyPID: 1}
	staleState := WorkspaceState{Name: "stale-ws", Active: true, KittyPID: 2}
	Save(realState)
	Save(staleState)

	// Verify both state files exist
	if _, err := os.Stat(filepath.Join(sd, "stale-ws.yaml")); err != nil {
		t.Fatalf("stale state file should exist before ListActive")
	}

	active, err := ListActive()
	if err != nil {
		t.Fatalf("ListActive failed: %v", err)
	}

	// Only real-ws should be returned
	if len(active) != 1 {
		t.Fatalf("expected 1 active workspace, got %d", len(active))
	}
	if active[0].Name != "real-ws" {
		t.Errorf("expected active workspace %q, got %q", "real-ws", active[0].Name)
	}

	// Stale state file should have been cleaned up
	if _, err := os.Stat(filepath.Join(sd, "stale-ws.yaml")); !os.IsNotExist(err) {
		t.Error("stale state file should have been removed")
	}
}

func TestListActive_SkipsInactive(t *testing.T) {
	_, cd := setupTestDirs(t)

	os.WriteFile(filepath.Join(cd, "inactive-ws.yaml"), []byte("name: inactive-ws\ndir: /tmp\n"), 0644)

	ws := WorkspaceState{Name: "inactive-ws", Active: false}
	Save(ws)

	active, err := ListActive()
	if err != nil {
		t.Fatalf("ListActive failed: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("expected 0 active workspaces, got %d", len(active))
	}
}

func TestListActive_EmptyStateDir(t *testing.T) {
	setupTestDirs(t)

	active, err := ListActive()
	if err != nil {
		t.Fatalf("ListActive failed: %v", err)
	}
	if len(active) != 0 {
		t.Errorf("expected 0, got %d", len(active))
	}
}

func TestDetachedField(t *testing.T) {
	setupTestDirs(t)

	ws := WorkspaceState{
		Name:     "detach-test",
		Active:   true,
		KittyPID: 1234,
		Detached: true,
	}
	if err := Save(ws); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load("detach-test")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if !loaded.Detached {
		t.Error("expected Detached=true after load")
	}
	if !loaded.Active {
		t.Error("expected Active=true after load")
	}
}

func TestDetachedOmitempty(t *testing.T) {
	setupTestDirs(t)

	ws := WorkspaceState{
		Name:     "no-detach",
		Active:   true,
		KittyPID: 5678,
		Detached: false,
	}
	Save(ws)

	data, err := os.ReadFile(statePath("no-detach"))
	if err != nil {
		t.Fatalf("reading state file: %v", err)
	}
	if strings.Contains(string(data), "detached") {
		t.Errorf("Detached=false should be omitted from YAML, got:\n%s", data)
	}
}

func TestListActive_RemoteStatePreserved(t *testing.T) {
	sd, _ := setupTestDirs(t)

	// Save a remote state file with host@name key — no local config exists
	remoteState := WorkspaceState{
		Name:   "myhost@myws",
		Active: true,
		Remote: true,
		Host:   "myhost",
	}
	Save(remoteState)

	// Verify it exists before ListActive
	if _, err := os.Stat(filepath.Join(sd, "myhost@myws.yaml")); err != nil {
		t.Fatalf("remote state file should exist before ListActive")
	}

	_, err := ListActive()
	if err != nil {
		t.Fatalf("ListActive failed: %v", err)
	}

	// Remote state file must NOT be deleted
	if _, err := os.Stat(filepath.Join(sd, "myhost@myws.yaml")); err != nil {
		t.Error("remote state file was incorrectly deleted by ListActive")
	}
}

func TestListActive_RemoteStateReturned(t *testing.T) {
	setupTestDirs(t)

	remoteState := WorkspaceState{
		Name:   "myhost@myws",
		Active: true,
		Remote: true,
		Host:   "myhost",
	}
	Save(remoteState)

	active, err := ListActive()
	if err != nil {
		t.Fatalf("ListActive failed: %v", err)
	}

	found := false
	for _, s := range active {
		if s.Name == "myhost@myws" {
			found = true
			if !s.Remote {
				t.Error("expected Remote=true")
			}
			if s.Host != "myhost" {
				t.Errorf("expected Host=myhost, got %q", s.Host)
			}
		}
	}
	if !found {
		t.Error("remote active workspace not returned by ListActive")
	}
}

func TestStateTransitions(t *testing.T) {
	tests := []struct {
		name   string
		setup  *WorkspaceState // nil = no prior state
		apply  WorkspaceState
		remove bool // if true, remove instead of save
		expect func(t *testing.T, wsName string)
	}{
		{
			name:  "inactive to active local",
			setup: nil,
			apply: WorkspaceState{Name: "trans-ws", Active: true, KittyPID: 100},
			expect: func(t *testing.T, wsName string) {
				s, _ := Load(wsName)
				if !s.Active || s.KittyPID != 100 {
					t.Errorf("expected active with PID 100, got active=%v pid=%d", s.Active, s.KittyPID)
				}
			},
		},
		{
			name:  "active to detached",
			setup: &WorkspaceState{Name: "trans-ws", Active: true, KittyPID: 100},
			apply: WorkspaceState{Name: "trans-ws", Active: true, KittyPID: 100, Detached: true},
			expect: func(t *testing.T, wsName string) {
				s, _ := Load(wsName)
				if !s.Active || !s.Detached || s.KittyPID != 100 {
					t.Errorf("expected active+detached PID 100, got active=%v detached=%v pid=%d", s.Active, s.Detached, s.KittyPID)
				}
			},
		},
		{
			name:  "detached to reattached",
			setup: &WorkspaceState{Name: "trans-ws", Active: true, KittyPID: 100, Detached: true},
			apply: WorkspaceState{Name: "trans-ws", Active: true, KittyPID: 100, Detached: false},
			expect: func(t *testing.T, wsName string) {
				s, _ := Load(wsName)
				if !s.Active || s.Detached {
					t.Errorf("expected active+not-detached, got active=%v detached=%v", s.Active, s.Detached)
				}
			},
		},
		{
			name:   "active to killed",
			setup:  &WorkspaceState{Name: "trans-ws", Active: true, KittyPID: 100},
			remove: true,
			expect: func(t *testing.T, wsName string) {
				s, _ := Load(wsName)
				if s.Active {
					t.Error("expected inactive after kill")
				}
			},
		},
		{
			name:  "inactive to active remote",
			setup: nil,
			apply: WorkspaceState{Name: "host@trans-ws", Active: true, Remote: true, Host: "host"},
			expect: func(t *testing.T, wsName string) {
				s, _ := Load(wsName)
				if !s.Active || !s.Remote || s.Host != "host" {
					t.Errorf("expected active remote, got active=%v remote=%v host=%q", s.Active, s.Remote, s.Host)
				}
			},
		},
		{
			name:  "remote active to detached",
			setup: &WorkspaceState{Name: "host@trans-ws", Active: true, Remote: true, Host: "host", KittyPID: 200},
			apply: WorkspaceState{Name: "host@trans-ws", Active: true, Remote: true, Host: "host", KittyPID: 0, Detached: true},
			expect: func(t *testing.T, wsName string) {
				s, _ := Load(wsName)
				if !s.Detached || s.KittyPID != 0 {
					t.Errorf("expected detached with PID 0, got detached=%v pid=%d", s.Detached, s.KittyPID)
				}
			},
		},
		{
			name:  "remote detached to reattached with new PID",
			setup: &WorkspaceState{Name: "host@trans-ws", Active: true, Remote: true, Host: "host", KittyPID: 0, Detached: true},
			apply: WorkspaceState{Name: "host@trans-ws", Active: true, Remote: true, Host: "host", KittyPID: 300, Detached: false},
			expect: func(t *testing.T, wsName string) {
				s, _ := Load(wsName)
				if s.Detached || s.KittyPID != 300 {
					t.Errorf("expected reattached PID 300, got detached=%v pid=%d", s.Detached, s.KittyPID)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupTestDirs(t)
			if tt.setup != nil {
				Save(*tt.setup)
			}
			if tt.remove {
				wsName := "trans-ws"
				if tt.setup != nil {
					wsName = tt.setup.Name
				}
				Remove(wsName)
				tt.expect(t, wsName)
			} else {
				Save(tt.apply)
				tt.expect(t, tt.apply.Name)
			}
		})
	}
}
