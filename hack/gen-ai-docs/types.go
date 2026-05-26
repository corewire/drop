package main

// ─── Knowledge Model ─────────────────────────────────────────────────────────
//
// These types define the intermediate representation between source extraction
// and template rendering. Add fields here when you need new data in templates.

// Knowledge is the unified intermediate representation of the project.
type Knowledge struct {
	Project       Project       `yaml:"project"`
	CRDs          []CRD         `yaml:"crds"`
	HelperTypes   []TypeDef     `yaml:"helperTypes"`
	Relationships []Relation    `yaml:"relationships"`
	Packages      []Package     `yaml:"packages"`
	Conventions   []Convention  `yaml:"conventions"`
	Errors        []ErrorReason `yaml:"errors"`
	Metrics       []Metric      `yaml:"metrics"`
	MakeTargets   []MakeTarget  `yaml:"makeTargets"`
	Samples       string        `yaml:"samples"`
	SampleGroups  []SampleGroup `yaml:"sampleGroups,omitempty"`
}

// Project holds top-level project metadata.
type Project struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	APIGroup    string `yaml:"apiGroup"`
	GoVersion   string `yaml:"goVersion"`
	Module      string `yaml:"module"`
	License     string `yaml:"license"`
}

// CRD represents a custom resource definition detected from source.
type CRD struct {
	Kind         string   `yaml:"kind"`
	Doc          string   `yaml:"doc"`
	Scope        string   `yaml:"scope"`
	Controller   string   `yaml:"controller,omitempty"`
	TestFile     string   `yaml:"testFile,omitempty"`
	SpecFields   []Field  `yaml:"specFields,omitempty"`
	StatusFields []Field  `yaml:"statusFields,omitempty"`
	Markers      []string `yaml:"markers,omitempty"`
}

// TypeDef represents a helper type (not a CRD root).
type TypeDef struct {
	Name   string  `yaml:"name"`
	Doc    string  `yaml:"doc"`
	Fields []Field `yaml:"fields"`
}

// Field represents a single struct field in a CRD spec/status/helper type.
type Field struct {
	Name     string   `yaml:"name"`
	JSON     string   `yaml:"json"`
	Type     string   `yaml:"type"`
	Required bool     `yaml:"required"`
	Default  string   `yaml:"default,omitempty"`
	Enum     []string `yaml:"enum,omitempty"`
	Doc      string   `yaml:"doc"`
}

// Relation represents a relationship between CRDs or resources.
type Relation struct {
	From      string `yaml:"from"`
	To        string `yaml:"to"`
	Type      string `yaml:"type"`
	Mechanism string `yaml:"mechanism,omitempty"`
}

// Package represents an internal Go package with its role and imports.
type Package struct {
	Path    string   `yaml:"path"`
	Role    string   `yaml:"role"`
	Imports []string `yaml:"imports,omitempty"`
}

// Convention is a project rule that applies to a given scope (code, use, or both).
type Convention struct {
	Rule  string   `yaml:"rule"`
	Scope []string `yaml:"scope"`
}

// ErrorReason represents a condition reason emitted by a controller.
type ErrorReason struct {
	Reason          string `yaml:"reason"`
	Controller      string `yaml:"controller"`
	Meaning         string `yaml:"meaning"`
	Troubleshooting string `yaml:"troubleshooting,omitempty"`
}

// Metric represents a Prometheus metric registered by the operator.
type Metric struct {
	Name string `yaml:"name"`
	Help string `yaml:"help"`
	Type string `yaml:"type"`
}

// MakeTarget represents a documented Makefile target (those with ## comments).
type MakeTarget struct {
	Name string `yaml:"name"`
	Desc string `yaml:"desc"`
}

// SampleGroup represents a group of related example manifests for the examples page.
type SampleGroup struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	YAML        string `yaml:"yaml"`
}

// OutputTarget defines a single generated file: where to write it and which template to use.
type OutputTarget struct {
	Path     string // relative path from repo root
	Template string // template content to render
}
