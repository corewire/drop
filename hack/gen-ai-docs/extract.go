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
	"sort"
	"strings"
)

// ─── Type Extraction ─────────────────────────────────────────────────────────

// parseAllTypes parses api/v1alpha1/*_types.go and returns CRDs + helper types.
// CRDs are auto-detected by the +kubebuilder:object:root=true marker.
// Helper types are exported structs that aren't CRD roots/Spec/Status/List.
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
		for fname, file := range pkg.Files {
			rawBytes, _ := os.ReadFile(filepath.Join(dir, filepath.Base(fname)))
			rawLines := strings.Split(string(rawBytes), "\n")

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

					typeLine := fset.Position(gd.Pos()).Line - 1 // 0-indexed
					markers := collectMarkersAbove(rawLines, typeLine)

					allTypes[name] = &typeInfo{
						name:    name,
						doc:     doc,
						markers: markers,
						fields:  parseFields(st),
					}
				}
			}
		}
	}

	// Identify CRD roots: types with +kubebuilder:object:root=true (excluding List types)
	var rootCRDs []string
	for name, t := range allTypes {
		if strings.HasSuffix(name, "List") {
			continue
		}
		for _, m := range t.markers {
			if strings.Contains(m, "object:root=true") {
				rootCRDs = append(rootCRDs, name)
				break
			}
		}
	}
	sort.Strings(rootCRDs)

	// Build CRD entries with controller detection
	crds := make([]CRD, 0, len(rootCRDs))
	for _, kind := range rootCRDs {
		root := allTypes[kind]
		crd := CRD{
			Kind:    kind,
			Doc:     root.doc,
			Scope:   "Cluster",
			Markers: root.markers,
		}
		// Convention: controller file is <lowerkind>_controller.go
		controllerFile := "internal/controller/" + strings.ToLower(kind) + "_controller.go"
		if fileExists(filepath.Join(dir, "..", "..", controllerFile)) {
			crd.Controller = controllerFile
			crd.TestFile = strings.TrimSuffix(controllerFile, ".go") + "_test.go"
		}
		if spec, ok := allTypes[kind+"Spec"]; ok {
			crd.SpecFields = spec.fields
		}
		if status, ok := allTypes[kind+"Status"]; ok {
			crd.StatusFields = status.fields
		}
		crds = append(crds, crd)
	}

	// Helper types: exported structs not associated with any CRD
	skipSet := map[string]bool{}
	for _, kind := range rootCRDs {
		skipSet[kind] = true
		skipSet[kind+"Spec"] = true
		skipSet[kind+"Status"] = true
		skipSet[kind+"List"] = true
	}
	var helpers []TypeDef
	for name, t := range allTypes {
		if skipSet[name] || strings.HasSuffix(name, "List") || !ast.IsExported(name) {
			continue
		}
		helpers = append(helpers, TypeDef{Name: t.name, Doc: t.doc, Fields: t.fields})
	}
	sort.Slice(helpers, func(i, j int) bool { return helpers[i].Name < helpers[j].Name })

	return crds, helpers
}

// collectMarkersAbove scans raw source lines upward from a type declaration
// to find +kubebuilder: markers (which may be separated by blank lines).
func collectMarkersAbove(lines []string, typeLine int) []string {
	var markers []string
	for i := typeLine - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "//") {
			break
		}
		text := strings.TrimPrefix(trimmed, "//")
		text = strings.TrimSpace(text)
		if strings.HasPrefix(text, "+kubebuilder:") {
			markers = append(markers, text)
		}
	}
	return markers
}

