package cmd

import (
	"testing"
)

func TestRootCommand(t *testing.T) {
	if rootCmd.Use != "fastbrew" {
		t.Errorf("Expected root command use 'fastbrew', got %q", rootCmd.Use)
	}

	subCommands := rootCmd.Commands()
	foundInstall := false
	for _, cmd := range subCommands {
		if cmd.Name() == "install" {
			foundInstall = true
			break
		}
	}

	if !foundInstall {
		t.Error("Expected 'install' subcommand to be registered")
	}
}

func TestInstallFlags(t *testing.T) {
	cmd, _, _ := rootCmd.Find([]string{"install"})
	if cmd == nil {
		t.Fatal("Could not find install command")
	}

	progressFlag := cmd.Flags().Lookup("progress")
	if progressFlag == nil {
		t.Error("Expected 'progress' flag on install command")
	} else if progressFlag.Shorthand != "p" {
		t.Errorf("Expected shorthand 'p' for progress flag, got %q", progressFlag.Shorthand)
	}

	verboseFlag := cmd.Flags().Lookup("verbose")
	if verboseFlag == nil {
		t.Error("Expected 'verbose' flag on install command")
	}
}

func TestCommandRegistration(t *testing.T) {
	expectedSubCommands := []string{
		"install", "uninstall", "update", "upgrade", "search", "list",
		"info", "deps", "leaves", "doctor", "tap", "services", "bundle",
		"cleanup", "pin", "reinstall", "autoremove", "sh", "link",
	}

	for _, name := range expectedSubCommands {
		cmd, _, _ := rootCmd.Find([]string{name})
		if cmd == nil || cmd.Name() != name {
			t.Errorf("Subcommand %q not registered correctly", name)
		}
	}
}
