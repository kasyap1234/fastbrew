package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type CommandCase struct {
	Name     string
	FastBrew []string
	Homebrew []string
}

type CommandStats struct {
	Name        string
	Mode        string
	FastBrewP50 time.Duration
	FastBrewP95 time.Duration
	HomebrewP50 time.Duration
	HomebrewP95 time.Duration
}

var readCommands = []CommandCase{
	{Name: "list", FastBrew: []string{"list"}, Homebrew: []string{"list"}},
	{Name: "search", FastBrew: []string{"search", "curl"}, Homebrew: []string{"search", "curl"}},
	{Name: "outdated", FastBrew: []string{"outdated"}, Homebrew: []string{"outdated"}},
	{Name: "info", FastBrew: []string{"info", "curl"}, Homebrew: []string{"info", "curl"}},
}

func main() {
	var (
		runs     = flag.Int("runs", 7, "number of runs per command/mode")
		mode     = flag.String("mode", "all", "benchmark mode: cold, warm, all")
		fastbrew = flag.String("fastbrew", "./fastbrew", "path to fastbrew binary")
		homebrew = flag.String("brew", "", "path to brew binary (auto-detect if empty)")
		project  = flag.String("project-root", ".", "project root for running fastbrew")
		showRaw  = flag.Bool("raw", false, "print raw samples for each command")
		skipBrew = flag.Bool("skip-brew", false, "skip brew baseline runs")
	)
	flag.Parse()

	if *runs < 3 {
		fmt.Println("runs must be >= 3")
		os.Exit(1)
	}

	projectRoot, err := filepath.Abs(*project)
	if err != nil {
		fmt.Printf("failed to resolve project root: %v\n", err)
		os.Exit(1)
	}

	brewPath := *homebrew
	if brewPath == "" && !*skipBrew {
		if detected, detectErr := exec.LookPath("brew"); detectErr == nil {
			brewPath = detected
		}
	}

	modes := resolveModes(*mode)
	if len(modes) == 0 {
		fmt.Printf("invalid mode %q (expected cold, warm, or all)\n", *mode)
		os.Exit(1)
	}

	fmt.Println("FastBrew benchmark (multi-run p50/p95)")
	fmt.Printf("Runs per command: %d\n", *runs)
	fmt.Printf("Modes: %s\n", strings.Join(modes, ", "))
	if brewPath == "" || *skipBrew {
		fmt.Println("Homebrew baseline: disabled")
	} else {
		fmt.Printf("Homebrew baseline: %s\n", brewPath)
	}
	fmt.Println()

	var summary []CommandStats
	for _, benchMode := range modes {
		fmt.Printf("=== %s ===\n", strings.ToUpper(benchMode))
		if benchMode == "warm" {
			_ = warmupCaches(*fastbrew, projectRoot)
		}

		for _, tc := range readCommands {
			if benchMode == "warm" {
				_ = runCommand(*fastbrew, projectRoot, tc.FastBrew...)
				if brewPath != "" && !*skipBrew {
					_ = runCommand(brewPath, projectRoot, tc.Homebrew...)
				}
			}

			fbSamples := runSamples(*runs, benchMode, projectRoot, *fastbrew, tc.FastBrew, true)
			var hbSamples []time.Duration
			if brewPath != "" && !*skipBrew {
				hbSamples = runSamples(*runs, benchMode, projectRoot, brewPath, tc.Homebrew, false)
			}

			stats := CommandStats{
				Name:        tc.Name,
				Mode:        benchMode,
				FastBrewP50: percentileDuration(fbSamples, 50),
				FastBrewP95: percentileDuration(fbSamples, 95),
			}
			if len(hbSamples) > 0 {
				stats.HomebrewP50 = percentileDuration(hbSamples, 50)
				stats.HomebrewP95 = percentileDuration(hbSamples, 95)
			}
			summary = append(summary, stats)

			fmt.Printf("%-10s fastbrew p50=%-9v p95=%-9v", tc.Name, stats.FastBrewP50, stats.FastBrewP95)
			if len(hbSamples) > 0 {
				fmt.Printf(" | brew p50=%-9v p95=%-9v", stats.HomebrewP50, stats.HomebrewP95)
				if stats.FastBrewP50 > 0 && stats.HomebrewP50 > 0 {
					speedupP50 := float64(stats.HomebrewP50) / float64(stats.FastBrewP50)
					speedupP95 := float64(stats.HomebrewP95) / float64(stats.FastBrewP95)
					fmt.Printf(" | speedup p50=%.2fx p95=%.2fx", speedupP50, speedupP95)
				}
			}
			fmt.Println()

			if *showRaw {
				fmt.Printf("  raw fastbrew: %s\n", formatSamples(fbSamples))
				if len(hbSamples) > 0 {
					fmt.Printf("  raw brew:     %s\n", formatSamples(hbSamples))
				}
			}
		}
		fmt.Println()
	}

	fmt.Println("Summary")
	fmt.Println("-------")
	for _, item := range summary {
		line := fmt.Sprintf("%s/%s fastbrew p50=%v p95=%v", item.Mode, item.Name, item.FastBrewP50, item.FastBrewP95)
		if item.HomebrewP50 > 0 {
			line += fmt.Sprintf(" | brew p50=%v p95=%v", item.HomebrewP50, item.HomebrewP95)
		}
		fmt.Println(line)
	}
}

func runSamples(runs int, mode, projectRoot, bin string, args []string, clearFastbrewCache bool) []time.Duration {
	samples := make([]time.Duration, 0, runs)
	for i := 0; i < runs; i++ {
		if mode == "cold" && clearFastbrewCache {
			_ = clearFastbrewCaches()
		}
		start := time.Now()
		if err := runCommand(bin, projectRoot, args...); err != nil {
			samples = append(samples, 0)
			continue
		}
		samples = append(samples, time.Since(start))
	}
	return filterNonZero(samples)
}

func runCommand(bin, dir string, args ...string) error {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func clearFastbrewCaches() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cacheDir := filepath.Join(home, ".fastbrew", "cache")
	return os.RemoveAll(cacheDir)
}

func warmupCaches(fastbrewPath, projectRoot string) error {
	if err := runCommand(fastbrewPath, projectRoot, "update"); err != nil {
		return err
	}
	for _, tc := range readCommands {
		if err := runCommand(fastbrewPath, projectRoot, tc.FastBrew...); err != nil {
			continue
		}
	}
	return nil
}

func resolveModes(raw string) []string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "cold":
		return []string{"cold"}
	case "warm":
		return []string{"warm"}
	case "all":
		return []string{"cold", "warm"}
	default:
		return nil
	}
}

func percentileDuration(samples []time.Duration, p int) time.Duration {
	if len(samples) == 0 {
		return 0
	}
	copied := make([]time.Duration, len(samples))
	copy(copied, samples)
	sort.Slice(copied, func(i, j int) bool { return copied[i] < copied[j] })

	if p <= 0 {
		return copied[0]
	}
	if p >= 100 {
		return copied[len(copied)-1]
	}

	rank := int(math.Ceil((float64(p) / 100.0) * float64(len(copied))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(copied) {
		rank = len(copied)
	}
	return copied[rank-1]
}

func filterNonZero(samples []time.Duration) []time.Duration {
	filtered := make([]time.Duration, 0, len(samples))
	for _, sample := range samples {
		if sample > 0 {
			filtered = append(filtered, sample)
		}
	}
	return filtered
}

func formatSamples(samples []time.Duration) string {
	parts := make([]string, 0, len(samples))
	for _, sample := range samples {
		parts = append(parts, sample.String())
	}
	return strings.Join(parts, ", ")
}
