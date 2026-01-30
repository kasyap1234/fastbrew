package services

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type ServiceInfo struct {
	Label                string
	Program              string
	ProgramArgs          []string
	RunAtLoad            bool
	KeepAlive            bool
	StandardOutPath      string
	StandardErrorPath    string
	WorkingDirectory     string
	EnvironmentVariables map[string]string
}

type PlistParser struct{}

func NewPlistParser() *PlistParser {
	return &PlistParser{}
}

func (p *PlistParser) ParseFile(path string) (*ServiceInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, PlistNotFoundError{Path: path, Name: filepath.Base(path)}
		}
		return nil, fmt.Errorf("failed to read plist file: %w", err)
	}

	return p.Parse(data, path)
}

func (p *PlistParser) Parse(data []byte, sourcePath string) (*ServiceInfo, error) {
	content := string(data)

	labelRegex := regexp.MustCompile(`<key>Label</key>\s*<string>([^<]+)</string>`)
	programRegex := regexp.MustCompile(`<key>Program</key>\s*<string>([^<]+)</string>`)
	runAtLoadRegex := regexp.MustCompile(`<key>RunAtLoad</key>\s*<true\s*/?>`)
	stdoutRegex := regexp.MustCompile(`<key>StandardOutPath</key>\s*<string>([^<]+)</string>`)
	stderrRegex := regexp.MustCompile(`<key>StandardErrorPath</key>\s*<string>([^<]+)</string>`)
	workDirRegex := regexp.MustCompile(`<key>WorkingDirectory</key>\s*<string>([^<]+)</string>`)

	labelMatch := labelRegex.FindStringSubmatch(content)
	if len(labelMatch) < 2 {
		return nil, InvalidPlistError{
			Path:  sourcePath,
			Name:  filepath.Base(sourcePath),
			Cause: fmt.Errorf("missing required Label field"),
		}
	}

	info := &ServiceInfo{
		Label:                labelMatch[1],
		EnvironmentVariables: make(map[string]string),
	}

	if programMatch := programRegex.FindStringSubmatch(content); len(programMatch) >= 2 {
		info.Program = programMatch[1]
	}

	info.RunAtLoad = runAtLoadRegex.MatchString(content)

	if stdoutMatch := stdoutRegex.FindStringSubmatch(content); len(stdoutMatch) >= 2 {
		info.StandardOutPath = stdoutMatch[1]
	}

	if stderrMatch := stderrRegex.FindStringSubmatch(content); len(stderrMatch) >= 2 {
		info.StandardErrorPath = stderrMatch[1]
	}

	if workDirMatch := workDirRegex.FindStringSubmatch(content); len(workDirMatch) >= 2 {
		info.WorkingDirectory = workDirMatch[1]
	}

	return info, nil
}

func GetServiceNameFromPath(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if ext == ".plist" || ext == ".service" {
		return strings.TrimSuffix(base, ext)
	}
	return base
}

func IsHomebrewService(name string) bool {
	lowerName := strings.ToLower(name)
	return strings.Contains(lowerName, "homebrew") || strings.Contains(lowerName, "brew")
}

type CommandRunner interface {
	Run(name string, arg ...string) ([]byte, error)
	RunWithStdin(name string, stdin io.Reader, arg ...string) ([]byte, error)
}

type DefaultCommandRunner struct{}

func (d *DefaultCommandRunner) Run(name string, arg ...string) ([]byte, error) {
	cmd := exec.Command(name, arg...)
	return cmd.Output()
}

func (d *DefaultCommandRunner) RunWithStdin(name string, stdin io.Reader, arg ...string) ([]byte, error) {
	cmd := exec.Command(name, arg...)
	cmd.Stdin = stdin
	return cmd.Output()
}
