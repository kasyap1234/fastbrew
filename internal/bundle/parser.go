package bundle

import (
	"fmt"
	"io"
	"strings"
)

type Parser interface {
	Parse(r io.Reader) (*Brewfile, error)
	ParseFile(path string) (*Brewfile, error)
	ParseString(content string) (*Brewfile, error)
}

type ParserError struct {
	Pos     Position
	Message string
	Type    ErrorType
}

func (e *ParserError) Error() string {
	return fmt.Sprintf("%s at %s: %s", e.Type, e.Pos, e.Message)
}

type ErrorType string

const (
	SyntaxError             ErrorType = "SyntaxError"
	UnsupportedCommandError ErrorType = "UnsupportedCommand"
	InvalidArgumentError    ErrorType = "InvalidArgument"
	IoError                 ErrorType = "IOError"
)

func IsSyntaxError(err error) bool {
	if pe, ok := err.(*ParserError); ok {
		return pe.Type == SyntaxError
	}
	return false
}

func IsUnsupportedCommand(err error) bool {
	if pe, ok := err.(*ParserError); ok {
		return pe.Type == UnsupportedCommandError
	}
	return false
}

type ParserOptions struct {
	Strict               bool
	AllowUnknownCommands bool
	PreserveComments     bool
	MaxFileSize          int64
}

func DefaultParserOptions() ParserOptions {
	return ParserOptions{
		Strict:               false,
		AllowUnknownCommands: false,
		PreserveComments:     true,
		MaxFileSize:          10 * 1024 * 1024,
	}
}

func NewParser(opts ParserOptions) Parser {
	return &rubyParser{options: opts}
}

func SimpleParser() Parser {
	return NewParser(DefaultParserOptions())
}

type rubyParser struct {
	options ParserOptions
}

func (p *rubyParser) Parse(r io.Reader) (*Brewfile, error) {
	return nil, fmt.Errorf("parser implementation not yet available")
}

func (p *rubyParser) ParseFile(path string) (*Brewfile, error) {
	return nil, fmt.Errorf("parser implementation not yet available")
}

func (p *rubyParser) ParseString(content string) (*Brewfile, error) {
	return p.Parse(io.NopCloser(strings.NewReader(content)))
}