// parseFields extracts exported struct fields with their JSON tags and documentation.
func parseFields(st *ast.StructType) []Field {
	fields := make([]Field, 0, len(st.Fields.List))
	for _, f := range st.Fields.List {
		if len(f.Names) == 0 || !ast.IsExported(f.Names[0].Name) {
			continue
		}
		name := f.Names[0].Name

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

// ─── Relationship Inference ──────────────────────────────────────────────────

// inferRelationships derives CRD relationships from spec field names (e.g. policyRef)
// and supplements with staticRelationships() from config.go.
func inferRelationships(crds []CRD) []Relation {
	var rels []Relation
	crdSet := map[string]bool{}
	for _, c := range crds {
		crdSet[c.Kind] = true
	}

	for _, crd := range crds {
		if crd.Controller == "" || crd.Kind == "PullPolicy" {
			continue
		}
		for _, f := range crd.SpecFields {
			lj := strings.ToLower(f.JSON)
			if strings.Contains(lj, "policyref") && !strings.Contains(lj, "discovery") {
				rels = append(rels, Relation{From: crd.Kind, To: "PullPolicy", Type: "references", Mechanism: "spec." + f.JSON})
			}
			if strings.Contains(lj, "discoverypolicyref") {
				rels = append(rels, Relation{From: crd.Kind, To: "DiscoveryPolicy", Type: "references", Mechanism: "spec." + f.JSON})
			}
		}
	}

	return rels
}

// ─── Package Extraction ──────────────────────────────────────────────────────

type goListPkg struct {
	ImportPath string   `json:"ImportPath"`
	Imports    []string `json:"Imports"`
	Doc        string   `json:"Doc"`
}

// extractPackages uses `go list -json` to discover internal packages, their doc strings,
// and internal import relationships.
func extractPackages(root, module string) []Package {
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: go list failed: %v (package info will be empty)\n", err)
		return nil
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
			role = rel
		}

		pkgs = append(pkgs, Package{
			Path:    rel,
			Role:    role,
			Imports: internalImports,
		})
	}
	return pkgs
}

// ─── Error Reason Extraction ─────────────────────────────────────────────────

var reasonAssignRe = regexp.MustCompile(`(?:\.Reason|Reason)\s*[:=]\s*"([A-Z][a-zA-Z]+)"`)
var reasonConstRe = regexp.MustCompile(`reason\w+\s*=\s*"([A-Z][a-zA-Z]+)"`)
var messageRe = regexp.MustCompile(`\.Message\s*=\s*"([^"]+)"`)

// extractErrorReasons scans controller source files for Reason assignments/constants
// and extracts the associated Message string (if found within 3 lines).
func extractErrorReasons(root string) []ErrorReason {
	controllerDir := filepath.Join(root, "internal", "controller")
	entries, err := os.ReadDir(controllerDir)
	if err != nil {
		return nil
	}

	type reasonInfo struct {
		reason     string
		controller string
		meaning    string
	}
	seen := map[string]bool{}
	var reasons []reasonInfo

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_controller.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(controllerDir, entry.Name()))
		if err != nil {
			continue
		}
		content := string(data)
		lines := strings.Split(content, "\n")

		kind := controllerFileToKind(strings.TrimSuffix(entry.Name(), "_controller.go"))

		// Look for .Reason = "..." or Reason: "..." assignments
		for i, line := range lines {
			m := reasonAssignRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			r := m[1]
			if seen[kind+"/"+r] {
				continue
			}
			seen[kind+"/"+r] = true

			// Try to find .Message = "..." on nearby lines
			meaning := ""
			for j := i + 1; j < len(lines) && j <= i+3; j++ {
				if mm := messageRe.FindStringSubmatch(lines[j]); mm != nil {
					meaning = mm[1]
					break
				}
			}
			reasons = append(reasons, reasonInfo{reason: r, controller: kind, meaning: meaning})
		}

		// Also check for const definitions like reasonFoo = "Foo"
		for _, m := range reasonConstRe.FindAllStringSubmatch(content, -1) {
			r := m[1]
			if !seen[kind+"/"+r] {
				seen[kind+"/"+r] = true
				reasons = append(reasons, reasonInfo{reason: r, controller: kind})
			}
		}
	}

	sort.Slice(reasons, func(i, j int) bool {
		if reasons[i].controller != reasons[j].controller {
			return reasons[i].controller < reasons[j].controller
		}
		return reasons[i].reason < reasons[j].reason
	})

	result := make([]ErrorReason, 0, len(reasons))
	for _, r := range reasons {
		result = append(result, ErrorReason{Reason: r.reason, Controller: r.controller, Meaning: r.meaning})
	}
	return result
}

// controllerFileToKind converts a controller filename stem (e.g. "cachedimage")
// to its proper CRD kind (e.g. "CachedImage") by scanning api types.
func controllerFileToKind(stem string) string {
	if kindMap == nil {
		kindMap = buildKindMap()
	}
	if k, ok := kindMap[stem]; ok {
		return k
	}
	if len(stem) == 0 {
		return stem
	}
	return strings.ToUpper(stem[:1]) + stem[1:]
}

