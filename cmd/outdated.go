package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	outdatedQuiet bool
	outdatedJSON  bool
)

var outdatedCmd = &cobra.Command{
	Use:   "outdated",
	Short: "List outdated packages (faster than brew outdated)",
	Run: func(cmd *cobra.Command, args []string) {
		var outdated []OutdatedView

		if daemonClient, daemonErr := getDaemonClientForRead(); daemonClient != nil {
			daemonOutdated, err := daemonClient.Outdated()
			if err == nil {
				outdated = make([]OutdatedView, len(daemonOutdated))
				for i, item := range daemonOutdated {
					outdated[i] = OutdatedView{
						Name:           item.Name,
						CurrentVersion: item.CurrentVersion,
						NewVersion:     item.NewVersion,
						IsCask:         item.IsCask,
					}
				}
			} else {
				warnDaemonFallback("outdated", err)
			}
		} else if daemonErr != nil {
			warnDaemonFallback("outdated", daemonErr)
		}

		if outdated == nil {
			client, err := newBrewClient()
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			localOutdated, outdatedErr := client.GetOutdated()
			if outdatedErr != nil {
				fmt.Printf("Error checking for outdated packages: %v\n", outdatedErr)
				os.Exit(1)
			}
			outdated = make([]OutdatedView, len(localOutdated))
			for i, item := range localOutdated {
				outdated[i] = OutdatedView{
					Name:           item.Name,
					CurrentVersion: item.CurrentVersion,
					NewVersion:     item.NewVersion,
					IsCask:         item.IsCask,
				}
			}
		}

		if len(outdated) == 0 {
			os.Exit(0)
		}

		if outdatedJSON {
			output, err := json.MarshalIndent(outdated, "", "  ")
			if err != nil {
				fmt.Printf("Error encoding JSON: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(string(output))
		} else if outdatedQuiet {
			for _, pkg := range outdated {
				fmt.Println(pkg.Name)
			}
		} else {
			for _, pkg := range outdated {
				fmt.Printf("%s (%s) < %s\n", pkg.Name, pkg.CurrentVersion, pkg.NewVersion)
			}
		}

	},
}

type OutdatedView struct {
	Name           string `json:"name"`
	CurrentVersion string `json:"current_version"`
	NewVersion     string `json:"new_version"`
	IsCask         bool   `json:"is_cask"`
}

func init() {
	outdatedCmd.Flags().BoolVarP(&outdatedQuiet, "quiet", "q", false, "Only display names of outdated packages")
	outdatedCmd.Flags().BoolVar(&outdatedJSON, "json", false, "Output in JSON format")
	rootCmd.AddCommand(outdatedCmd)
}
