package cmd

import (
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/skarlsson/workshell/internal/config"
	"github.com/skarlsson/workshell/internal/git"
	"github.com/skarlsson/workshell/internal/kitty"
	"github.com/skarlsson/workshell/internal/ssh"
	"github.com/skarlsson/workshell/internal/state"
	"github.com/skarlsson/workshell/internal/zellij"
	"github.com/spf13/cobra"
)

// parseWorkspaceRef parses "host:name" or just "name".
// Returns (hostName, wsName). hostName is empty for local workspaces.
func parseWorkspaceRef(ref string) (string, string) {
	if i := strings.IndexByte(ref, ':'); i > 0 {
		return ref[:i], ref[i+1:]
	}
	return "", ref
}

// stateKey returns the state file key for a workspace.
// For remote: "host@name", for local: "name".
func stateKey(hostName, wsName string) string {
	if hostName != "" {
		return hostName + "@" + wsName
	}
	return wsName
}

// openWorkspace opens a workspace. Accepts "name" (local) or "host:name" (remote).
// Also supports legacy local configs with Host field.
func openWorkspace(ref string) error {
	hostName, wsName := parseWorkspaceRef(ref)

	// If no host in ref, check if local config has Host field (legacy)
	if hostName == "" {
		if ws, err := config.LoadWorkspace(wsName); err == nil && ws.IsRemote() {
			hostName = ws.Host
		}
	}

	if hostName != "" {
		return openRemoteWorkspace(hostName, wsName)
	}

	return openLocalWorkspace(wsName)
}

func openLocalWorkspace(name string) error {
	ws, err := config.LoadWorkspace(name)
	if err != nil {
		return fmt.Errorf("workspace %q not found: %w", name, err)
	}

	st, _ := state.Load(name)

	// Detached with kitty still alive → unminimize and reactivate
	if st.Active && st.Detached && kitty.IsAlive(name, st.KittyPID) {
		if err := kitty.Activate(name); err != nil {
			return fmt.Errorf("reactivating detached workspace %q: %w", name, err)
		}
		st.Detached = false
		if err := state.Save(st); err != nil {
			return fmt.Errorf("saving state: %w", err)
		}
		return nil
	}

	// Detached but kitty died → clean up stale state, launch fresh
	if st.Active && st.Detached && !kitty.IsAlive(name, st.KittyPID) {
		_ = state.Remove(name)
	}

	if st.Active && !st.Detached && kitty.IsAlive(name, st.KittyPID) {
		return fmt.Errorf("workspace %q is already open (PID %d)", name, st.KittyPID)
	}

	layoutPath, err := zellij.GenerateLayout(ws)
	if err != nil {
		return fmt.Errorf("generating layout: %w", err)
	}

	session := zellij.SessionName(name)
	zellij.CleanupSession(session)

	title := fmt.Sprintf("ws: %s", name)
	if git.IsGitRepo(ws.Dir) {
		if branch, err := git.CurrentBranch(ws.Dir); err == nil {
			title = fmt.Sprintf("ws: %s [%s]", name, branch)
		}
	}
	pid, err := kitty.Launch(name, ws.Dir, title)
	if err != nil {
		return fmt.Errorf("launching kitty: %w", err)
	}

	socket := kitty.SocketPath(name)
	zellijCmd := zellij.LaunchCommand(session, layoutPath, ws.Dir)

	if err := waitForSocket(socket, 5*time.Second); err != nil {
		fmt.Printf("Warning: kitty socket not ready: %v\n", err)
	}

	if err := kitty.SendText(socket, zellijCmd); err != nil {
		fmt.Printf("Warning: could not auto-start zellij: %v\n", err)
		fmt.Println("Start it manually with: zellij --session", session, "--layout", layoutPath)
	}

	st = state.WorkspaceState{
		Name:          name,
		KittyPID:      pid,
		ZellijSession: session,
		Active:        true,
	}
	if err := state.Save(st); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	return nil
}

func openRemoteWorkspace(hostName, wsName string) error {
	host, err := config.LoadHost(hostName)
	if err != nil {
		return fmt.Errorf("host %q not found: %w", hostName, err)
	}

	sk := stateKey(hostName, wsName)
	st, _ := state.Load(sk)

	// Check if kitty is alive via socket (reliable) or PID
	if kitty.IsAlive(sk, st.KittyPID) {
		// Already open — focus the window
		if err := kitty.Activate(sk); err != nil {
			return fmt.Errorf("workspace %q is already open (PID %d)", wsName, st.KittyPID)
		}
		st.Detached = false
		st.Active = true
		_ = state.Save(st)
		return nil
	}

	session := zellij.SessionName(wsName)

	// Query remote for branch info
	branch := ""
	statuses, err := ssh.GetRemoteStatuses(host.SSH)
	if err == nil {
		for _, rs := range statuses {
			if rs.Name == wsName {
				branch = rs.Branch
				break
			}
		}
	}

	title := fmt.Sprintf("ws: %s [%s]", wsName, hostName)
	if branch != "" {
		title = fmt.Sprintf("ws: %s [%s] (%s)", wsName, branch, hostName)
	}
	pid, err := kitty.LaunchRemote(sk, title)
	if err != nil {
		return fmt.Errorf("launching kitty: %w", err)
	}

	socket := kitty.SocketPath(sk)
	if err := waitForSocket(socket, 5*time.Second); err != nil {
		fmt.Printf("Warning: kitty socket not ready: %v\n", err)
	}

	sshCmd := ssh.InteractiveCommand(host.SSH, fmt.Sprintf("~/.local/bin/ws attach %s", wsName))
	if err := kitty.SendText(socket, sshCmd); err != nil {
		fmt.Printf("Warning: could not send SSH command: %v\n", err)
	}

	st = state.WorkspaceState{
		Name:          sk,
		KittyPID:      pid,
		ZellijSession: session,
		Active:        true,
		Detached:      false,
		Remote:        true,
		Host:          hostName,
	}
	if err := state.Save(st); err != nil {
		return fmt.Errorf("saving state: %w", err)
	}

	return nil
}

var openCmd = &cobra.Command{
	Use:   "open <workspace>",
	Short: "Open a workspace in a new kitty window with zellij",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ref := args[0]
		if err := openWorkspace(ref); err != nil {
			return err
		}
		hostName, wsName := parseWorkspaceRef(ref)
		if hostName == "" {
			if ws, err := config.LoadWorkspace(wsName); err == nil && ws.IsRemote() {
				hostName = ws.Host
			}
		}
		sk := stateKey(hostName, wsName)
		st, _ := state.Load(sk)
		fmt.Printf("Opened workspace %q (kitty PID %d, zellij session %q)\n", ref, st.KittyPID, st.ZellijSession)
		return nil
	},
}

func waitForSocket(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			// Socket file exists, try connecting
			conn, err := net.Dial("unix", path)
			if err == nil {
				conn.Close()
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", path)
}

func init() {
	rootCmd.AddCommand(openCmd)
}
