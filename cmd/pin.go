package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func getPinFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".fastbrew", "pinned")
}

func loadPinnedPackages() (map[string]bool, error) {
	pinned := make(map[string]bool)
	path := getPinFilePath()

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return pinned, nil
		}
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name != "" && !strings.HasPrefix(name, "#") {
			pinned[name] = true
		}
	}
	return pinned, scanner.Err()
}

func savePinnedPackages(pinned map[string]bool) error {
	path := getPinFilePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	for name := range pinned {
		fmt.Fprintln(file, name)
	}
	return nil
}

var pinCmd = &cobra.Command{
	Use:   "pin <package>",
	Short: "Pin a package to prevent upgrades",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pkg := args[0]
		pinned, err := loadPinnedPackages()
		if err != nil {
			fmt.Printf("Error loading pinned packages: %v\n", err)
			os.Exit(1)
		}

		if pinned[pkg] {
			fmt.Printf("📌 %s is already pinned\n", pkg)
			return
		}

		pinned[pkg] = true
		if err := savePinnedPackages(pinned); err != nil {
			fmt.Printf("Error saving pinned packages: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("📌 Pinned %s\n", pkg)
	},
}

var unpinCmd = &cobra.Command{
	Use:   "unpin <package>",
	Short: "Unpin a package to allow upgrades",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		pkg := args[0]
		pinned, err := loadPinnedPackages()
		if err != nil {
			fmt.Printf("Error loading pinned packages: %v\n", err)
			os.Exit(1)
		}

		if !pinned[pkg] {
			fmt.Printf("%s is not pinned\n", pkg)
			return
		}

		delete(pinned, pkg)
		if err := savePinnedPackages(pinned); err != nil {
			fmt.Printf("Error saving pinned packages: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("📍 Unpinned %s\n", pkg)
	},
}

var pinnedCmd = &cobra.Command{
	Use:   "pinned",
	Short: "List pinned packages",
	Run: func(cmd *cobra.Command, args []string) {
		pinned, err := loadPinnedPackages()
		if err != nil {
			fmt.Printf("Error loading pinned packages: %v\n", err)
			os.Exit(1)
		}

		if len(pinned) == 0 {
			fmt.Println("No pinned packages.")
			return
		}

		fmt.Println("📌 Pinned packages:")
		for name := range pinned {
			fmt.Printf("  • %s\n", name)
		}
	},
}

func init() {
	rootCmd.AddCommand(pinCmd)
	rootCmd.AddCommand(unpinCmd)
	rootCmd.AddCommand(pinnedCmd)
}
