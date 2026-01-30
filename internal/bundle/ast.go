package bundle

import (
	"fmt"
)

// Position represents a location in a Brewfile for error reporting
type Position struct {
	Line   int
	Column int
	Offset int
}

func (p Position) String() string {
	return fmt.Sprintf("line %d, column %d", p.Line, p.Column)
}

// Node is the interface for all AST nodes
type Node interface {
	Position() Position
	Type() string
}

// BrewCommand represents a "brew" command in a Brewfile
type BrewCommand struct {
	Pos  Position
	Name string
	Args map[string]interface{}
}

func (b *BrewCommand) Position() Position { return b.Pos }
func (b *BrewCommand) Type() string       { return "brew" }

// CaskCommand represents a "cask" command in a Brewfile
type CaskCommand struct {
	Pos  Position
	Name string
	Args map[string]interface{}
}

func (c *CaskCommand) Position() Position { return c.Pos }
func (c *CaskCommand) Type() string       { return "cask" }

// TapCommand represents a "tap" command in a Brewfile
type TapCommand struct {
	Pos    Position
	User   string
	Repo   string
	URL    string
	Force  bool
	Custom map[string]interface{}
}

func (t *TapCommand) Position() Position { return t.Pos }
func (t *TapCommand) Type() string       { return "tap" }

// MasCommand represents a "mas" (Mac App Store) command in a Brewfile
type MasCommand struct {
	Pos  Position
	Name string
	ID   int
	Args map[string]interface{}
}

func (m *MasCommand) Position() Position { return m.Pos }
func (m *MasCommand) Type() string       { return "mas" }

// WhitespaceCommand represents a blank line or comment (for preserving formatting)
type WhitespaceCommand struct {
	Pos     Position
	Content string // comment text or empty for blank line
}

func (w *WhitespaceCommand) Position() Position { return w.Pos }
func (w *WhitespaceCommand) Type() string       { return "whitespace" }

// Brewfile represents the entire parsed Brewfile
type Brewfile struct {
	Nodes []Node
	Path  string // original file path
}

// GetBrews returns all brew commands from the Brewfile
func (b *Brewfile) GetBrews() []*BrewCommand {
	var brews []*BrewCommand
	for _, node := range b.Nodes {
		if brew, ok := node.(*BrewCommand); ok {
			brews = append(brews, brew)
		}
	}
	return brews
}

// GetCasks returns all cask commands from the Brewfile
func (b *Brewfile) GetCasks() []*CaskCommand {
	var casks []*CaskCommand
	for _, node := range b.Nodes {
		if cask, ok := node.(*CaskCommand); ok {
			casks = append(casks, cask)
		}
	}
	return casks
}

// GetTaps returns all tap commands from the Brewfile
func (b *Brewfile) GetTaps() []*TapCommand {
	var taps []*TapCommand
	for _, node := range b.Nodes {
		if tap, ok := node.(*TapCommand); ok {
			taps = append(taps, tap)
		}
	}
	return taps
}

// GetMasApps returns all mas commands from the Brewfile
func (b *Brewfile) GetMasApps() []*MasCommand {
	var apps []*MasCommand
	for _, node := range b.Nodes {
		if app, ok := node.(*MasCommand); ok {
			apps = append(apps, app)
		}
	}
	return apps
}

// GetAllPackages returns all installable packages (brews + casks + mas apps)
func (b *Brewfile) GetAllPackages() []Node {
	var packages []Node
	for _, node := range b.Nodes {
		switch node.(type) {
		case *BrewCommand, *CaskCommand, *MasCommand:
			packages = append(packages, node)
		}
	}
	return packages
}

// PackageReference represents a reference to a package with its type
type PackageReference struct {
	Name string
	Type string // "brew", "cask", "mas"
	Args map[string]interface{}
	Pos  Position
}

// ToReference converts a command to a PackageReference
func ToReference(node Node) (PackageReference, bool) {
	switch n := node.(type) {
	case *BrewCommand:
		return PackageReference{Name: n.Name, Type: "brew", Args: n.Args, Pos: n.Pos}, true
	case *CaskCommand:
		return PackageReference{Name: n.Name, Type: "cask", Args: n.Args, Pos: n.Pos}, true
	case *MasCommand:
		return PackageReference{Name: n.Name, Type: "mas", Args: n.Args, Pos: n.Pos}, true
	default:
		return PackageReference{}, false
	}
}
