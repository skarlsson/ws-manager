package cmd

import (
	"fmt"

	"github.com/skarlsson/workshell/internal/config"
	"github.com/skarlsson/workshell/internal/git"
	"github.com/spf13/cobra"
)

var taskCmd = &cobra.Command{
	Use:   "task",
	Short: "Manage workspace tasks (git branches)",
}

var taskStartCmd = &cobra.Command{
	Use:   "start <task-name>",
	Short: "Create a task branch and switch to it",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskName := args[0]
		wsName, _ := cmd.Flags().GetString("workspace")

		ws, err := resolveWorkspace(wsName)
		if err != nil {
			return err
		}

		branch := "task/" + taskName

		// Auto-stash if dirty
		if dirty, _ := git.HasChanges(ws.Dir); dirty {
			fmt.Println("Stashing uncommitted changes...")
			if err := git.StashPush(ws.Dir, "workshell: before task "+taskName); err != nil {
				return fmt.Errorf("stashing changes: %w", err)
			}
		}

		if git.BranchExists(ws.Dir, branch) {
			if err := git.Checkout(ws.Dir, branch); err != nil {
				return fmt.Errorf("checking out branch: %w", err)
			}
			fmt.Printf("Switched to existing task branch %q\n", branch)
		} else {
			if err := git.CreateAndCheckout(ws.Dir, branch); err != nil {
				return fmt.Errorf("creating branch: %w", err)
			}
			fmt.Printf("Created and switched to task branch %q\n", branch)
		}

		ws.CurrentTask = taskName
		refreshTitle(ws.Name)
		return config.SaveWorkspace(ws)
	},
}

var taskDoneCmd = &cobra.Command{
	Use:   "done",
	Short: "Finish current task and return to main branch",
	RunE: func(cmd *cobra.Command, args []string) error {
		wsName, _ := cmd.Flags().GetString("workspace")

		ws, err := resolveWorkspace(wsName)
		if err != nil {
			return err
		}

		if ws.CurrentTask == "" {
			return fmt.Errorf("no active task in workspace %q", ws.Name)
		}

		currentBranch, err := git.CurrentBranch(ws.Dir)
		if err != nil {
			return fmt.Errorf("getting current branch: %w", err)
		}

		// Auto-stash if dirty
		if dirty, _ := git.HasChanges(ws.Dir); dirty {
			fmt.Println("Stashing uncommitted changes...")
			if err := git.StashPush(ws.Dir, "workshell: finishing task "+ws.CurrentTask); err != nil {
				return fmt.Errorf("stashing changes: %w", err)
			}
		}

		// Switch back to default branch
		base := ws.DefaultBranch
		if base == "" {
			base = "main"
		}
		if err := git.Checkout(ws.Dir, base); err != nil {
			return fmt.Errorf("checking out %s: %w", base, err)
		}

		fmt.Printf("Finished task %q (branch %q preserved)\n", ws.CurrentTask, currentBranch)
		fmt.Println("Branch was not deleted. Merge or create a PR when ready.")

		ws.CurrentTask = ""
		refreshTitle(ws.Name)
		return config.SaveWorkspace(ws)
	},
}

var taskListCmd = &cobra.Command{
	Use:   "list",
	Short: "List task branches for the workspace",
	RunE: func(cmd *cobra.Command, args []string) error {
		wsName, _ := cmd.Flags().GetString("workspace")

		ws, err := resolveWorkspace(wsName)
		if err != nil {
			return err
		}

		if !git.IsGitRepo(ws.Dir) {
			return fmt.Errorf("%s is not a git repository", ws.Dir)
		}

		branches, err := git.ListBranches(ws.Dir, "task/")
		if err != nil {
			return fmt.Errorf("listing branches: %w", err)
		}

		if len(branches) == 0 {
			fmt.Println("No task branches found.")
			return nil
		}

		current, _ := git.CurrentBranch(ws.Dir)
		for _, b := range branches {
			marker := "  "
			if b == current {
				marker = "* "
			}
			fmt.Printf("%s%s\n", marker, b)
		}
		return nil
	},
}

var taskSwitchCmd = &cobra.Command{
	Use:   "switch <task-name>",
	Short: "Switch to an existing task branch",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		taskName := args[0]
		wsName, _ := cmd.Flags().GetString("workspace")

		ws, err := resolveWorkspace(wsName)
		if err != nil {
			return err
		}

		branch := "task/" + taskName
		if !git.BranchExists(ws.Dir, branch) {
			return fmt.Errorf("task branch %q does not exist", branch)
		}

		// Auto-stash if dirty
		if dirty, _ := git.HasChanges(ws.Dir); dirty {
			fmt.Println("Stashing uncommitted changes...")
			if err := git.StashPush(ws.Dir, "workshell: switching to "+taskName); err != nil {
				return fmt.Errorf("stashing changes: %w", err)
			}
		}

		if err := git.Checkout(ws.Dir, branch); err != nil {
			return fmt.Errorf("checking out branch: %w", err)
		}

		ws.CurrentTask = taskName
		refreshTitle(ws.Name)
		if err := config.SaveWorkspace(ws); err != nil {
			return fmt.Errorf("saving workspace: %w", err)
		}

		fmt.Printf("Switched to task %q\n", taskName)
		return nil
	},
}

func resolveWorkspace(name string) (config.Workspace, error) {
	if name != "" {
		return config.LoadWorkspace(name)
	}
	// Try to detect from current directory
	workspaces, err := config.ListWorkspaces()
	if err != nil {
		return config.Workspace{}, err
	}
	// For now, require explicit workspace name if we can't detect
	if len(workspaces) == 1 {
		return workspaces[0], nil
	}
	return config.Workspace{}, fmt.Errorf("specify workspace with --workspace flag")
}

func init() {
	taskCmd.PersistentFlags().StringP("workspace", "w", "", "workspace name")
	taskCmd.AddCommand(taskStartCmd)
	taskCmd.AddCommand(taskDoneCmd)
	taskCmd.AddCommand(taskListCmd)
	taskCmd.AddCommand(taskSwitchCmd)
	rootCmd.AddCommand(taskCmd)
}
