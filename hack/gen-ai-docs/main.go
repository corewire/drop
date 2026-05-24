// hack/gen-ai-docs generates all documentation from source code.
//
// It parses api/v1alpha1/*_types.go, internal/controller/*.go, internal/metrics/,
// Makefile, and go.mod to build a unified knowledge model. From that model it
// generates documentation for three audiences:
//   - USE agents: llms.txt, llms-full.txt
//   - CODE agents: .github/copilot-instructions.md, .cursorrules, AGENTS.md
//   - HUMANS: Hugo content pages (CRD reference, errors, metrics, architecture)
//
// Usage: go run ./hack/gen-ai-docs/
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// ─── Knowledge Model ─────────────────────────────────────────────────────────

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
}

type Project struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	APIGroup    string `yaml:"apiGroup"`
	GoVersion   string `yaml:"goVersion"`
	Module      string `yaml:"module"`
	License     string `yaml:"license"`
}

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

type TypeDef struct {
	Name   string  `yaml:"name"`
	Doc    string  `yaml:"doc"`
	Fields []Field `yaml:"fields"`
}

type Field struct {
	Name     string   `yaml:"name"`
	JSON     string   `yaml:"json"`
	Type     string   `yaml:"type"`
	Required bool     `yaml:"required"`
	Default  string   `yaml:"default,omitempty"`
	Enum     []string `yaml:"enum,omitempty"`
	Doc      string   `yaml:"doc"`
}

type Relation struct {
	From      string `yaml:"from"`
	To        string `yaml:"to"`
	Type      string `yaml:"type"`
	Mechanism string `yaml:"mechanism,omitempty"`
}

type Package struct {
	Path    string   `yaml:"path"`
	Role    string   `yaml:"role"`
	Imports []string `yaml:"imports,omitempty"`
}

type Convention struct {
	Rule  string   `yaml:"rule"`
	Scope []string `yaml:"scope"`
}

type ErrorReason struct {
	Reason          string `yaml:"reason"`
	Controller      string `yaml:"controller"`
	Meaning         string `yaml:"meaning"`
	Troubleshooting string `yaml:"troubleshooting,omitempty"`
}

type Metric struct {
	Name string `yaml:"name"`
	Help string `yaml:"help"`
	Type string `yaml:"type"`
}

type MakeTarget struct {
	Name string `yaml:"name"`
	Desc string `yaml:"desc"`
}

// ─── Main ────────────────────────────────────────────────────────────────────

func main() {
	root := findRepoRoot()
	k := buildKnowledge(root)

	// Write intermediate knowledge file
	writeKnowledgeYAML(root, k)

	// USE agents (repo-root for IDE/GitHub consumption)
	generateFile(root, "llms.txt", llmsTxtTmpl, k)
	generateFile(root, "llms-full.txt", llmsFullTxtTmpl, k)

	// USE agents (Hugo static — serve llms-full.txt on the site)
	generateFile(root, filepath.Join("docs", "static", "llms-full.txt"), llmsFullTxtTmpl, k)

	// CODE agents
	generateFile(root, filepath.Join(".github", "copilot-instructions.md"), copilotInstructionsTmpl, k)
	generateFile(root, ".cursorrules", cursorRulesTmpl, k)
	generateFile(root, "AGENTS.md", agentsMdTmpl, k)

	// HUMANS (Hugo)
	generateFile(root, filepath.Join("docs", "content", "docs", "reference", "_generated_crds.md"), hugoCRDsTmpl, k)
	generateFile(root, filepath.Join("docs", "content", "docs", "reference", "_generated_errors.md"), hugoErrorsTmpl, k)
	generateFile(root, filepath.Join("docs", "content", "docs", "reference", "_generated_metrics.md"), hugoMetricsTmpl, k)
	generateFile(root, filepath.Join("docs", "content", "docs", "reference", "_generated_architecture.md"), hugoArchTmpl, k)

	// Repo-level doc generation diagram
	generateFile(root, filepath.Join("docs", "doc-generation.md"), docGenDiagramTmpl, k)

	fmt.Println("✓ Generated: knowledge.yaml + llms.txt + llms-full.txt + agent instructions + Hugo reference pages + doc-generation.md")
}

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

