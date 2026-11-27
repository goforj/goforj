# Performance evidence and graphs

Read this when a library task includes performance comparisons or published graphs. Benchmarks are conditional deliverables, not a reason to run a large dataset for every library or documentation edit.

## Comparable measurements

Inspect current sibling renderers before designing charts. Search's `docs/bench/render.go` follows the queue and cache SVG patterns for horizontal bars, typography, orange/gold palette, and legends. Reuse the presentation while adapting the workloads and labels to the domain.

Choose a dataset size that answers the question within available memory, disk, and time. Search uses a million-document baseline; ten or one hundred million documents are separate scaling experiments, not prerequisites for a useful comparison. Do not mix sizes in one comparison or present selective queries over a large corpus as evidence for broad-match performance.

Use one deterministic dataset generator, schema, and query schedule across drivers. Record a fingerprint of the ordered source IDs and bodies, and validate loaded counts. Build one harness without race or coverage instrumentation and record its identity. Run drivers sequentially in isolated processes with fresh storage, releasing each engine before starting the next. Use real embedded engines and disposable service containers with verified versions and settings.

Define each workload and its expected results independently of driver output. Check hit identity, decoded data, ordering, totals and their precision, and facet counts where applicable. Time the public library call; perform correctness assertions outside the timed interval. Failed or incomplete results must not become valid samples or performance bars. Report unsupported workloads explicitly.

## Timing and provenance

State what each measurement includes. For example, search load timing includes generation, validation, bulk writes, and waiting for visibility, while excluding container startup and schema provisioning. Distinguish initial loading from updates and deletes. State warm-up, sample count, percentile calculation, client concurrency, cache conditions, and whether writes run concurrently. Sequential request latency is not concurrent throughput.

Retain raw samples and units, timestamps, corpus and harness identity, source revision, engine versions, pool and persistence settings, and host/container resources. Distinguish measured environment metadata from a human-readable machine label. Explain host-versus-container differences and limits of a single run. Large artifacts and temporary indexes stay ignored; commit the validated snapshot, methodology, and generated projections.

Keep measurement and rendering separate. A renderer validates compatible reports and generates SVGs, tables, and README content from one snapshot. `make generate` should redraw recorded data without starting engines. Test unit conversions, percentile arithmetic, incompatible-report rejection, and failure presentation. Use labeled log axes when needed for readability and disclose them; throughput bars should use a zero-based linear axis. Avoid presenting rounded close results as decisive wins.

## Investigate surprising results

Before explaining a slow driver, verify arithmetic and units, corpus identity and counts, equivalent request semantics, timing boundaries, and instrumentation. Replay through the public API. Inspect actual native requests and plans, statistics, index coverage, and work done for totals or facets, rather than attributing a library path's performance to the entire backend.

Use disposable controls to isolate hypotheses. Diagnostic runs with a smaller corpus or different settings remain separate from the published comparison. When optimization changes native storage, distinguish query improvements that work immediately from physical indexes requiring explicit reprovisioning. Preserve existing persisted layouts and consumer APIs unless an incompatible change is separately justified.

After fixing a measured bottleneck, run a fresh comparable experiment before claiming new performance. Validate and import the completed reports, then regenerate every graph and README figure together. Preserve earlier snapshots through history and link methodology or an audit when useful. Redrawing an old snapshot does not constitute a rerun or new results; report measured, generated, committed, and pushed states separately.
