package brew

import (
	"testing"
)

func TestOutdatedPackageFiltering(t *testing.T) {
	packages := []OutdatedPackage{
		{Name: "pkg1", CurrentVersion: "1.0", NewVersion: "1.1", IsCask: false},
		{Name: "pkg2", CurrentVersion: "2.0", NewVersion: "2.1", IsCask: true},
		{Name: "pkg3", CurrentVersion: "3.0", NewVersion: "3.0", IsCask: false},
	}

	var formulae, casks []OutdatedPackage
	for _, pkg := range packages {
		if pkg.IsCask {
			casks = append(casks, pkg)
		} else {
			formulae = append(formulae, pkg)
		}
	}

	if len(formulae) != 2 {
		t.Errorf("Expected 2 formulae, got %d", len(formulae))
	}
	if len(casks) != 1 {
		t.Errorf("Expected 1 cask, got %d", len(casks))
	}
}

func TestOutdatedPackageSort(t *testing.T) {
	packages := []OutdatedPackage{
		{Name: "zebra", CurrentVersion: "1.0", NewVersion: "1.1", IsCask: false},
		{Name: "alpha", CurrentVersion: "2.0", NewVersion: "2.1", IsCask: false},
		{Name: "mango", CurrentVersion: "3.0", NewVersion: "3.0", IsCask: true},
	}

	sorted := make([]OutdatedPackage, len(packages))
	copy(sorted, packages)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[j].Name < sorted[i].Name {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	if sorted[0].Name != "alpha" {
		t.Errorf("Expected alpha first, got %s", sorted[0].Name)
	}
	if sorted[1].Name != "mango" {
		t.Errorf("Expected mango second, got %s", sorted[1].Name)
	}
	if sorted[2].Name != "zebra" {
		t.Errorf("Expected zebra third, got %s", sorted[2].Name)
	}
}