// ─── Knowledge Builder ───────────────────────────────────────────────────────

func buildKnowledge(root string) Knowledge {
	goVer, module := parseGoMod(filepath.Join(root, "go.mod"))

	k := Knowledge{
		Project: Project{
			Name:        "puller",
			Description: "Kubernetes operator that pre-caches container images on cluster nodes",
			APIGroup:    "puller.corewire.io/v1alpha1",
			GoVersion:   goVer,
			Module:      module,
			License:     "Apache-2.0",
		},
	}

	crds, helpers := parseAllTypes(filepath.Join(root, "api", "v1alpha1"))
	k.CRDs = crds
	k.HelperTypes = helpers
	k.Relationships = buildRelationships()
	k.Packages = extractPackages(root, module)
	k.Errors = buildErrorCatalog()
	k.Metrics = extractMetrics(filepath.Join(root, "internal", "metrics", "metrics.go"))
	k.MakeTargets = extractMakeTargets(filepath.Join(root, "Makefile"))
	k.Samples = readFileStr(filepath.Join(root, "hack", "dev-samples.yaml"))

	k.Conventions = []Convention{
		{Rule: "All CRDs are cluster-scoped", Scope: []string{"code", "use"}},
		{Rule: "Status uses metav1.Condition with type \"Ready\"", Scope: []string{"code", "use"}},
		{Rule: "No privileged containers — kubelet-based image pulls only", Scope: []string{"code"}},
		{Rule: "Single responsibility reconcilers — one controller per CRD", Scope: []string{"code"}},
		{Rule: "Pod builder is a pure function in internal/podbuilder/ (no k8s client)", Scope: []string{"code"}},
		{Rule: "Pacing logic lives exclusively in internal/pacing/", Scope: []string{"code"}},
		{Rule: "ownerReferences: CachedImageSet→CachedImage, controller→Pod", Scope: []string{"code"}},
		{Rule: "Table-driven tests preferred; envtest for controllers", Scope: []string{"code"}},
		{Rule: "Pods use nodeName placement + command: [\"true\"]", Scope: []string{"code", "use"}},
		{Rule: "Don't manually edit generated files — run make docs-gen", Scope: []string{"code"}},
	}

	return k
}

// ─── Type Parser ─────────────────────────────────────────────────────────────

func parseAllTypes(dir string) ([]CRD, []TypeDef) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return strings.HasSuffix(fi.Name(), "_types.go")
	}, parser.ParseComments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error parsing types: %v\n", err)
		os.Exit(1)
	}

	type typeInfo struct {
		name    string
		doc     string
		markers []string
		fields  []Field
	}
	allTypes := map[string]*typeInfo{}

	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, spec := range gd.Specs {
					ts := spec.(*ast.TypeSpec)
					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						continue
					}
					name := ts.Name.Name
					doc := ""
					if gd.Doc != nil {
						doc = cleanDoc(gd.Doc.Text())
					}
					allTypes[name] = &typeInfo{
						name:    name,
						doc:     doc,
						markers: extractMarkers(gd.Doc),
						fields:  parseFields(st),
					}
				}
			}
		}
	}

	rootCRDs := []string{"CachedImage", "CachedImageSet", "PullPolicy", "DiscoveryPolicy"}
	controllerMap := map[string]string{
		"CachedImage":     "internal/controller/cachedimage_controller.go",
		"CachedImageSet":  "internal/controller/cachedimageset_controller.go",
		"DiscoveryPolicy": "internal/controller/discoverypolicy_controller.go",
	}

	var crds []CRD
	for _, kind := range rootCRDs {
		root, ok := allTypes[kind]
		if !ok {
			continue
		}
		crd := CRD{
			Kind:    kind,
			Doc:     root.doc,
			Scope:   "Cluster",
			Markers: root.markers,
		}
		if c, ok := controllerMap[kind]; ok {
			crd.Controller = c
			crd.TestFile = strings.TrimSuffix(c, ".go") + "_test.go"
		}
		if spec, ok := allTypes[kind+"Spec"]; ok {
			crd.SpecFields = spec.fields
		}
		if status, ok := allTypes[kind+"Status"]; ok {
			crd.StatusFields = status.fields
		}
		crds = append(crds, crd)
	}

	helperNames := []string{
		"PolicyReference", "DiscoveryPolicyReference", "ImageEntry",
		"BackoffConfig", "DiscoverySource", "PrometheusSource",
		"RegistrySource", "DiscoveredImage",
	}
	var helpers []TypeDef
	for _, name := range helperNames {
		if t, ok := allTypes[name]; ok {
			helpers = append(helpers, TypeDef{Name: t.name, Doc: t.doc, Fields: t.fields})
		}
	}

	return crds, helpers
}

