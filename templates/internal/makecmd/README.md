# Make Commands

This package powers generated App make commands such as `make:event`, `make:subscriber`, `make:job`, `make:queue`, `make:schedule`, and `make:model`.

## Common Commands

```bash
forj make:event billing:invoice-paid
forj make:subscriber billing:invoice-paid
forj make:job reports:generate --queue reports
forj make:queue reports --workers 2
forj make:schedule reports:daily --every 24h
forj make:model users
```

Grouped names colocate generated files with their owning package when the command supports file generation. Resource makers such as `make:queue` update App configuration instead of creating a Go type.

`make:model` inspects your database schema, generates models + repositories, and wires repositories into Wire. It also supports relationships and per-field encryption/compression hooks.

## Quick Start

```
forj make:model users
```

Outputs:

- `internal/models/user.go`
- Repository methods and Wire registration

## Model Output

Generated model files include:

- Struct fields with `gorm` and `json` tags
- `TableName` and `Relationships` helpers
- Optional GORM hooks for encryption/compression
- A repository with basic CRUD helpers

## Relationships (.db-relationships.yaml)

Relationships are configured via `.db-relationships.yaml` and applied during generation.

Syntax:

```
<relationship_type> <local_key> -> <remote_table>:<remote_key> [via join_table:local_key:remote_key]
```

Supported types:

- `1-1` (one-to-one)
- `1-many` (one-to-many)
- `many-many` (many-to-many)
- `poly` (polymorphic has‑many)

### Examples

```
users:
  - "1-many id -> posts:user_id"
posts:
  - "1-1 user_id -> users:id"
```

Many-to-many with join table:

```
users:
  - "many-many id -> roles:id via user_roles:user_id:role_id"
```

Polymorphic:

```
posts:
  - "poly commentable_id,commentable_type -> comments:id as Commentable value=posts"
```

Notes:

- `many-many` requires `via`. If `join_remote_key` is omitted, it is inferred from the join table.
- Polymorphic relationships assume the `commentable_*` columns are on the *remote* table.

## Encryption & Compression (per field)

Use flags on `make:model` to mark fields:

```
forj make:model users --encrypt=password_hash
forj make:model users --encrypt=password_hash --compress=password_hash
```

Behavior:

- Adds `forj:"encrypt"` or `forj:"encrypt,compress"` tag to the field.
- Generates `BeforeSave` and `AfterFind` hooks on the model.
- Hooks are **idempotent**:
  - If hooks already exist and flags are omitted, they are not removed.
  - If flags change, hooks are regenerated to match the current directives.

Supported types:

- `string`
- `null.String`

Compression uses `zstd`. If compression is enabled without encryption, data is Base64‑encoded.

## Repository Generation

Each model includes a repository with common operations:

- `Builder()`, `ByID`, `All`, `GetWhere`, `FirstWhere`, `Create`, `Update`, `DeleteByID`, `DeleteWhere`

Repositories are injected into Wire automatically.

## Updating Models

Re-running `make:model` updates:

- Struct fields (add/remove/rename by schema)
- Relationship fields (adds/removes when config changes)
- `forj` tags (merged without clobbering other tags)

Custom tags (e.g., `json:"-"`) are preserved.

## Integration Tests

MySQL integration tests validate:

- Relationship generation and preload behavior
- Polymorphic scenarios
- Join key inference
- Hook generation & idempotency

Run:

```
forj test:integration -v
```
