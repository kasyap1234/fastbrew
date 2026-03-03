package brew

import "time"

type TapFormulaRef struct {
	Ref     string
	FullRef string
	TapName string
	Formula string
}

type TapFormulaResolver struct {
	tapManager *TapManager
	tapsCache  map[string][]string
}

func NewTapFormulaResolver(tm *TapManager) *TapFormulaResolver {
	return &TapFormulaResolver{
		tapManager: tm,
		tapsCache:  make(map[string][]string),
	}
}

type ResolvedFormula struct {
	Name        string
	FullRef     string
	TapName     string
	LocalPath   string
	FormulaPath string
	IsCore      bool
}

type ResolveError struct {
	Ref        string
	Candidates []string
}

func (e *ResolveError) Error() string {
	if len(e.Candidates) > 0 {
		return "ambiguous formula: " + e.Ref + " (candidates: " + joinStrings(e.Candidates) + ")"
	}
	return "formula not found: " + e.Ref
}

func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}

type FormulaNotFoundError struct {
	Ref string
}

func (e *FormulaNotFoundError) Error() string {
	return "formula not found: " + e.Ref
}

type TapFormulaMetadata struct {
	Name        string
	FullName    string
	Version     string
	Revision    int
	Description string
	Homepage    string

	DependsOn       []string
	RuntimeDeps     []string
	BuildDeps       [][]string
	OptionalDeps    []string
	RecommendedDeps []string

	OnMacOS  bool
	OnLinux  bool
	CPUIntel *bool
	CPUArm   *bool
	CPU64Bit *bool

	InstallBlock  string
	InstallMethod string
	BinaryBottle  *BottleInfo
	RootURL       string
	SHA256s       map[string]string

	BinFiles        []InstallDirective
	SbinFiles       []InstallDirective
	LibexecFiles    []InstallDirective
	BashCompletions []InstallDirective
	ZshCompletions  []InstallDirective
	FishCompletions []InstallDirective
	ManPages        []InstallDirective

	KegOnly       bool
	UsesFromMacos []string

	UnsupportedStanzas []string
}

type InstallDirective struct {
	Source      string
	Destination string
	Renames     map[string]string
}

type BottleInfo struct {
	RootURL string
	Rebuild int
	SHA256  map[string]map[string]string
	Disable bool
}

type UnsupportedError struct {
	Formula string
	Stanzas []string
}

func (e *UnsupportedError) Error() string {
	return "unsupported formula: " + e.Formula + " (unsupported: " + joinStrings(e.Stanzas) + ")"
}

type TapPackage struct {
	FormulaMetadata *TapFormulaMetadata
	Resolved        *ResolvedFormula
}

func NewTapPackage() *TapPackage {
	return &TapPackage{
		FormulaMetadata: &TapFormulaMetadata{
			SHA256s:     make(map[string]string),
			RuntimeDeps: make([]string, 0),
			BuildDeps:   make([][]string, 0),
		},
	}
}

type InstalledTapPackage struct {
	Name        string
	Version     string
	InstalledAt time.Time
	CellarPath  string
	Linked      bool
	LinkedPath  string
}
