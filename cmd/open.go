package cmd

import (
	"fmt"
	"net"
	"os"
	"time"

	"github.com/skarlsson/ws-manager/internal/config"
	"github.com/skarlsson/ws-manager/internal/git"
	"github.com/skarlsson/ws-manager/internal/kitty"
	"github.com/skarlsson/ws-manager/internal/ssh"
	"github.com/skarlsson/ws-manager/internal/state"
	"github.com/skarlsson/ws-manager/internal/zellij"
	"github.com/spf13/cobra"
)

// openWorkspace opens a workspace by launching kitty + zellij.
func openWorkspace(name string) error {
	ws, err := config.LoadWorkspace(name)
	if err != nil {
		return fmt.Errorf("workspace %q not found: %w", name, err)
	}

	if ws.IsRemote() {
		return openRemoteWorkspace(name, ws)
	}

	// Check if already active
	st, _ := state.Load(name)
	if st.Active && kitty.IsRunning(st.KittyPID) {
		return fmt.Errorf("workspace %q is already open (PID %d)", name, st.KittyPID)
	}

	// Generate layout
	layoutPath, err := zellij.GenerateLayout(ws)
	if err != nil {
		return fmt.Errorf("generating layout: %w", err)
	}

	// Clean up any dead zellij session with the same name
	session := zellij.SessionName(name)
	zellij.CleanupSession(session)

	// Launch kitty with branch in title
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

	// Wait for kitty socket to be ready
	socket := kitty.SocketPath(name)
	zellijCmd := zellij.LaunchCommand(session, layoutPath, ws.Dir)

	if err := waitForSocket(socket, 5*time.Second); err != nil {
		fmt.Printf("Warning: kitty socket not ready: %v\n", err)
	}

	if err := kitty.SendText(socket, zellijCmd); err != nil {
		fmt.Printf("Warning: could not auto-start zellij: %v\n", err)
		fmt.Println("Start it manually with: zellij --session", session, "--layout", layoutPath)
	}

	// Save state
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

// openRemoteWorkspace opens a remote workspace: local kitty + SSH to remote ws attach.
func openRemoteWorkspace(name string, ws config.Workspace) error {
	host, err := config.LoadHost(ws.Host)
	if err != nil {
		return fmt.Errorf("loading host %q: %w", ws.Host, err)
	}

	// Check if local kitty is already running
	st, _ := state.Load(name)
	if st.Active && kitty.IsRunning(st.KittyPID) {
		return fmt.Errorf("workspace %q is already open (PID %d)", name, st.KittyPID)
	}

	session := zellij.SessionName(name)

	// Launch kitty without --directory (workspace dir is on remote)
	title := fmt.Sprintf("ws: %s [%s]", name, host.Name)
	pid, err := kitty.LaunchRemote(name, title)
	if err != nil {
		return fmt.Errorf("launching kitty: %w", err)
	}

	// Wait for kitty socket
	socket := kitty.SocketPath(name)
	if err := waitForSocket(socket, 5*time.Second); err != nil {
		fmt.Printf("Warning: kitty socket not ready: %v\n", err)
	}

	// Send SSH command that runs ws attach on remote
	sshCmd := ssh.InteractiveCommand(host.SSH, fmt.Sprintf("~/.local/bin/ws attach %s", name))
	if err := kitty.SendText(socket, sshCmd); err != nil {
		fmt.Printf("Warning: could not send SSH command: %v\n", err)
	}

	// Save state
	st = state.WorkspaceState{
		Name:          name,
		KittyPID:      pid,
		ZellijSession: session,
		Active:        true,
		Remote:        true,
		Host:          ws.Host,
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
		name := args[0]
		if err := openWorkspace(name); err != nil {
			return err
		}
		st, _ := state.Load(name)
		fmt.Printf("Opened workspace %q (kitty PID %d, zellij session %q)\n", name, st.KittyPID, st.ZellijSession)
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
