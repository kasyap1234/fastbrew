package bundle

import (
	"fmt"
	"testing"
)

func TestPosition_String(t *testing.T) {
	pos := Position{Line: 1, Column: 5}
	expected := "line 1, column 5"
	if pos.String() != expected {
		t.Errorf("Expected %q, got %q", expected, pos.String())
	}
}

func TestBrewfile_Getters(t *testing.T) {
	bf := &Brewfile{
		Nodes: []Node{
			&BrewCommand{Name: "wget", Pos: Position{Line: 1}},
			&CaskCommand{Name: "iterm2", Pos: Position{Line: 2}},
			&TapCommand{User: "homebrew", Repo: "bundle", Pos: Position{Line: 3}},
			&MasCommand{Name: "Xcode", ID: 497799835, Pos: Position{Line: 4}},
			&WhitespaceCommand{Content: "# comment", Pos: Position{Line: 5}},
		},
	}

	brews := bf.GetBrews()
	if len(brews) != 1 || brews[0].Name != "wget" {
		t.Errorf("GetBrews failed, got %v", brews)
	}

	casks := bf.GetCasks()
	if len(casks) != 1 || casks[0].Name != "iterm2" {
		t.Errorf("GetCasks failed, got %v", casks)
	}

	taps := bf.GetTaps()
	if len(taps) != 1 || taps[0].User != "homebrew" {
		t.Errorf("GetTaps failed, got %v", taps)
	}

	apps := bf.GetMasApps()
	if len(apps) != 1 || apps[0].Name != "Xcode" {
		t.Errorf("GetMasApps failed, got %v", apps)
	}

	pkgs := bf.GetAllPackages()
	if len(pkgs) != 3 { // brew, cask, mas
		t.Errorf("GetAllPackages failed, got %d packages", len(pkgs))
	}
}

func TestToReference(t *testing.T) {
	brew := &BrewCommand{Name: "wget", Args: map[string]interface{}{"restart_service": true}}
	ref, ok := ToReference(brew)
	if !ok || ref.Name != "wget" || ref.Type != "brew" || ref.Args["restart_service"] != true {
		t.Errorf("ToReference(brew) failed: %v, %v", ref, ok)
	}

	cask := &CaskCommand{Name: "iterm2"}
	ref, ok = ToReference(cask)
	if !ok || ref.Name != "iterm2" || ref.Type != "cask" {
		t.Errorf("ToReference(cask) failed")
	}

	mas := &MasCommand{Name: "Xcode", ID: 123}
	ref, ok = ToReference(mas)
	if !ok || ref.Name != "Xcode" || ref.Type != "mas" {
		t.Errorf("ToReference(mas) failed")
	}

	ws := &WhitespaceCommand{}
	_, ok = ToReference(ws)
	if ok {
		t.Errorf("ToReference(whitespace) should return false")
	}
}

func TestParserError(t *testing.T) {
	pe := &ParserError{
		Pos:     Position{Line: 10, Column: 1},
		Message: "unexpected token",
		Type:    SyntaxError,
	}
	expected := "SyntaxError at line 10, column 1: unexpected token"
	if pe.Error() != expected {
		t.Errorf("Expected %q, got %q", expected, pe.Error())
	}

	if !IsSyntaxError(pe) {
		t.Error("IsSyntaxError should be true")
	}
	if IsUnsupportedCommand(pe) {
		t.Error("IsUnsupportedCommand should be false")
	}

	otherErr := fmt.Errorf("other error")
	if IsSyntaxError(otherErr) {
		t.Error("IsSyntaxError should be false for generic error")
	}
}
