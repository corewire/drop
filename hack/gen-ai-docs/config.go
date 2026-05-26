package main

import "path/filepath"

// ─── User Configuration ──────────────────────────────────────────────────────
//
// This file contains all human-editable configuration for the doc generator.
// Edit this file to:
//   - Change project metadata
//   - Add/remove coding conventions
//   - Add/remove output targets (generated files)
//   - Adjust static relationships that can't be auto-detected
//   - Update sample group descriptions

// projectConfig returns the project metadata. Edit here when the project identity changes.
func projectConfig(goVersion, module string) Project {
	return Project{
		Name:        "drop",
		Description: "Kubernetes operator that pre-caches container images on cluster nodes",
		APIGroup:    "drop.corewire.io/v1alpha1",
		GoVersion:   goVersion,
		Module:      module,
		License:     "MIT",
	}
}

// conventions returns the project's coding conventions.
// Scope values: "code" (for CODE agents), "use" (for USE agents), "both" (shown everywhere).
func conventions() []Convention {
	return []Convention{
		{Rule: "No privileged containers — kubelet-based image pulls only", Scope: []string{"code"}},
		{Rule: "Single responsibility reconcilers — one controller per CRD", Scope: []string{"code"}},
		{Rule: "Pod builder is a pure function in internal/podbuilder/ (no k8s client)", Scope: []string{"code"}},
		{Rule: "Pacing logic lives exclusively in internal/pacing/", Scope: []string{"code"}},
		{Rule: "Don't manually edit generated files — run make docs-gen", Scope: []string{"code"}},
	}
}

// outputTargets returns all files to generate and which template to use for each.
// Add new entries here to generate additional output files.
func outputTargets() []OutputTarget {
	return []OutputTarget{
		// USE agents (consumed by LLMs exploring the project)
		{Path: "llms.txt", Template: llmsTxtTmpl},
		{Path: "llms-full.txt", Template: llmsFullTxtTmpl},
		{Path: filepath.Join("docs", "static", "llms-full.txt"), Template: llmsFullTxtTmpl},

		// CODE agents (consumed by IDE coding assistants)
		{Path: filepath.Join(".github", "copilot-instructions.md"), Template: copilotInstructionsTmpl},
		{Path: ".cursorrules", Template: cursorRulesTmpl},
		{Path: "AGENTS.md", Template: agentsMdTmpl},

		// HUMANS (Hugo reference pages)
		{Path: filepath.Join("docs", "content", "docs", "reference", "_generated_crds.md"), Template: hugoCRDsTmpl},
		{Path: filepath.Join("docs", "content", "docs", "reference", "_generated_errors.md"), Template: hugoErrorsTmpl},
		{Path: filepath.Join("docs", "content", "docs", "reference", "_generated_metrics.md"), Template: hugoMetricsTmpl},
		{Path: filepath.Join("docs", "content", "docs", "reference", "_generated_architecture.md"), Template: hugoArchTmpl},
	}
}
