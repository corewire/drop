// hack/gen-ai-docs generates all documentation from source code.
//
// It parses api/v1alpha1/*_types.go, internal/controller/*.go, internal/metrics/,
// Makefile, and go.mod to build a unified knowledge model. From that model it
// generates documentation for three audiences:
//   - USE agents: llms.txt, llms-full.txt
//   - CODE agents: .github/copilot-instructions.md, .cursorrules, AGENTS.md
//   - HUMANS: Hugo content pages (CRD reference, errors, metrics, architecture)
//
// File layout:
//   - main.go      — entry point and orchestration (you're here)
//   - config.go    — user-editable: project metadata, conventions, output targets
//   - types.go     — knowledge model structs
//   - extract.go   — source code parsing and data extraction
//   - render.go    — template rendering and file I/O
//   - templates.go — output templates (one per generated file)
//
// Usage: go run ./hack/gen-ai-docs/
package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	root := findRepoRoot()
	k := buildKnowledge(root)

	writeKnowledgeYAML(root, k)
	renderAll(root, k)

	fmt.Println("✓ Generated: knowledge.yaml + llms.txt + llms-full.txt + agent instructions + Hugo reference pages + examples + doc-generation.md")
}

// buildKnowledge assembles the full knowledge model from source code and config.
func buildKnowledge(root string) Knowledge {
	goVer, module := parseGoMod(filepath.Join(root, "go.mod"))

	crds, helpers := parseAllTypes(filepath.Join(root, "api", "v1alpha1"))
	samples := readFileStr(filepath.Join(root, "hack", "dev-samples.yaml"))

	return Knowledge{
		Project:       projectConfig(goVer, module),
		CRDs:          crds,
		HelperTypes:   helpers,
		Relationships: inferRelationships(crds),
		Packages:      extractPackages(root, module),
		Conventions:   conventions(),
		Errors:        extractErrorReasons(root),
		Metrics:       extractMetrics(filepath.Join(root, "internal", "metrics", "metrics.go")),
		MakeTargets:   extractMakeTargets(filepath.Join(root, "Makefile")),
		Samples:       samples,
		SampleGroups:  parseSampleGroups(samples, crds),
	}
}

// findRepoRoot walks up from cwd to find the directory containing go.mod.
func findRepoRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			fmt.Fprintln(os.Stderr, "error: cannot find repo root (no go.mod)")
			os.Exit(1)
		}
		dir = parent
	}
}
