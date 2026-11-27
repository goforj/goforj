# Security and persistence

Read this for libraries that persist sensitive values or make encryption claims. Do not impose encrypted storage, key management, or rotation on a pure utility or a deliberately read-only mount reader.

## Establish the protection boundary

Challenge the design against the complete consumer workflow. For secret storage, establish who provides encryption at rest, authentication of stored data, access control, key persistence, recovery, and rotation. A cloud service can own some of these responsibilities; a local backend must not imply the service provides them. Reading plaintext files delivered by Docker or Kubernetes is a distinct capability from storing encrypted local secrets.

State which guarantees the library implements and which the caller or provider must supply. Resolve missing essentials before a release-readiness claim. Passing tests for a narrower implementation cannot establish that the intended product is complete. Do not silently change existing file semantics to close a scope gap.

## Encryption and key ownership

Use maintained standard cryptographic primitives with authenticated encryption. Specify nonce requirements, associated-data identity, key ownership, a bounded and versioned persisted format, and what happens on malformed data or a wrong key. Authentication failures must return no partial plaintext. For local encrypted storage, a wrong key must not silently overwrite or delete existing ciphertext.

Keep key material separate from ciphertext and diagnostics. A disposable random key in an example is not a persistent-key deployment strategy. Explain how the application supplies and retains its key, the consequence of losing it, and any algorithm-specific usage limits. Do not generate a fresh production key on every startup or store it beside the encrypted files as an implicit convenience.

Make format compatibility and plaintext migration explicit. Preserve existing APIs and formats where possible; an added encrypted store must not silently reinterpret an established plaintext mount reader.

## Graceful rotation

For a multi-key design, a useful default is current-key encryption with current-then-previous-key decryption. Keep ordinary reads read-only unless read-time migration is an explicit, justified contract. Do not infer automatic rewriting, key generation, or key retirement from another framework's term "rotation"; verify its actual behavior.

An explicit backend rotation operation can migrate existing files without widening the domain-facing Reader or Store interfaces. Hold the same per-binding mutation lock across authentication and replacement so a concurrent update cannot be overwritten by stale migration data. Skip already-current data, and define how missing resources behave. Document partial completion, cancellation, safe retries, and the scope of a successful run. Configured bindings do not include unbound files or backups.

Retiring a key from live configuration and destroying the last recovery copy are separate actions. Keep old keys recoverable for unprocessed data and retained backups. Rolling deployments must establish compatible readers and switch writers before retirement; the library must not silently orchestrate that deployment.

## Filesystem and failure evidence

For encrypted local writes, stage only ciphertext with restrictive permissions, publish through the platform's safe replacement primitive, and define file and directory durability. Distinguish prepublication failure from an operation that may have committed. Directory confinement does not provide encryption, cross-process locking, or crash durability. Atomic replacement and directory syncing are platform-specific claims that need matching evidence.

Test exact empty and binary values, maximum sizes, wrong and copied keys, tampering, truncation, cross-binding substitution, reopening, old-key reads, current-key writes, partial rotation, retries, cancellation, and competing mutations as applicable. Use real files plus deterministic failure injection. Subprocess crash/restart testing addresses a different boundary from injected write errors. Do not claim power-loss testing from either. Verify claimed operating systems through execution, not cross-compilation alone.

Document actual limits without promising rollback detection, filename secrecy, secure erasure of snapshots, memory zeroization, or cross-process coordination unless implemented and tested. Exact unit coverage is useful evidence, not a cryptographic audit or proof of domain completeness.
