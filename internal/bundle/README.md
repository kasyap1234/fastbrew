# FastBrew Bundle Package

Package bundle provides Brewfile DSL parsing and generation for fastbrew.

## Overview

This package implements a parser for the Homebrew Bundle DSL (Domain Specific Language), which allows users to specify dependencies in a Brewfile. The parser converts Brewfile syntax into an Abstract Syntax Tree (AST) that can be executed or dumped.

## Brewfile Syntax Support

### Commands

The parser supports the following Brewfile commands:

#### brew
Install a formula package:
```ruby
brew "package-name"
brew "package-name", args: ["--with-feature"]
brew "package-name", args: {:branch => "main"}
brew "package-name", restart_service: true
```

#### cask
Install a cask (GUI application):
```ruby
cask "application-name"
cask "application-name", args: {appdir: "~/Applications"}
```

#### tap
Add a Homebrew tap:
```ruby
tap "user/repo"
tap "user/repo", "https://custom-url.git"
tap "user/repo", force: true
```

#### mas (Mac App Store)
Install Mac App Store applications:
```ruby
mas "App Name", id: 123456789
```

## AST Structure

The parser produces an AST consisting of the following node types:

### Node Interface

All AST nodes implement the `Node` interface:
```go
type Node interface {
    Position() Position
    Type() string
}
```

### Command Types

- **BrewCommand**: Represents `brew` installations
- **CaskCommand**: Represents `cask` installations  
- **TapCommand**: Represents `tap` additions
- **MasCommand**: Represents Mac App Store installations

### Brewfile Structure

The top-level `Brewfile` struct contains a slice of nodes:
```go
type Brewfile struct {
    Nodes []Node
    Path  string
}
```

## Usage

### Parsing a Brewfile

```go
parser := bundle.SimpleParser()
brewfile, err := parser.ParseFile("Brewfile")
if err != nil {
    log.Fatal(err)
}

// Access parsed commands
for _, brew := range brewfile.GetBrews() {
    fmt.Printf("Brew: %s\n", brew.Name)
}

for _, cask := range brewfile.GetCasks() {
    fmt.Printf("Cask: %s\n", cask.Name)
}
```

### Parsing from String

```go
content := `
brew "wget"
brew "git", args: ["--with-pcre2"]
cask "firefox"
tap "homebrew/cask-versions"
`

parser := bundle.SimpleParser()
brewfile, err := parser.ParseString(content)
```

## Error Handling

Parser errors include position information:

```go
if perr, ok := err.(*bundle.ParserError); ok {
    fmt.Printf("Error at %s: %s\n", perr.Pos, perr.Message)
}
```

Error types:
- `SyntaxError`: Invalid Brewfile syntax
- `UnsupportedCommandError`: Unknown command
- `InvalidArgumentError`: Invalid command arguments
- `IoError`: File reading error

## Parser Options

Configure the parser with options:

```go
opts := bundle.ParserOptions{
    Strict:               false,
    AllowUnknownCommands: false,
    PreserveComments:     true,
    MaxFileSize:          10 * 1024 * 1024, // 10MB
}
parser := bundle.NewParser(opts)
```

## Future Enhancements

- Full Ruby DSL parser (currently simplified)
- Whitespace preservation for round-trip editing
- Comment attachment to nodes
- Brewfile formatting/linting