func parseFields(st *ast.StructType) []Field {
	var fields []Field
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 {
			continue
		}
		name := f.Names[0].Name
		if !ast.IsExported(name) {
			continue
		}

		jsonTag := ""
		required := true
		if f.Tag != nil {
			tag := f.Tag.Value
			if idx := strings.Index(tag, `json:"`); idx >= 0 {
				rest := tag[idx+6:]
				end := strings.Index(rest, `"`)
				jsonTag = rest[:end]
				if strings.Contains(jsonTag, "omitempty") {
					required = false
				}
				jsonTag = strings.Split(jsonTag, ",")[0]
			}
		}

		doc := ""
		if f.Doc != nil {
			doc = cleanDoc(f.Doc.Text())
		} else if f.Comment != nil {
			doc = cleanDoc(f.Comment.Text())
		}

		fields = append(fields, Field{
			Name:     name,
			JSON:     jsonTag,
			Type:     typeString(f.Type),
			Doc:      doc,
			Required: required,
			Default:  extractDefault(f.Doc),
			Enum:     extractEnum(f.Doc),
		})
	}
	return fields
}

func typeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return typeString(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.ArrayType:
		return "[]" + typeString(t.Elt)
	case *ast.MapType:
		return "map[" + typeString(t.Key) + "]" + typeString(t.Value)
	default:
		return "unknown"
	}
}

func extractMarkers(doc *ast.CommentGroup) []string {
	if doc == nil {
		return nil
	}
	var markers []string
	for _, c := range doc.List {
		text := strings.TrimPrefix(c.Text, "//")
		text = strings.TrimSpace(text)
		if strings.HasPrefix(text, "+kubebuilder:") {
			markers = append(markers, text)
		}
	}
	return markers
}

var defaultRe = regexp.MustCompile(`\+kubebuilder:default=(.+)`)
var enumRe = regexp.MustCompile(`\+kubebuilder:validation:Enum=(.+)`)

func extractDefault(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	for _, c := range doc.List {
		if m := defaultRe.FindStringSubmatch(c.Text); len(m) > 1 {
			return strings.Trim(m[1], `"`)
		}
	}
	return ""
}

func extractEnum(doc *ast.CommentGroup) []string {
	if doc == nil {
		return nil
	}
	for _, c := range doc.List {
		if m := enumRe.FindStringSubmatch(c.Text); len(m) > 1 {
			return strings.Split(m[1], ";")
		}
	}
	return nil
}

func cleanDoc(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	var clean []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "+") {
			continue
		}
		if l != "" {
			clean = append(clean, l)
		}
	}
	return strings.Join(clean, " ")
}

// ─── Relationships ───────────────────────────────────────────────────────────

func buildRelationships() []Relation {
	return []Relation{
		{From: "CachedImageSet", To: "CachedImage", Type: "owns", Mechanism: "ownerReferences"},
		{From: "CachedImage", To: "Pod", Type: "creates", Mechanism: "controller-runtime client"},
		{From: "CachedImage", To: "PullPolicy", Type: "references", Mechanism: "spec.policyRef"},
		{From: "CachedImageSet", To: "PullPolicy", Type: "references", Mechanism: "spec.policyRef"},
		{From: "CachedImageSet", To: "DiscoveryPolicy", Type: "references", Mechanism: "spec.discoveryPolicyRef"},
		{From: "DiscoveryPolicy", To: "CachedImageSet", Type: "feeds", Mechanism: "status.discoveredImages"},
	}
}

