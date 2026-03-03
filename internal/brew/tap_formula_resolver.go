package brew

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (r *TapFormulaResolver) Resolve(pkg string) (*ResolvedFormula, error) {
	if strings.Contains(pkg, "/") {
		return r.resolveFullRef(pkg)
	}
	return r.resolveShortName(pkg)
}

func (r *TapFormulaResolver) resolveFullRef(pkg string) (*ResolvedFormula, error) {
	parts := strings.Split(pkg, "/")
	if len(parts) != 2 && len(parts) != 3 {
		return nil, fmt.Errorf("invalid full ref: %s", pkg)
	}

	if len(parts) == 2 {
		return r.resolveShortNameWithTap(parts[0], parts[1])
	}

	tapName := parts[0] + "/" + parts[1]
	formulaName := parts[2]

	tap, ok := r.tapManager.GetTap(tapName)
	if !ok {
		if err := r.tapManager.Tap(tapName, false); err != nil {
			return nil, fmt.Errorf("failed to tap %s: %w", tapName, err)
		}
		tap, _ = r.tapManager.GetTap(tapName)
	}

	if tap.LocalPath == "" {
		return nil, fmt.Errorf("tap not found: %s", tapName)
	}

	formulaPath := r.findFormulaPath(tap.LocalPath, formulaName)
	if formulaPath == "" {
		return nil, &FormulaNotFoundError{Ref: pkg}
	}

	return &ResolvedFormula{
		Name:        formulaName,
		FullRef:     pkg,
		TapName:     tapName,
		LocalPath:   tap.LocalPath,
		FormulaPath: formulaPath,
		IsCore:      false,
	}, nil
}

func (r *TapFormulaResolver) resolveShortNameWithTap(user, repo string) (*ResolvedFormula, error) {
	tapName := user + "/" + repo

	tap, ok := r.tapManager.GetTap(tapName)
	if !ok {
		if err := r.tapManager.Tap(tapName, false); err != nil {
			return nil, fmt.Errorf("failed to tap %s: %w", tapName, err)
		}
		tap, _ = r.tapManager.GetTap(tapName)
	}

	if tap.LocalPath == "" {
		return nil, fmt.Errorf("tap not found: %s", tapName)
	}

	return &ResolvedFormula{
		Name:        user + "/" + repo,
		FullRef:     tapName,
		TapName:     tapName,
		LocalPath:   tap.LocalPath,
		FormulaPath: tap.LocalPath,
		IsCore:      false,
	}, nil
}

func (r *TapFormulaResolver) resolveShortName(name string) (*ResolvedFormula, error) {
	taps, err := r.tapManager.ListTaps()
	if err != nil {
		return nil, fmt.Errorf("failed to list taps: %w", err)
	}

	var candidates []string

	for _, tap := range taps {
		formulaPath := r.findFormulaPath(tap.LocalPath, name)
		if formulaPath != "" {
			candidates = append(candidates, tap.Name+"/"+name)
		}
	}

	if len(candidates) == 0 {
		return nil, &FormulaNotFoundError{Ref: name}
	}

	if len(candidates) > 1 {
		return nil, &ResolveError{
			Ref:        name,
			Candidates: candidates,
		}
	}

	parts := strings.Split(candidates[0], "/")
	tapName := parts[0] + "/" + parts[1]

	tap, ok := r.tapManager.GetTap(tapName)
	if !ok {
		return nil, fmt.Errorf("tap not found: %s", tapName)
	}

	formulaPath := r.findFormulaPath(tap.LocalPath, name)
	if formulaPath == "" {
		return nil, &FormulaNotFoundError{Ref: name}
	}

	return &ResolvedFormula{
		Name:        name,
		FullRef:     tapName + "/" + name,
		TapName:     tapName,
		LocalPath:   tap.LocalPath,
		FormulaPath: formulaPath,
		IsCore:      false,
	}, nil
}

func (r *TapFormulaResolver) findFormulaPath(tapPath, name string) string {
	formulaName := name + ".rb"

	candidates := []string{
		filepath.Join(tapPath, "Formula", formulaName),
		filepath.Join(tapPath, "Formula", name, formulaName),
		filepath.Join(tapPath, formulaName),
		filepath.Join(tapPath, "Aliases", name),
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}

	return ""
}

func (r *TapFormulaResolver) ScanTapFormulas(tapName string) ([]string, error) {
	if cached, ok := r.tapsCache[tapName]; ok {
		return cached, nil
	}

	tap, ok := r.tapManager.GetTap(tapName)
	if !ok {
		return nil, fmt.Errorf("tap not found: %s", tapName)
	}

	if tap.LocalPath == "" {
		return nil, nil
	}

	var formulas []string

	formulaDirs := []string{
		filepath.Join(tap.LocalPath, "Formula"),
		tap.LocalPath,
	}

	for _, dir := range formulaDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if strings.HasSuffix(entry.Name(), ".rb") {
				name := strings.TrimSuffix(entry.Name(), ".rb")
				formulas = append(formulas, name)
			}
		}
	}

	aliasesDir := filepath.Join(tap.LocalPath, "Aliases")
	if entries, err := os.ReadDir(aliasesDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				formulas = append(formulas, entry.Name())
			}
		}
	}

	r.tapsCache[tapName] = formulas
	return formulas, nil
}
