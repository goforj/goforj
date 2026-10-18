# Atlas Documentation Query Index Design

## Status

- Design status: proposed
- Design date: 2026-09-05
- Target repository: `github.com/goforj/atlas`
- Related design:
  [`completed/goforj-atlas-agent-integration-design.md`](completed/goforj-atlas-agent-integration-design.md)

## Decision

Atlas should continue loading the complete selected Markdown corpus into memory
once per MCP process. It should replace repeated query-time parsing with one
immutable, pre-parsed corpus built during startup.

Do not add a database, hosted search dependency, embedding model, or vector
store for framework documentation. The current corpus is small, versioned, and
local. A compact in-memory index preserves deterministic results, offline use,
simple deployment, and debuggability.

## Current Behavior

Atlas selects documentation from:

1. `GOFORJ_DOCS_PATH`, when configured and usable
2. a refreshed local clone of `github.com/goforj/docs`

The filesystem provider resolves a nested `docs` directory when present, walks
it recursively, skips hidden directories, `node_modules`, and `dist`, and loads
every lowercase `.md` file as a document containing its relative path, title,
and complete Markdown body.

The MCP server warms a caching provider before serving stdio requests. The
provider retains every document for the lifetime of the process and returns a
copy of the document slice to callers. Document strings continue to share their
immutable backing data.

`search-docs` and `explain-api` currently perform this work for every query:

1. iterate every document
2. split every document into heading sections
3. lowercase each section's path, title, heading, and body
4. score substring matches
5. allocate snippets for matching sections
6. sort every match before applying the result limit

Exact section and heading tools avoid searching every body, but they still scan
the document slice for a path and parse the selected document again.

The initial load also reads the selected tree more than once. Provider fallback
probes documents to establish usability, document retrieval reads them again,
and manifest generation performs another document load to calculate its digest.
The outer cache removes later filesystem access but does not remove this startup
duplication.

## Goals

- Read the selected Markdown tree once per process startup.
- Parse each Markdown document once per loaded revision.
- Preserve the complete source body in memory for bounded section reads.
- Make exact document lookup independent of corpus size.
- Search normalized section data without rebuilding it per request.
- Preserve current result limits, token limits, ranking intent, and deterministic
  tie-breaking.
- Keep the docs revision fixed for the lifetime of one MCP process.
- Keep the implementation local, dependency-light, and easy to inspect.

## Non-Goals

- Semantic or natural-language vector search.
- A remote search service.
- Runtime mutation or incremental filesystem watching.
- Searching arbitrary project source files.
- Combining framework docs with generated project inventory.
- Changing the public MCP tool names or response shapes.
- Changing documentation version-selection policy.

## Corpus Model

Introduce one internal immutable corpus owned by the docs package:

```go
type Corpus struct {
    Manifest        Manifest
    Documents       []Document
    DocumentsByPath map[string]int
    Sections        []IndexedSection
    SectionsByPath  map[string][]int
}

type IndexedSection struct {
    Section
    NormalizedPath    string
    NormalizedTitle   string
    NormalizedHeading string
    NormalizedBody    string
}
```

These names illustrate ownership, not a required exported API. Prefer keeping
the corpus and indexed section private unless tests or another Atlas package
need a stable read-only contract.

The index stores integer positions rather than duplicate documents or section
bodies. The original document and section strings remain the response source;
normalized strings exist only for matching.

`DocumentsByPath` provides exact path lookup. `SectionsByPath` provides ordered
section and neighborhood reads. The flat `Sections` slice preserves a simple
full scan for free-form search.

## Loading Boundary

Provider selection, documents, and manifest metadata should be resolved as one
snapshot operation. Atlas may implement that through an internal loader beside
the existing public `Provider`, or by caching provider selection and the first
document result before asking for the manifest. Do not require each provider to
walk and read the same tree independently for `Documents` and `Manifest`.

The corpus build sequence is:

1. select the first usable provider
2. synchronize its source once
3. read eligible Markdown files once
4. calculate the manifest from those exact bytes
5. parse documents into ordered sections
6. build path lookups and normalized search fields
7. publish the complete immutable corpus to readers

Do not expose a partially built corpus. If loading fails or the context is
cancelled, startup should return the error and publish no state.

The source revision must not change between reading files and producing the
manifest. A git-backed provider should synchronize once, then read and identify
the resulting checkout without another refresh attempt.

## Query Behavior

### Search

