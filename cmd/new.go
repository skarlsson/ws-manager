package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/skarlsson/workshell/internal/config"
	"github.com/skarlsson/workshell/internal/git"
	"github.com/skarlsson/workshell/internal/ssh"
	"github.com/spf13/cobra"
)

var newCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new workspace interactively",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.EnsureDirs(); err != nil {
			return err
		}

		hostFlag, _ := cmd.Flags().GetString("host")
		nonInteractive, _ := cmd.Flags().GetBool("non-interactive")
		nameFlag, _ := cmd.Flags().GetString("name")
		dirFlag, _ := cmd.Flags().GetString("dir")
		gitURLFlag, _ := cmd.Flags().GetString("git-url")

		// Non-interactive mode (for remote setup via SSH)
		if nonInteractive {
			return createWorkspaceNonInteractive(nameFlag, dirFlag, gitURLFlag, hostFlag)
		}

		// Remote workspace creation
		if hostFlag != "" {
			return createRemoteWorkspace(cmd, hostFlag)
		}

		reader := bufio.NewReader(os.Stdin)
		prompt := func(label, defaultVal string) string {
			if defaultVal != "" {
				fmt.Printf("%s [%s]: ", label, defaultVal)
			} else {
				fmt.Printf("%s: ", label)
			}
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(input)
			if input == "" {
				return defaultVal
			}
			return input
		}

		// 1. Name
		name := prompt("Workspace name", "")
		if name == "" {
			return fmt.Errorf("workspace name is required")
		}
		if config.WorkspaceExists(name) {
			return fmt.Errorf("workspace %q already exists", name)
		}

		// 2. Directory
		cwd, _ := os.Getwd()
		defaultDir := filepath.Join(cwd, name)
		dir := prompt("Directory", defaultDir)
		dir, _ = filepath.Abs(dir)

		// 3. Ensure directory exists — clone or init repo
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			repo := prompt("Directory doesn't exist. Git repo URL to clone (empty to init new repo)", "")
			if repo != "" {
				fmt.Printf("Cloning %s into %s...\n", repo, dir)
				clone := exec.Command("git", "clone", "--recursive", repo, dir)
				clone.Stdout = os.Stdout
				clone.Stderr = os.Stderr
				if err := clone.Run(); err != nil {
					return fmt.Errorf("cloning repo: %w", err)
				}
			} else {
				fmt.Printf("Creating %s with git init...\n", dir)
				if err := os.MkdirAll(dir, 0755); err != nil {
					return fmt.Errorf("creating directory: %w", err)
				}
				initCmd := exec.Command("git", "init", dir)
				initCmd.Stdout = os.Stdout
				initCmd.Stderr = os.Stderr
				if err := initCmd.Run(); err != nil {
					return fmt.Errorf("git init: %w", err)
				}
			}
		} else if !git.IsGitRepo(dir) {
			fmt.Printf("Initializing git repo in %s...\n", dir)
			initCmd := exec.Command("git", "init", dir)
			initCmd.Stdout = os.Stdout
			initCmd.Stderr = os.Stderr
			if err := initCmd.Run(); err != nil {
				fmt.Printf("Warning: git init failed: %v\n", err)
			}
		} else {
			fmt.Printf("Using existing repo %s\n", dir)
		}

		// 4. Layout
		layout := prompt("Layout", "default")

		// 5. Auto-start claude
		autoClaudeStr := prompt("Auto-start claude in left pane? (y/n)", "y")
		autoClaude := strings.ToLower(autoClaudeStr) == "y" || strings.ToLower(autoClaudeStr) == "yes"

		// 6. Setup commands
		fmt.Println("Setup commands (one per line, empty line to finish):")
		var setupCmds []string
		for {
			line, _ := reader.ReadString('\n')
			line = strings.TrimSpace(line)
			if line == "" {
				break
			}
			setupCmds = append(setupCmds, line)
		}

		// Detect default branch from the repo
		defaultBranch := git.DefaultBranch(dir)

		ws := config.Workspace{
			Name:          name,
			Dir:           dir,
			DefaultBranch: defaultBranch,
			Layout:        layout,
			AutoClaude:    autoClaude,
			SetupCommands: setupCmds,
		}

		if err := config.SaveWorkspace(ws); err != nil {
			return fmt.Errorf("saving workspace: %w", err)
		}

		fmt.Printf("\nWorkspace %q created!\n", name)
		fmt.Printf("Config: %s\n", config.WorkspacePath(name))
		fmt.Printf("Open it with: ws open %s\n", name)

		// Run setup commands if any
		if len(setupCmds) > 0 {
			runSetup := prompt("Run setup commands now? (y/n)", "y")
			if strings.ToLower(runSetup) == "y" || strings.ToLower(runSetup) == "yes" {
				for _, c := range setupCmds {
					fmt.Printf("Running: %s\n", c)
					setup := exec.Command("bash", "-c", c)
					setup.Dir = dir
					setup.Stdout = os.Stdout
					setup.Stderr = os.Stderr
					if err := setup.Run(); err != nil {
						fmt.Printf("Warning: command failed: %v\n", err)
					}
				}
			}
		}

		return nil
	},
}

