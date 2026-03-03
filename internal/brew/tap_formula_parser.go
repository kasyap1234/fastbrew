package brew

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	reVersion        = regexp.MustCompile(`^\s*version\s+['"]([^'"]+)['"]`)
	reRevision       = regexp.MustCompile(`^\s*revision\s+(\d+)`)
	reDependsOn      = regexp.MustCompile(`^\s*depends_on\s+(.+)$`)
	reOnMacOS        = regexp.MustCompile(`^\s*on_macos\s+`)
	reOnLinux        = regexp.MustCompile(`^\s*on_linux\s+`)
	reHardwareIntel  = regexp.MustCompile(`Hardware::CPU\.intel\?`)
	reHardwareArm    = regexp.MustCompile(`Hardware::CPU\.arm\?`)
	reHardware64Bit  = regexp.MustCompile(`Hardware::CPU\.is_64_bit\?`)
	reBottleBlock    = regexp.MustCompile(`^\s*bottle\s+do`)
	reBottleRootURL  = regexp.MustCompile(`root_url\s+['"]([^'"]+)['"]`)
	reBottleRebuild  = regexp.MustCompile(`rebuild\s+(\d+)`)
	reBottleSHA256   = regexp.MustCompile(`sha256\s+['"]([a-f0-9]+)['"]\s+=>\s+['"]([^'"]+)['"]:`)
	reBinInstall     = regexp.MustCompile(`^\s*bin\.install(?:\s+(.+))?$`)
	reSbinInstall    = regexp.MustCompile(`^\s*sbin\.install(?:\s+(.+))?$`)
	reLibexecInstall = regexp.MustCompile(`^\s*libexec\.install(?:\s+(.+))?$`)
	reBashComp       = regexp.MustCompile(`^\s*bash_completion\.install(?:\s+(.+))?$`)
	reZshComp        = regexp.MustCompile(`^\s*zsh_completion\.install(?:\s+(.+))?$`)
	reFishComp       = regexp.MustCompile(`^\s*fish_completion\.install(?:\s+(.+))?$`)
	reManPage        = regexp.MustCompile(`^\s*man(\d)\.install(?:\s+(.+))?$`)
	reInstallBlock   = regexp.MustCompile(`^\s*def\s+install\s*$`)
	reInstallMethod  = regexp.MustCompile(`^\s*define_method\s*\(\s*[:"]?install[:"]?\s*\)`)
	reKegOnly        = regexp.MustCompile(`^\s*keg_only\s+['"]?`)
)

type ParserState int

const (
	StateNormal ParserState = iota
	StateBottleBlock
	StateOnMacOS
	StateOnLinux
	StateDependsOn
)

func ParseTapFormula(formulaPath string) (*TapFormulaMetadata, error) {
	content, err := os.ReadFile(formulaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read formula: %w", err)
	}

	meta := &TapFormulaMetadata{
		SHA256s:         make(map[string]string),
		RuntimeDeps:     make([]string, 0),
		BuildDeps:       make([][]string, 0),
		OptionalDeps:    make([]string, 0),
		RecommendedDeps: make([]string, 0),
		OnMacOS:         true,
		OnLinux:         true,
	}

	if err := parseFormulaContent(string(content), meta); err != nil {
		return nil, err
	}

	return meta, nil
}