// ─── Package Extractor ───────────────────────────────────────────────────────

type goListPkg struct {
	ImportPath string   `json:"ImportPath"`
	Imports    []string `json:"Imports"`
	Doc        string   `json:"Doc"`
}

func extractPackages(root, module string) []Package {
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return staticPackages()
	}

	decoder := json.NewDecoder(bytes.NewReader(out))
	var pkgs []Package
	for decoder.More() {
		var p goListPkg
		if err := decoder.Decode(&p); err != nil {
			break
		}
		rel := strings.TrimPrefix(p.ImportPath, module+"/")
		if !strings.HasPrefix(rel, "internal/") && !strings.HasPrefix(rel, "api/") {
			continue
		}

		var internalImports []string
		for _, imp := range p.Imports {
			if strings.HasPrefix(imp, module) {
				internalImports = append(internalImports, strings.TrimPrefix(imp, module+"/"))
			}
		}

		role := p.Doc
		if role == "" {
			role = inferRole(rel)
		}

		pkgs = append(pkgs, Package{
			Path:    rel,
			Role:    role,
			Imports: internalImports,
		})
	}

	if len(pkgs) == 0 {
		return staticPackages()
	}
	return pkgs
}

func inferRole(path string) string {
	roles := map[string]string{
		"api/v1alpha1":        "CRD type definitions (source of truth)",
		"internal/controller": "Reconciler implementations (one per CRD)",
		"internal/podbuilder": "Pure Pod construction function (no k8s client)",
		"internal/pacing":     "Shared pacing engine for rate-limited pulls",
		"internal/discovery":  "Discovery source interface + implementations",
		"internal/metrics":    "Prometheus metrics registration",
	}
	if r, ok := roles[path]; ok {
		return r
	}
	return ""
}

func staticPackages() []Package {
	return []Package{
		{Path: "api/v1alpha1", Role: "CRD type definitions (source of truth)"},
		{Path: "internal/controller", Role: "Reconciler implementations (one per CRD)", Imports: []string{"api/v1alpha1", "internal/podbuilder", "internal/pacing", "internal/metrics"}},
		{Path: "internal/podbuilder", Role: "Pure Pod construction (no k8s client)", Imports: []string{"api/v1alpha1"}},
		{Path: "internal/pacing", Role: "Shared pacing engine for rate-limited pulls"},
		{Path: "internal/discovery", Role: "Discovery source interface + implementations"},
		{Path: "internal/metrics", Role: "Prometheus metrics registration"},
	}
}

// ─── Error Catalog ───────────────────────────────────────────────────────────

func buildErrorCatalog() []ErrorReason {
	defs := []ErrorReason{
		{Reason: "Cached", Controller: "CachedImage", Meaning: "All target nodes have the image cached"},
		{Reason: "Degraded", Controller: "CachedImageSet", Meaning: "Some child CachedImages have failures", Troubleshooting: "Check individual CachedImage statuses"},
		{Reason: "ErrImagePull", Controller: "CachedImage", Meaning: "Registry unreachable or image does not exist", Troubleshooting: "Verify registry DNS, image name, tag. Check network policies"},
		{Reason: "ImagePullBackOff", Controller: "CachedImage", Meaning: "Repeated pull failures, kubelet is backing off", Troubleshooting: "Check imagePullSecrets, registry auth. Verify image exists"},
		{Reason: "InProgress", Controller: "CachedImage", Meaning: "Image pulls are actively running on some nodes"},
		{Reason: "InvalidImageName", Controller: "CachedImage", Meaning: "The image reference is malformed", Troubleshooting: "Check spec.image format: registry/repository"},
		{Reason: "PartiallyFailed", Controller: "DiscoveryPolicy", Meaning: "Some discovery sources failed to sync", Troubleshooting: "Check source endpoints and credentials"},
		{Reason: "PodFailed", Controller: "CachedImage", Meaning: "Puller Pod failed for a non-image-pull reason", Troubleshooting: "Check node health, resource limits, Pod security policies"},
		{Reason: "Progressing", Controller: "CachedImageSet", Meaning: "Children are still being pulled"},
		{Reason: "PullFailed", Controller: "CachedImage", Meaning: "One or more nodes failed to pull the image", Troubleshooting: "Check image name, tag, registry connectivity, imagePullSecrets"},
		{Reason: "Ready", Controller: "CachedImageSet", Meaning: "All child CachedImages are ready"},
		{Reason: "RegistryUnavailable", Controller: "CachedImage", Meaning: "Cannot connect to the container registry", Troubleshooting: "Check registry URL, DNS, firewall rules"},
		{Reason: "SourceError", Controller: "DiscoveryPolicy", Meaning: "One or more discovery sources returned errors", Troubleshooting: "Check source configuration and connectivity"},
		{Reason: "SyncFailed", Controller: "DiscoveryPolicy", Meaning: "All discovery sources failed", Troubleshooting: "Check all source endpoints, credentials, network"},
		{Reason: "Synced", Controller: "DiscoveryPolicy", Meaning: "All sources synced successfully"},
	}
	return defs
}

