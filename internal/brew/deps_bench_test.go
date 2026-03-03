package brew

import (
	"fmt"
	"testing"
)

func BenchmarkResolveDeps(b *testing.B) {
	const packageCount = 400

	formulae := make([]Formula, 0, packageCount)
	for i := 0; i < packageCount; i++ {
		name := fmt.Sprintf("pkg-%d", i)
		deps := []string{}
		if i > 0 {
			deps = append(deps, fmt.Sprintf("pkg-%d", i-1))
		}
		if i > 1 {
			deps = append(deps, fmt.Sprintf("pkg-%d", i-2))
		}

		formulae = append(formulae, Formula{
			Name:         name,
			Versions:     FormulaVersions{Stable: "1.0.0"},
			Dependencies: deps,
		})
	}

	client := &Client{
		index: &Index{
			Formulae: formulae,
			Casks:    []Cask{},
		},
	}

	target := []string{"pkg-399", "pkg-355", "pkg-275"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		deps, err := client.ResolveDeps(target)
		if err != nil {
			b.Fatalf("ResolveDeps failed: %v", err)
		}
		if len(deps) == 0 {
			b.Fatal("ResolveDeps returned no dependencies")
		}
	}
}

func BenchmarkVersionCompare(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = versionCompare("3.12.1_1", "3.13.0")
		_ = versionCompare("1.0.0", "1.0.0")
		_ = versionCompare("10.2.0", "9.8.4")
	}
}