func parseFormulaContent(content string, meta *TapFormulaMetadata) error {
	scanner := bufio.NewScanner(strings.NewReader(content))
	state := StateNormal
	lineNum := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		if strings.HasPrefix(line, "#") {
			continue
		}

		if reOnMacOS.MatchString(line) {
			state = StateOnMacOS
			onMacOS := true
			meta.OnMacOS = onMacOS
			meta.OnLinux = false
			continue
		}

		if reOnLinux.MatchString(line) {
			state = StateOnLinux
			onLinux := true
			meta.OnLinux = onLinux
			meta.OnMacOS = false
			continue
		}

		if strings.HasPrefix(line, "end") && state != StateBottleBlock {
			if state == StateOnMacOS || state == StateOnLinux {
				state = StateNormal
			}
			continue
		}

		if state == StateOnMacOS || state == StateOnLinux {
			if !isPlatformSupported(line, meta) {
				continue
			}
		}

		if strings.Contains(line, "resource ") || strings.Contains(line, "patch ") {
			if !sliceContains(meta.UnsupportedStanzas, "resource/patch") {
				meta.UnsupportedStanzas = append(meta.UnsupportedStanzas, "resource/patch")
			}
		}

		if strings.Contains(line, "system ") {
			if !sliceContains(meta.UnsupportedStanzas, "system") {
				meta.UnsupportedStanzas = append(meta.UnsupportedStanzas, "system")
			}
		}

		if strings.Contains(line, "virtualenv") || strings.Contains(line, "venv") {
			if !sliceContains(meta.UnsupportedStanzas, "virtualenv") {
				meta.UnsupportedStanzas = append(meta.UnsupportedStanzas, "virtualenv")
			}
		}

		if strings.Contains(line, "test ") {
			if !sliceContains(meta.UnsupportedStanzas, "test") {
				meta.UnsupportedStanzas = append(meta.UnsupportedStanzas, "test")
			}
		}

		if reBottleBlock.MatchString(line) {
			state = StateBottleBlock
			continue
		}

		if state == StateBottleBlock {
			if strings.HasPrefix(line, "end") {
				state = StateNormal
				continue
			}

			if match := reBottleRootURL.FindStringSubmatch(line); match != nil {
				meta.RootURL = match[1]
			}

			if match := reBottleRebuild.FindStringSubmatch(line); match != nil {
				fmt.Sscanf(match[1], "%d", &meta.Revision)
			}

			if matches := reBottleSHA256.FindAllStringSubmatch(line, -1); len(matches) > 0 {
				for _, m := range matches {
					if len(m) == 3 {
						tag := m[2]
						meta.SHA256s[tag] = m[1]
					}
				}
			}
			continue
		}

		if match := reVersion.FindStringSubmatch(line); match != nil {
			meta.Version = match[1]
			continue
		}

		if match := reRevision.FindStringSubmatch(line); match != nil {
			fmt.Sscanf(match[1], "%d", &meta.Revision)
			continue
		}

		if reDependsOn.MatchString(line) {
			dep := extractDependsOn(line)
			if dep != "" {
				meta.RuntimeDeps = append(meta.RuntimeDeps, dep)
			}
			continue
		}

		if reBinInstall.MatchString(line) {
			dirs := extractInstallArgs(line, "bin.install")
			for _, dir := range dirs {
				meta.BinFiles = append(meta.BinFiles, InstallDirective{Source: dir})
			}
			meta.InstallBlock = "bin"
		}

		if reSbinInstall.MatchString(line) {
			dirs := extractInstallArgs(line, "sbin.install")
			for _, dir := range dirs {
				meta.SbinFiles = append(meta.SbinFiles, InstallDirective{Source: dir})
			}
			meta.InstallBlock = "sbin"
		}

		if reLibexecInstall.MatchString(line) {
			dirs := extractInstallArgs(line, "libexec.install")
			for _, dir := range dirs {
				meta.LibexecFiles = append(meta.LibexecFiles, InstallDirective{Source: dir})
			}
			meta.InstallBlock = "libexec"
		}

		if reBashComp.MatchString(line) {
			dirs := extractInstallArgs(line, "bash_completion.install")
			for _, dir := range dirs {
				meta.BashCompletions = append(meta.BashCompletions, InstallDirective{Source: dir})
			}
		}

		if reZshComp.MatchString(line) {
			dirs := extractInstallArgs(line, "zsh_completion.install")
			for _, dir := range dirs {
				meta.ZshCompletions = append(meta.ZshCompletions, InstallDirective{Source: dir})
			}
		}

		if reFishComp.MatchString(line) {
			dirs := extractInstallArgs(line, "fish_completion.install")
			for _, dir := range dirs {
				meta.FishCompletions = append(meta.FishCompletions, InstallDirective{Source: dir})
			}
		}

		if match := reManPage.FindStringSubmatch(line); match != nil {
			manNum := match[1]
			dirs := extractInstallArgs(line, "man"+manNum+".install")
			for _, dir := range dirs {
				meta.ManPages = append(meta.ManPages, InstallDirective{Source: dir})
			}
		}

		if reInstallBlock.MatchString(line) || reInstallMethod.MatchString(line) {
			meta.InstallMethod = "custom"
			meta.InstallBlock = "custom"
		}

		if reKegOnly.MatchString(line) {
			meta.KegOnly = true
		}

		if strings.Contains(line, "uses_from_macos") {
			meta.UsesFromMacos = append(meta.UsesFromMacos, extractUsesFromMacos(line)...)
		}
	}

	meta.OnMacOS = true
	meta.OnLinux = true

	if meta.Version == "" {
		return fmt.Errorf("version not found in formula")
	}

	return nil
}

