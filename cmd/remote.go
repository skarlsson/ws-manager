package cmd

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"

	"github.com/skarlsson/workshell/internal/config"
	"github.com/skarlsson/workshell/internal/ssh"
	"github.com/spf13/cobra"
)

var remoteCmd = &cobra.Command{
	Use:   "remote",
	Short: "Manage remote hosts for workspace sessions",
}

var remoteAddCmd = &cobra.Command{
	Use:   "add <name> <ssh-target>",
	Short: "Add a remote host",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		sshTarget := args[1]
		wsDir, _ := cmd.Flags().GetString("workspace-dir")

		h := config.HostConfig{
			Name:         name,
			SSH:          sshTarget,
			WorkspaceDir: wsDir,
		}
		if err := config.AddHost(h); err != nil {
			return err
		}
		fmt.Printf("Added host %q (ssh: %s)\n", name, sshTarget)
		return nil
	},
}

var remoteListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured remote hosts",
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		hosts, err := config.LoadHosts()
		if err != nil {
			return err
		}
		if len(hosts) == 0 {
			fmt.Println("No remote hosts configured. Use 'ws remote add' to add one.")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tSSH\tWORKSPACE DIR")
		fmt.Fprintln(w, "----\t---\t-------------")
		for _, h := range hosts {
			fmt.Fprintf(w, "%s\t%s\t%s\n", h.Name, h.SSH, h.WorkspaceDir)
		}
		w.Flush()
		return nil
	},
}

var remoteRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a remote host",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := config.RemoveHost(name); err != nil {
			return err
		}
		fmt.Printf("Removed host %q\n", name)
		return nil
	},
}

var remoteSetupCmd = &cobra.Command{
	Use:   "setup <host>",
	Short: "Install ws binary on a remote host",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		host, err := config.LoadHost(args[0])
		if err != nil {
			return err
		}

		fmt.Printf("Setting up ws on %s (%s)...\n", host.Name, host.SSH)

		// Check connection
		fmt.Print("  Checking SSH connection... ")
		if err := ssh.CheckConnection(host.SSH); err != nil {
			fmt.Println("FAILED")
			return fmt.Errorf("cannot connect to %s: %w", host.SSH, err)
		}
		fmt.Println("OK")

		// Get architecture
		fmt.Print("  Detecting architecture... ")
		arch, err := ssh.GetArch(host.SSH)
		if err != nil {
			fmt.Println("FAILED")
			return fmt.Errorf("cannot detect arch: %w", err)
		}
		goarch := mapArch(arch)
		fmt.Printf("%s (%s)\n", arch, goarch)

		// Strategy: try scp of local binary if arch matches, otherwise GitHub release
		installed := false

		// 1. Direct copy if local arch matches
		localBin, _ := os.Executable()
		if runtime.GOOS == "linux" && runtime.GOARCH == goarch && localBin != "" {
			fmt.Print("  Copying local ws binary via scp... ")
			if _, err := ssh.Run(host.SSH, "mkdir -p ~/.local/bin"); err == nil {
				if err := ssh.CopyFile(host.SSH, localBin, "~/.local/bin/ws"); err == nil {
					if _, err := ssh.Run(host.SSH, "chmod +x ~/.local/bin/ws"); err == nil {
						fmt.Println("OK")
						installed = true
					}
				}
			}
			if !installed {
				fmt.Println("failed, trying GitHub release...")
			}
		}

		// 2. GitHub release download
		if !installed {
			fmt.Print("  Installing via GitHub release... ")
			dlCmd := fmt.Sprintf(
				"mkdir -p ~/.local/bin && curl -fsSL -L https://github.com/skarlsson/workshell/releases/latest/download/ws-linux-%s -o ~/.local/bin/ws && chmod +x ~/.local/bin/ws",
				goarch,
			)
			if _, err := ssh.Run(host.SSH, dlCmd); err != nil {
				fmt.Println("FAILED")
				return fmt.Errorf("no release found — push a tag (e.g. git tag v0.1.0 && git push --tags) to create one: %w", err)
			}
			fmt.Println("OK")
		}

		// Verify ws
		fmt.Print("  Verifying ws... ")
		ver, err := ssh.Run(host.SSH, "~/.local/bin/ws version 2>&1 || echo 'ws not working'")
		if err != nil {
			fmt.Println("FAILED")
			return fmt.Errorf("verification failed: %w", err)
		}
		fmt.Println(ver)

		// Ensure ~/.local/bin is in PATH before installing deps
		fmt.Print("  Checking PATH... ")
		pathCheck, _ := ssh.Run(host.SSH, "echo $PATH")
		if !strings.Contains(pathCheck, ".local/bin") {
			fmt.Println("adding ~/.local/bin to PATH")
			ssh.Run(host.SSH, `grep -q 'export PATH="$HOME/.local/bin:$PATH"' ~/.bashrc 2>/dev/null || echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc`)
		} else {
			fmt.Println("OK")
		}

		// Install remote dependencies via ws on the remote
		fmt.Println("  Installing remote dependencies...")
		depOut, err := ssh.Run(host.SSH, "export PATH=\"$HOME/.local/bin:$PATH\" && ws deps install --remote 2>&1")
		if err != nil {
			fmt.Printf("  Warning: %v\n", err)
		}
		if depOut != "" {
			for _, line := range strings.Split(depOut, "\n") {
				fmt.Printf("    %s\n", line)
			}
		}

		fmt.Printf("\nSetup complete for %s\n", host.Name)
		return nil
	},
}

var remoteStatusCmd = &cobra.Command{
	Use:   "status <host>",
	Short: "Check status of a remote host",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		host, err := config.LoadHost(args[0])
		if err != nil {
			return err
		}

		fmt.Printf("Host: %s (%s)\n", host.Name, host.SSH)

		// Connection
		fmt.Print("  Connection: ")
		if err := ssh.CheckConnection(host.SSH); err != nil {
			fmt.Println("FAILED")
			return nil
		}
		fmt.Println("OK")

		// ws version
		fmt.Print("  ws version: ")
		ver, err := ssh.Run(host.SSH, "~/.local/bin/ws version 2>&1")
		if err != nil {
			fmt.Println("not installed")
		} else {
			fmt.Println(ver)
		}

		// zellij sessions
		fmt.Print("  Zellij sessions: ")
		sessions, err := ssh.Run(host.SSH, "zellij list-sessions --short 2>/dev/null")
		if err != nil || sessions == "" {
			fmt.Println("none")
		} else {
			fmt.Println()
			for _, s := range strings.Split(sessions, "\n") {
				fmt.Printf("    %s\n", s)
			}
		}

		return nil
	},
}

func mapArch(uname string) string {
	switch strings.TrimSpace(uname) {
	case "x86_64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	default:
		return uname
	}
}

func init() {
	remoteAddCmd.Flags().String("workspace-dir", "", "Default workspace directory on remote")
	remoteCmd.AddCommand(remoteAddCmd)
	remoteCmd.AddCommand(remoteListCmd)
	remoteCmd.AddCommand(remoteRemoveCmd)
	remoteCmd.AddCommand(remoteSetupCmd)
	remoteCmd.AddCommand(remoteStatusCmd)
	rootCmd.AddCommand(remoteCmd)
}
