package cmd

import (
	"fastbrew/internal/brew"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/sahilm/fuzzy"
	"github.com/spf13/cobra"
)

type searchSource struct {
	items []brew.SearchItem
}

func (s searchSource) String(i int) string {
	return s.items[i].Name + " " + s.items[i].Desc
}

func (s searchSource) Len() int {
	return len(s.items)
}

type matchedResult struct {
	item           brew.SearchItem
	score          int
	matchedIndexes []int
	nameLen        int
}

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Instant search for packages",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		client, err := brew.NewClient()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		items, err := client.GetSearchIndex()
		if err != nil {
			fmt.Printf("Error loading index: %v\n", err)
			os.Exit(1)
		}

		query := args[0]
		fmt.Printf("🔍 Searching for '%s'...\n", query)

		source := searchSource{items: items}
		matches := fuzzy.FindFrom(query, source)

		if len(matches) == 0 {
			fmt.Println("No matches found.")
			return
		}

		results := make([]matchedResult, 0, len(matches))
		for _, match := range matches {
			item := items[match.Index]
			nameLen := len(item.Name)
			results = append(results, matchedResult{
				item:           item,
				score:          match.Score,
				matchedIndexes: match.MatchedIndexes,
				nameLen:        nameLen,
			})
		}

		sort.Slice(results, func(i, j int) bool {
			return results[i].score > results[j].score
		})

		limit := 40
		if len(results) < limit {
			limit = len(results)
		}

		for i := 0; i < limit; i++ {
			displayResult(results[i])
		}

		if len(results) > 40 {
			fmt.Println("... and more results")
		}
	},
}

func displayResult(result matchedResult) {
	item := result.item
	nameLen := result.nameLen
	matchedIndexes := result.matchedIndexes

	emoji := "🍺"
	if item.IsCask {
		emoji = "🍷"
	}

	fmt.Printf("%s %s: %s\n", emoji, item.Name, item.Desc)

	highlightLine := buildHighlightLine(nameLen, len(item.Desc), matchedIndexes)

	if highlightLine != "" {
		fmt.Printf("   %s\n", highlightLine)
	}
}

func buildHighlightLine(nameLen, descLen int, matchedIndexes []int) string {
	if len(matchedIndexes) == 0 {
		return ""
	}

	totalLen := nameLen + 2 + descLen
	highlight := make([]byte, totalLen)
	for i := range highlight {
		highlight[i] = ' '
	}

	for _, idx := range matchedIndexes {
		if idx >= 0 && idx < totalLen {
			highlight[idx] = '^'
		}
	}

	return strings.TrimRight(string(highlight), " ")
}

func init() {
	rootCmd.AddCommand(searchCmd)
}