func extractDependsOn(line string) string {
	re := regexp.MustCompile(`['"]([^'"]+)['"]`)
	matches := re.FindAllStringSubmatch(line, -1)
	if len(matches) > 0 {
		dep := matches[0][1]
		if strings.Contains(line, ":build") || strings.Contains(line, ":test") {
			return ""
		}
		return dep
	}
	return ""
}

func extractInstallArgs(line, prefix string) []string {
	line = strings.TrimPrefix(line, prefix)
	line = strings.TrimSpace(line)

	if line == "" || line == "do" {
		return nil
	}

	line = strings.TrimPrefix(line, "do")
	line = strings.TrimSpace(line)
	line = strings.TrimSuffix(line, "end")
	line = strings.TrimSpace(line)

	var result []string
	if strings.HasPrefix(line, "[") {
		re := regexp.MustCompile(`['"]([^'"]+)['"]`)
		matches := re.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			result = append(result, m[1])
		}
	} else if strings.HasPrefix(line, "{") {
		parts := strings.Split(line, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			part = strings.Trim(part, "{} ")
			if part != "" {
				result = append(result, part)
			}
		}
	} else {
		re := regexp.MustCompile(`['"]([^'"]+)['"]`)
		matches := re.FindAllStringSubmatch(line, -1)
		for _, m := range matches {
			result = append(result, m[1])
		}
	}

	return result
}

func isPlatformSupported(line string, meta *TapFormulaMetadata) bool {
	if reHardwareIntel.MatchString(line) {
		meta.CPUIntel = boolPtr(false)
	}
	if reHardwareArm.MatchString(line) {
		meta.CPUArm = boolPtr(true)
	}
	if reHardware64Bit.MatchString(line) {
		meta.CPU64Bit = boolPtr(true)
	}

	return true
}

func sliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func boolPtr(b bool) *bool {
	return &b
}

func extractUsesFromMacos(line string) []string {
	re := regexp.MustCompile(`['"]([^'"]+)['"]`)
	matches := re.FindAllStringSubmatch(line, -1)
	var result []string
	for _, m := range matches {
		result = append(result, m[1])
	}
	return result
}

func ParseTapFormulaFromContent(content string) (*TapFormulaMetadata, error) {
	meta := &TapFormulaMetadata{
		SHA256s:         make(map[string]string),
		RuntimeDeps:     make([]string, 0),
		BuildDeps:       make([][]string, 0),
		OptionalDeps:    make([]string, 0),
		RecommendedDeps: make([]string, 0),
		OnMacOS:         true,
		OnLinux:         true,
	}

	if err := parseFormulaContent(content, meta); err != nil {
		return nil, err
	}

	return meta, nil
}

func ClassifyFormulaForNativeInstall(meta *TapFormulaMetadata) (bool, []string) {
	if meta == nil {
		return false, []string{"nil metadata"}
	}

	var unsupported []string

	if len(meta.UnsupportedStanzas) > 0 {
		unsupported = append(unsupported, meta.UnsupportedStanzas...)
	}

	if meta.InstallMethod != "bin" && meta.InstallMethod != "custom" && meta.InstallBlock == "" {
		unsupported = append(unsupported, "no install block")
	}

	if meta.RootURL == "" && len(meta.SHA256s) == 0 {
		unsupported = append(unsupported, "no bottle")
	}

	return len(unsupported) == 0, unsupported
}

func GetArchitectureTag(meta *TapFormulaMetadata) string {
	arch := "intel"
	if meta.CPUArm != nil && *meta.CPUArm {
		arch = "arm"
	} else if meta.CPUIntel != nil && !*meta.CPUIntel {
		arch = "arm"
	}

	if meta.CPU64Bit != nil && *meta.CPU64Bit {
		_ = "arm64"
	}

	if arch == "arm" {
		return "arm64"
	}
	return "x86_64"
}

func GetOSFromMeta(meta *TapFormulaMetadata) string {
	if meta.OnMacOS && !meta.OnLinux {
		return "darwin"
	}
	if meta.OnLinux && !meta.OnMacOS {
		return "linux"
	}
	return "any"
}

func GetBottleURL(meta *TapFormulaMetadata, version string) (string, bool) {
	if meta.RootURL == "" {
		return "", false
	}

	osTag := GetOSFromMeta(meta)

	if osTag == "any" {
		osTag = "darwin"
	}

	url := fmt.Sprintf("%s/%s/%s-%s.%s.bottle.tar.gz", meta.RootURL, version, meta.Name, version, osTag)

	if sha, ok := meta.SHA256s[osTag]; ok {
		meta.SHA256s[url] = sha
	}

	return url, true
}
