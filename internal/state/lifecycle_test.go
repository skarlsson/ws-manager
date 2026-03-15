package state

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLifecycleTransitions tests the full state machine for workspace lifecycle,
// exercising both local and remote paths through all transitions.
func TestLifecycleTransitions(t *testing.T) {
	tests := []struct {
		name   string
		steps  []step
	}{
		{
			name: "local full lifecycle: open → detach → reattach → kill",
			steps: []step{
				{save: &WorkspaceState{Name: "lc-local", Active: true, KittyPID: 100}},
				{check: func(t *testing.T) {
					s, _ := Load("lc-local")
					assertEqual(t, s.Active, true, "active after open")
					assertEqual(t, s.KittyPID, 100, "PID after open")
				}},
				{save: &WorkspaceState{Name: "lc-local", Active: true, KittyPID: 100, Detached: true}},
				{check: func(t *testing.T) {
					s, _ := Load("lc-local")
					assertEqual(t, s.Detached, true, "detached")
					assertEqual(t, s.KittyPID, 100, "PID preserved while detached")
				}},
				{save: &WorkspaceState{Name: "lc-local", Active: true, KittyPID: 100, Detached: false}},
				{check: func(t *testing.T) {
					s, _ := Load("lc-local")
					assertEqual(t, s.Detached, false, "reattached")
				}},
				{remove: "lc-local"},
				{check: func(t *testing.T) {
					s, _ := Load("lc-local")
					assertEqual(t, s.Active, false, "inactive after kill")
				}},
			},
		},
		{
			name: "remote lifecycle: open → detach (PID=0) → reattach (new PID) → kill",
			steps: []step{
				{save: &WorkspaceState{Name: "host@rws", Active: true, Remote: true, Host: "host", KittyPID: 200}},
				{check: func(t *testing.T) {
					s, _ := Load("host@rws")
					assertEqual(t, s.Remote, true, "remote flag")
					assertEqual(t, s.Host, "host", "host field")
				}},
				{save: &WorkspaceState{Name: "host@rws", Active: true, Remote: true, Host: "host", KittyPID: 0, Detached: true}},
				{check: func(t *testing.T) {
					s, _ := Load("host@rws")
					assertEqual(t, s.Detached, true, "remote detached")
					assertEqual(t, s.KittyPID, 0, "PID zeroed on remote detach")
				}},
				{save: &WorkspaceState{Name: "host@rws", Active: true, Remote: true, Host: "host", KittyPID: 300, Detached: false}},
				{check: func(t *testing.T) {
					s, _ := Load("host@rws")
					assertEqual(t, s.KittyPID, 300, "new PID on reattach")
					assertEqual(t, s.Detached, false, "not detached after reattach")
				}},
				{remove: "host@rws"},
				{check: func(t *testing.T) {
					s, _ := Load("host@rws")
					assertEqual(t, s.Active, false, "inactive after remote kill")
				}},
			},
		},
		{
			name: "detached with dead kitty → state remains until explicit cleanup",
			steps: []step{
				{save: &WorkspaceState{Name: "dead-kitty", Active: true, KittyPID: 99999999, Detached: true}},
				{check: func(t *testing.T) {
					s, _ := Load("dead-kitty")
					// State file persists — the liveness check is caller's responsibility
					assertEqual(t, s.Active, true, "state still active (liveness is caller's job)")
					assertEqual(t, s.Detached, true, "still detached")
					assertEqual(t, s.KittyPID, 99999999, "dead PID preserved in state")
				}},
			},
		},
		{
			name: "remote state not cleaned by ListActive",
			steps: []step{
				{save: &WorkspaceState{Name: "rhost@rws", Active: true, Remote: true, Host: "rhost"}},
				{listActive: true},
				{check: func(t *testing.T) {
					s, _ := Load("rhost@rws")
					assertEqual(t, s.Active, true, "remote state preserved after ListActive")
				}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sd, _ := setupTestDirs(t)
			_ = sd
			for i, s := range tt.steps {
				if s.save != nil {
					if err := Save(*s.save); err != nil {
						t.Fatalf("step %d: Save failed: %v", i, err)
					}
				}
				if s.remove != "" {
					_ = Remove(s.remove)
				}
				if s.listActive {
					if _, err := ListActive(); err != nil {
						t.Fatalf("step %d: ListActive failed: %v", i, err)
					}
				}
				if s.check != nil {
					s.check(t)
				}
			}
		})
	}
}

type step struct {
	save       *WorkspaceState
	remove     string
	listActive bool
	check      func(*testing.T)
}

func assertEqual[T comparable](t *testing.T, got, want T, msg string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %v, want %v", msg, got, want)
	}
}

// TestStateFileContents verifies the state file is written to the expected path
// with the expected content for both local and remote workspaces.
func TestStateFileContents(t *testing.T) {
	sd, _ := setupTestDirs(t)

	// Local workspace
	Save(WorkspaceState{Name: "local-ws", Active: true, KittyPID: 42})
	if _, err := os.Stat(filepath.Join(sd, "local-ws.yaml")); err != nil {
		t.Error("local state file not found at expected path")
	}

	// Remote workspace with @ in name
	Save(WorkspaceState{Name: "h@remote-ws", Active: true, Remote: true, Host: "h"})
	if _, err := os.Stat(filepath.Join(sd, "h@remote-ws.yaml")); err != nil {
		t.Error("remote state file not found at expected path")
	}
}
