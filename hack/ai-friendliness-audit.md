# AI-Friendliness Audit — Documentation Site Ranking

## Ranking System (0–5 per dimension, max score 50)

| # | Dimension | Weight | What it measures |
|---|-----------|--------|------------------|
| 1 | **Discoverability** | × 1 | Can an agent find and understand what this site offers within one request? (llms.txt, meta tags, link alternate) |
| 2 | **Machine-Readable Output** | × 1 | Are pages available in clean Markdown/plain text without HTML noise? |
| 3 | **Structured Data** | × 1 | Tables, consistent headings, predictable field schemas — can an agent parse reliably? |
| 4 | **Context Density** | × 1 | Information-to-noise ratio. Are pages concise with minimal boilerplate/decorative text? |
| 5 | **Navigation Clarity** | × 1 | Flat hierarchy, descriptive page names, logical grouping — can an agent orient itself? |
| 6 | **Completeness** | × 1 | Does the documentation cover all CRDs, fields, status, errors, metrics? |
| 7 | **Actionability** | × 1 | Examples, commands, copy-pasteable YAML — can an agent generate correct manifests? |
| 8 | **Self-Description** | × 1 | Does the site explain its own structure to agents? (llmsDescription, frontmatter, README) |
| 9 | **Freshness Signals** | × 1 | Last-updated dates, git info, generation timestamps — can an agent assess staleness? |
| 10 | **Integration Surface** | × 1 | Can agents open this content directly in ChatGPT/Claude? Context menu links, URL patterns? |

### Scoring Guide

- **5** — Best-in-class, nothing missing
- **4** — Solid, minor gaps
- **3** — Functional but has clear room for improvement
- **2** — Present but barely usable by an agent
- **1** — Technically exists, practically useless
- **0** — Absent

---

## Audit of `http://localhost:1314/drop/` (2026-05-24)

| # | Dimension | Score | Notes |
|---|-----------|-------|-------|
| 1 | Discoverability | **5** | `/llms.txt` at site root with all page links + descriptions. `<link rel="alternate" type="text/markdown">` in HTML head. Homepage `llmsDescription` frontmatter explains the project in plain text. |
| 2 | Machine-Readable Output | **5** | Every page available at `{url}index.md` as clean Markdown — no frontmatter leakage, no HTML. Hugo output format configured correctly. |
| 3 | Structured Data | **5** | CRD reference uses consistent tables (Field/Type/Required/Default/Description). Metrics table. Architecture has relationship graph. Predictable patterns across all reference pages. |
| 4 | Context Density | **4** | Pages are concise. Homepage hero is slightly wordy ("Declarative image pre-caching for Kubernetes" + subtitle both exist). Reference pages are excellent — zero fluff. Minor: docs landing page could collapse Quick Start into Getting Started. |
| 5 | Navigation Clarity | **4** | Flat hierarchy: docs/ → 4 pages + reference/ (4 generated pages). Logical grouping. Minor: `_generated_` prefix in URLs is ugly but functional. Section index at `/docs/reference/` exists. |
| 6 | Completeness | **5** | All 4 CRDs documented with every field. Status conditions, error reasons, metrics all covered. Architecture shows relationships. Discovery sources documented. |
| 7 | Actionability | **4** | Getting Started has helm install command. Missing: sample CachedImage YAML in docs (exists in `config/samples/` but not linked from docs). No "copy this manifest" examples on CRD reference page. |
| 8 | Self-Description | **5** | `llmsDescription` on every page. Homepage describes the project scope. llms.txt has one-line summaries. Agent instructions in repo root (AGENTS.md, .github/copilot-instructions.md). |
| 9 | Freshness Signals | **5** | `enableGitInfo: true` + `displayUpdatedDate: true` shows "Last updated on May 22, 2026" on every page. llms.txt has generation timestamp. |
| 10 | Integration Surface | **4** | Context menu has "Open in ChatGPT" and "Open in Claude" with `{markdown_url}` interpolation. Missing: no `/llms-full.txt` endpoint on the Hugo site (only repo-root). Agents must discover markdown URLs via llms.txt → follow link → get content. |

### **Total: 46 / 50**

---

## Recommendations (to reach 50/50)

1. **Context Density → 5**: Remove redundant subtitle on homepage OR merge docs landing page Quick Start into Getting Started page.
2. **Navigation Clarity → 5**: Consider aliasing `_generated_crds` → `crds` (Hugo aliases in frontmatter).
3. **Actionability → 5**: Add a "Quick Example" code block on the CRD Reference page with a minimal CachedImage manifest.
4. **Integration Surface → 5**: Serve `llms-full.txt` as a Hugo static file (or generate it into `docs/static/`) so agents can get everything in one request.

---

## Audit Prompt

Use the following prompt to evaluate any documentation site for AI-friendliness:

```
You are an AI documentation agent evaluating a website for machine consumption.

Perform the following checks and score each dimension 0–5:

1. DISCOVERABILITY: Fetch the site root. Is there a /llms.txt or /llms-full.txt?
   Check HTML <head> for <link rel="alternate" type="text/markdown">.
   Check for meta descriptions or structured frontmatter.

2. MACHINE-READABLE OUTPUT: Can you fetch any page as plain Markdown by appending
   .md or /index.md to the URL? Is the output clean (no HTML, no frontmatter)?

3. STRUCTURED DATA: Are reference pages using consistent tables or schemas?
   Can you reliably extract field names, types, and descriptions programmatically?

4. CONTEXT DENSITY: What is the information-to-noise ratio? Count decorative text,
   repeated navigation, boilerplate vs. actual technical content.

5. NAVIGATION CLARITY: How many clicks/requests to reach any piece of information?
   Is the hierarchy flat? Are page names descriptive?

6. COMPLETENESS: Does the documentation cover all APIs, fields, status, errors?
   Are there undocumented features visible in the codebase but missing from docs?

7. ACTIONABILITY: Are there copy-pasteable examples? Can you generate a valid
   manifest/config from the docs alone without looking at source code?

8. SELF-DESCRIPTION: Does the site explain its own structure? Is there an index
   page that lists all content with summaries? Does frontmatter describe pages?

9. FRESHNESS SIGNALS: Are there timestamps, git commit info, or generation dates?
   Can you determine if the docs are current?

10. INTEGRATION SURFACE: Can you open this content directly in an AI assistant?
    Are there deep links with pre-filled prompts? Can you get all content in one
    request (llms-full.txt)?

For each dimension, output:
- Score (0–5)
- Evidence (specific URLs, content snippets)
- Recommendation (if score < 5)

Final output: Total score /50, letter grade (A: 45-50, B: 38-44, C: 30-37, D: <30)
```
