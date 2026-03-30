package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var descCmd = &cobra.Command{
	Use:   "desc [formula|cask...]",
	Short: "Display formula/cask description",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := newBrewClient()
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		idx, err := client.LoadIndex()
		if err != nil {
			fmt.Printf("Error loading index: %v\n", err)
			os.Exit(1)
		}

		// Build lookup maps
		formulaMap := make(map[string]string)
		for _, f := range idx.Formulae {
			formulaMap[f.Name] = f.Desc
		}

		caskMap := make(map[string]string)
		for _, c := range idx.Casks {
			caskMap[c.Token] = c.Desc
		}

		// Look up descriptions
		for _, name := range args {
			if desc, ok := formulaMap[name]; ok {
				fmt.Printf("%s: %s\n", name, desc)
			} else if desc, ok := caskMap[name]; ok {
				fmt.Printf("%s: %s\n", name, desc)
			} else {
				// Try to fetch from API
				desc := fetchDescription(name)
				if desc != "" {
					fmt.Printf("%s: %s\n", name, desc)
				} else {
					fmt.Printf("%s: No description available\n", name)
				}
			}
		}
	},
}

func fetchDescription(name string) string {
	// Try formula API
	url := fmt.Sprintf("https://formulae.brew.sh/api/formula/%s.json", name)
	desc := tryFetchDesc(url, "desc")
	if desc != "" {
		return desc
	}

	// Try cask API
	url = fmt.Sprintf("https://formulae.brew.sh/api/cask/%s.json", name)
	return tryFetchDesc(url, "desc")
}

func tryFetchDesc(url, field string) string {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ""
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}

	if val, ok := result[field].(string); ok {
		return val
	}
	return ""
}

func init() {
	rootCmd.AddCommand(descCmd)
}