Search should scan the pre-parsed `Sections` slice. Preserve the current
case-insensitive weighting unless a separate relevance change is justified:

- title match: 8
- heading match: 6
- path match: 4
- body match: 1

Terms retain OR semantics with additive scores. Ties remain ordered by path and
heading so identical inputs return identical results.

Avoid sorting an unbounded match set when only a small limit is requested. A
bounded top-result collector is appropriate after corpus sizes or benchmarks
show that sorting dominates. Pre-parsing and normalization are the first change;
do not complicate selection without evidence.

Snippets should continue returning the beginning of the section, bounded by the
requested word limit. Match-centered snippets would be a separate relevance
change and should not be hidden inside the indexing refactor.

### Exact section reads

Resolve the document through `DocumentsByPath`, then search only its indexed
sections for an equal-fold heading match. An empty heading continues to return
the document's first section.

### Neighborhood reads

Resolve the ordered positions through `SectionsByPath`, locate the requested
heading, and slice the requested number of adjacent sections. Apply the word
limit to response copies rather than mutating indexed sections.

### Heading lists

Return body-free copies of the indexed sections for the requested path. Do not
reparse Markdown to produce the heading tree.

## Markdown Semantics

This refinement should preserve the current ATX-heading contract during the
first implementation. Setext headings, frontmatter-derived tags, and fenced-code
awareness are relevance and parsing changes that need their own fixtures before
adoption.

The corpus builder should centralize parsing so every query tool sees identical
section boundaries. If fenced code containing a line beginning with `# ` is
later treated differently, update parser tests and all affected golden search
results together.

## Concurrency And Lifecycle

The published corpus is immutable. Concurrent MCP reads require no corpus-wide
lock after startup. Query-local result and response slices remain owned by the
caller.

Atlas should not refresh docs in the background. Restarting the MCP process is
the explicit revision boundary. This avoids mixed indexes, changing results
inside one agent session, and synchronization machinery that offers little
value for a local stdio server.

## Performance And Memory

The full Markdown corpus remains resident by design. Measure:

- source files and source bytes loaded
- parsed sections
- startup wall time and allocations
- steady-state search wall time and allocations
- exact section-read wall time and allocations
- retained heap after corpus construction

On 2026-09-05, a canonical docs checkout contained 127 eligible Markdown files
and approximately 1.37 MiB of source content after excluding ignored
directories. This is evidence that the in-memory boundary is proportionate, not
a fixed product limit.

Benchmarks should use a representative full docs checkout or a checked-in
fixture with comparable document and section counts. Compare the current raw
document cache against the indexed corpus using identical queries and result
limits.

The intended result is lower steady-state latency and allocation without a
materially surprising retained-memory increase. Do not claim a performance
improvement from microbenchmarks that omit parsing or normalization from the
current baseline.

## Compatibility

This is an internal retrieval change. Preserve:

- `Provider`, `Manifest`, `Document`, `Section`, and MCP response contracts
- environment-variable behavior
- git fallback and offline-cache behavior
- search weights and deterministic ordering
- default result and token limits
- case-insensitive heading lookup

Any intentional relevance change must be isolated, documented with before and
after queries, and covered by golden results so index work is not confused with
ranking redesign.

## Validation

Add direct tests proving:

- one source read builds documents, manifest, sections, and indexes
- fallback selects one provider and reuses its first successful snapshot
- hidden directories, `node_modules`, `dist`, and non-Markdown files remain
  excluded
- path lookup, section reads, neighborhoods, and heading lists match current
  behavior
- search scores and tie ordering match current behavior
- snippets match current behavior without exceeding their word limit
- concurrent reads return stable results
- cancellation publishes no partial corpus
- a failed refresh can still use an existing valid git cache
- repeated fixture construction produces byte-identical manifests and results

Run Atlas root and nested-module tests discovered from every relevant `go.mod`.
Add benchmarks for startup, search, exact reads, allocation counts, and retained
heap before removing the current query path.

## Rollout

1. Add corpus construction beside the existing provider and query path.
2. Add parity tests and full-corpus benchmarks.
3. Route exact section and heading reads through path indexes.
4. Route search through pre-parsed normalized sections.
5. Remove redundant startup reads only after snapshot parity is proven.
6. Update Atlas implementation notes and public docs if observable behavior
   changes.

The implementation is complete when the public tools retain compatible results,
the selected docs tree is read and parsed once per process, steady-state queries
perform no Markdown parsing, and the full Atlas validation matrix passes.