// ─── Metrics Extractor ───────────────────────────────────────────────────────

func extractMetrics(path string) []Metric {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	content := string(data)

	nameRe := regexp.MustCompile(`Name:\s+"([^"]+)"`)
	helpRe := regexp.MustCompile(`Help:\s+"([^"]+)"`)
	typeRe := regexp.MustCompile(`prometheus\.New(Counter|Gauge|Histogram|Summary)`)

	names := nameRe.FindAllStringSubmatch(content, -1)
	helps := helpRe.FindAllStringSubmatch(content, -1)
	types := typeRe.FindAllStringSubmatch(content, -1)

	var metrics []Metric
	for i, n := range names {
		m := Metric{Name: n[1]}
		if i < len(helps) {
			m.Help = helps[i][1]
		}
		if i < len(types) {
			m.Type = strings.ToLower(types[i][1])
		}
		metrics = append(metrics, m)
	}
	return metrics
}

// ─── Make Targets ────────────────────────────────────────────────────────────

var makeTargetRe = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_-]*):\s*.*?##\s*(.+)$`)

func extractMakeTargets(path string) []MakeTarget {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var targets []MakeTarget
	for _, line := range strings.Split(string(data), "\n") {
		m := makeTargetRe.FindStringSubmatch(line)
		if m != nil {
			targets = append(targets, MakeTarget{Name: m[1], Desc: m[2]})
		}
	}
	return targets
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func parseGoMod(path string) (string, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "1.23", "github.com/Breee/puller"
	}
	goVer := "1.23"
	module := ""
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "go ") {
			goVer = strings.TrimSpace(strings.TrimPrefix(line, "go "))
		}
		if strings.HasPrefix(line, "module ") {
			module = strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return goVer, module
}

func readFileStr(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func writeKnowledgeYAML(root string, k Knowledge) {
	var buf bytes.Buffer
	buf.WriteString("# Generated by make docs-gen — DO NOT EDIT\n")
	buf.WriteString("# Source: hack/gen-ai-docs/\n")
	buf.WriteString("# Regenerate: make docs-gen\n\n")

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(k); err != nil {
		fmt.Fprintf(os.Stderr, "error encoding knowledge.yaml: %v\n", err)
		os.Exit(1)
	}
	enc.Close()

	outPath := filepath.Join(root, "knowledge.yaml")
	if err := os.WriteFile(outPath, buf.Bytes(), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing knowledge.yaml: %v\n", err)
		os.Exit(1)
	}
}

func generateFile(root, relPath string, tmplStr string, data Knowledge) {
	funcMap := template.FuncMap{
		"join":  strings.Join,
		"lower": strings.ToLower,
	}
	t := template.Must(template.New(relPath).Funcs(funcMap).Parse(tmplStr))
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		fmt.Fprintf(os.Stderr, "error rendering %s: %v\n", relPath, err)
		os.Exit(1)
	}

	outPath := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating dir for %s: %v\n", relPath, err)
		os.Exit(1)
	}
	if err := os.WriteFile(outPath, buf.Bytes(), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", relPath, err)
		os.Exit(1)
	}
}