var kindMap map[string]string

func buildKindMap() map[string]string {
	root := findRepoRoot()
	dir := filepath.Join(root, "api", "v1alpha1")
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return strings.HasSuffix(fi.Name(), "_types.go")
	}, parser.ParseComments)
	if err != nil {
		return map[string]string{}
	}
	m := map[string]string{}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				gd, ok := decl.(*ast.GenDecl)
				if !ok || gd.Tok != token.TYPE {
					continue
				}
				for _, spec := range gd.Specs {
					ts := spec.(*ast.TypeSpec)
					name := ts.Name.Name
					if strings.HasSuffix(name, "Spec") || strings.HasSuffix(name, "Status") || strings.HasSuffix(name, "List") {
						continue
					}
					if ast.IsExported(name) {
						m[strings.ToLower(name)] = name
					}
				}
			}
		}
	}
	return m
}

// ─── Metrics Extraction ──────────────────────────────────────────────────────

// extractMetrics parses metrics.go for Prometheus metric registrations.
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

	metrics := make([]Metric, 0, len(names))
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

// ─── Makefile Extraction ─────────────────────────────────────────────────────

var makeTargetRe = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_-]*):\s*.*?##\s*(.+)$`)

// extractMakeTargets finds Makefile targets documented with ## comments.
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

// ─── Sample Parsing ──────────────────────────────────────────────────────────

// parseSampleGroups splits dev-samples.yaml on "---" separators, reads the `kind:`
// field from each YAML document, and groups them by kind. Descriptions are derived
// from CRD doc strings — no hardcoded list needed.
func parseSampleGroups(raw string, crds []CRD) []SampleGroup {
	crdDocs := map[string]string{}
	for _, c := range crds {
		crdDocs[c.Kind] = c.Doc
	}

	// Split on YAML document separators
	docs := splitYAMLDocs(raw)

	// Group documents by kind
	kindGroups := map[string]*SampleGroup{}
	var kindOrder []string

	for _, doc := range docs {
		kind := extractYAMLKind(doc)
		if kind == "" {
			kind = "Other"
		}
		if _, exists := kindGroups[kind]; !exists {
			kindGroups[kind] = &SampleGroup{Title: kind, Description: crdDocs[kind]}
			kindOrder = append(kindOrder, kind)
		}
		g := kindGroups[kind]
		if g.YAML != "" {
			g.YAML += "\n---\n"
		}
		g.YAML += doc
	}

	groups := make([]SampleGroup, 0, len(kindOrder))
	for _, kind := range kindOrder {
		groups = append(groups, *kindGroups[kind])
	}
	return groups
}

// splitYAMLDocs splits a multi-document YAML file on "---" lines,
// strips comments and blank lines around each doc, and returns non-empty documents.
func splitYAMLDocs(raw string) []string {
	var docs []string
	var current []string

	for _, line := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			if doc := joinAndClean(current); doc != "" {
				docs = append(docs, doc)
			}
			current = nil
			continue
		}
		current = append(current, line)
	}
	if doc := joinAndClean(current); doc != "" {
		docs = append(docs, doc)
	}
	return docs
}

// joinAndClean joins lines, strips leading comment-only lines, and trims whitespace.
func joinAndClean(lines []string) string {
	// Drop leading comment/blank lines (section headers like "# === PullPolicy ===")
	start := 0
	for start < len(lines) {
		t := strings.TrimSpace(lines[start])
		if t == "" || strings.HasPrefix(t, "#") {
			start++
		} else {
			break
		}
	}
	result := strings.TrimSpace(strings.Join(lines[start:], "\n"))
	return result
}

// extractYAMLKind reads the `kind:` field from a YAML document without full unmarshaling.
var kindLineRe = regexp.MustCompile(`(?m)^kind:\s*(\S+)`)

func extractYAMLKind(doc string) string {
	m := kindLineRe.FindStringSubmatch(doc)
	if m == nil {
		return ""
	}
	return m[1]
}

// ─── AST Helpers ─────────────────────────────────────────────────────────────

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
