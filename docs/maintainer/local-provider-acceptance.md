# Local provider acceptance

Keep the driver picker scope fixed while validating these guarantees. Passing CI alone does not establish them; the tests below exercise the filesystem boundaries and the generated application code.

| Guarantee | Evidence |
| --- | --- |
| Written dotenv values match the reviewed configuration | `publication_semantics_test.go` reparses output and rejects changed direct, transitive, and unrelated references before any write. Stable expressions remain byte-stable. The same check covers restoration. |
| Excluded resources retain their provider and database target | Selection and SQLite target tests cover named resources, App overrides, inherited settings, shared keys, whitespace, and ambiguous scopes. Unsafe combinations fail before publication. |
| First switches, repeated switches, and restores retain recovery data at every publication boundary | `TestRecoverySurvivesEveryPublicationBoundary` terminates a subprocess before and after each relevant replacement. It asserts the exact active configuration and restore point for that phase, checks credentials remain in the active file or backup, retries, and restores the original settings. |
| Absence is distinct from an explicit empty value | `TestInterruptedRestoreKeepsAbsentAndEmptySettings` adds six subprocess interruption cases. Reconciliation tests also reject mixed states. |
| Concurrent edits survive, or the operation stops with recovery intact | Finalization, failed-publication, raw-backup snapshot, and existing rollback tests exercise active and private-file edits. No finalization error rolls back a published `.env`. |
| Private recovery data stays out of ordinary Git staging | Publication tests run `git add -A` during writes and after interruption once ignore rules are published. Secret-bearing files are owner-only. Forced staging is outside this guarantee. |
| Finished restores can complete cleanup | CLI coverage confirms cleanup of an interrupted completed restore leaves `.env` byte-identical and does not request a rebuild. |
| The generated runtime selects the intended targets | `TestLocalProvidersRenderedComposition` builds both full-feature Apps after local, named-service, service, and restored phases. For each phase it runs the generated App overlay, then passes its output to the generated database resolver for both Apps. Root, named, and existing SQLite targets are checked. |

## Publication lifecycle

1. Publish ignore rules before creating secret-bearing temporary files.
2. Publish the pending recovery record, retaining the existing restore point and both sides of the changed assignments.
3. Replace `.env`. This is the commit point.
4. Verify the published `.env` and finalize the recovery record. Cleanup errors retain the pending record and explicitly report that settings changed.

Stable restore points use format version 1. Pending records use version 2, which older binaries reject instead of ignoring the transaction. Version 2 requires a pending record; version 1 forbids one. Successful cleanup returns to version 1.

Opening a pending record matches assignment presence and value against the whole changed-key set. Before `.env` publication it selects the old restore point; afterward it selects the intended new point. It keeps the raw backup as the concurrency baseline. A mixed state fails closed. A pending record with no actual difference is invalid.

Before the commit point, returned errors roll back earlier replacements when safe. If the active file changed concurrently, rollback keeps recovery data and ignore protection. After the commit point, cleanup errors do not roll back the active environment.

The subprocess matrix contains 26 hard exits: 20 publication cases and six absent/empty restoration cases. This establishes process-interruption behavior, including exits that bypass deferred cleanup. It does not establish power-loss durability, serialize external editors, provision services, or move database contents. A stale exclusive lock must be removed only after its writer has stopped.

## Run the checks

```sh
go test ./internal/localproviders ./internal/forj ./internal/generate -count=1
go test -race ./internal/localproviders -count=1
go vet ./...
go test -tags=integration ./internal/forj -run '^TestLocalProvidersRenderedComposition$' -count=1 -v
```

Use the repository-pinned Go and Wire versions. Rendered projects must be created under `/tmp`. The integration test builds ordinary Apps using published dependencies with workspace resolution disabled, and checks that no local module replacements were selected.