func createWorkspaceNonInteractive(name, dir, gitURL, host string) error {
	if name == "" {
		return fmt.Errorf("--name is required in non-interactive mode")
	}
	if dir == "" {
		return fmt.Errorf("--dir is required in non-interactive mode")
	}

	if config.WorkspaceExists(name) {
		fmt.Printf("Workspace %q already exists, skipping\n", name)
		return nil
	}

	// Clone if git URL provided, otherwise create dir + git init
	if gitURL != "" {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			fmt.Printf("Cloning %s into %s...\n", gitURL, dir)
			clone := exec.Command("git", "clone", "--recursive", gitURL, dir)
			clone.Stdout = os.Stdout
			clone.Stderr = os.Stderr
			if err := clone.Run(); err != nil {
				return fmt.Errorf("cloning repo: %w", err)
			}
		}
	} else {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating directory: %w", err)
		}
		if !git.IsGitRepo(dir) {
			initCmd := exec.Command("git", "init", dir)
			initCmd.Stdout = os.Stdout
			initCmd.Stderr = os.Stderr
			if err := initCmd.Run(); err != nil {
				fmt.Printf("Warning: git init failed: %v\n", err)
			}
		}
	}

	defaultBranch := "main"
	if git.IsGitRepo(dir) {
		defaultBranch = git.DefaultBranch(dir)
	}

	ws := config.Workspace{
		Name:          name,
		Dir:           dir,
		DefaultBranch: defaultBranch,
		Layout:        "default",
		AutoClaude:    true,
		Host:          host,
	}

	if err := config.SaveWorkspace(ws); err != nil {
		return fmt.Errorf("saving workspace: %w", err)
	}
	fmt.Printf("Workspace %q created\n", name)
	return nil
}

func createRemoteWorkspace(cmd *cobra.Command, hostName string) error {
	host, err := config.LoadHost(hostName)
	if err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)
	prompt := func(label, defaultVal string) string {
		if defaultVal != "" {
			fmt.Printf("%s [%s]: ", label, defaultVal)
		} else {
			fmt.Printf("%s: ", label)
		}
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "" {
			return defaultVal
		}
		return input
	}

	name := prompt("Workspace name", "")
	if name == "" {
		return fmt.Errorf("workspace name is required")
	}
	defaultDir := ""
	if host.WorkspaceDir != "" {
		defaultDir = host.WorkspaceDir + "/" + name
	}
	dir := prompt("Remote directory", defaultDir)
	if dir == "" {
		return fmt.Errorf("remote directory is required")
	}

	gitURL := prompt("Git repo URL (empty for existing dir)", "")

	// Create workspace on remote only — auto-discovery handles the rest
	fmt.Printf("Creating workspace on %s...\n", hostName)
	if gitURL != "" {
		ssh.EnsureGitHubHostKey(host.SSH)
	}
	remoteArgs := fmt.Sprintf("~/.local/bin/ws new --non-interactive --name %s --dir %s", name, dir)
	if gitURL != "" {
		remoteArgs += fmt.Sprintf(" --git-url %s", gitURL)
	}
	out, err := ssh.Run(host.SSH, remoteArgs)
	if err != nil {
		return fmt.Errorf("remote workspace creation failed: %w", err)
	}
	fmt.Println(out)

	fmt.Printf("\nWorkspace %q created (remote: %s)\n", name, hostName)
	fmt.Printf("Open it with: ws open %s:%s\n", hostName, name)
	return nil
}

func init() {
	newCmd.Flags().String("host", "", "Create workspace on a remote host")
	newCmd.Flags().Bool("non-interactive", false, "Non-interactive mode (for programmatic use)")
	newCmd.Flags().String("name", "", "Workspace name (non-interactive mode)")
	newCmd.Flags().String("dir", "", "Workspace directory (non-interactive mode)")
	newCmd.Flags().String("git-url", "", "Git repo URL to clone (non-interactive mode)")
	rootCmd.AddCommand(newCmd)
}
