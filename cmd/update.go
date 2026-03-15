package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"

	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update ws to the latest release from GitHub",
	RunE: func(cmd *cobra.Command, args []string) error {
		suffix := runtime.GOOS + "-" + runtime.GOARCH
		url := "https://github.com/skarlsson/ws-manager/releases/latest/download/ws-" + suffix

		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("finding current executable: %w", err)
		}

		fmt.Printf("Downloading ws-%s...\n", suffix)
		resp, err := http.Get(url)
		if err != nil {
			return fmt.Errorf("downloading: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("download failed: HTTP %d (no release for %s?)", resp.StatusCode, suffix)
		}

		tmp, err := os.CreateTemp("", "ws-update-*")
		if err != nil {
			return fmt.Errorf("creating temp file: %w", err)
		}
		tmpPath := tmp.Name()
		defer os.Remove(tmpPath)

		if _, err := io.Copy(tmp, resp.Body); err != nil {
			tmp.Close()
			return fmt.Errorf("writing download: %w", err)
		}
		tmp.Close()

		if err := os.Chmod(tmpPath, 0755); err != nil {
			return fmt.Errorf("chmod: %w", err)
		}

		if err := os.Rename(tmpPath, self); err != nil {
			return fmt.Errorf("replacing binary: %w", err)
		}

		fmt.Printf("Updated: %s\n", self)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
