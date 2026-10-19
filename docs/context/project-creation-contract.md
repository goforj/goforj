# Project creation contract

`forj new` remains interactive by default. Graphical clients can discover versioned options and submit explicit argv without parsing terminal output:

```sh
forj new --schema
# A single JSON object with schema_version: 1, goforj_version, and fields.

forj new --no-interaction \
  --name="My application" \
  --module=example.com/myapp \
  --path=/tmp/myapp \
  --components=cli,web_api,web_ui,database_sqlite \
  --starter-kit=vue \
  --component-library=false \
  --help-format=guided \
  --atlas=skip
# Project initialized at /tmp/myapp
```

The schema contains ordered fields with CLI keys, labels, descriptions, input kinds, steps, defaults, and choices. Supported input kinds are `text`, `path`, `multi`, `select`, and `boolean`. Selectable component choices carry `requires` and `exclusive_group` metadata. Field-level `requires` refers to choices in the field named by `depends_on`, for example starter-kit availability requires `web_ui` in `components`. A field's `when` map shows it only when the named fields have the exact advertised values. Semantic `role` values identify the display name and destination directory without relying on CLI flag names.

Selectable options can include optional `preview` text. Help-format previews come from the same real formatters used by the TUI. Clients display the selected preview as terminal text, never as HTML, and omit the preview panel when the metadata is absent.

Catalogs come from the same source as the TUI. Clients should send each available field as `--<key>=<value>`, join multiselect keys with commas, and serialize booleans as `true` or `false`. Pass argv directly, never construct a shell command. An explicitly empty `--components=` selects no optional components; omitting the argument preserves framework defaults.

GoForj validates names, module paths, component keys, database exclusivity, starter-kit requirements, preset values, and destination availability before publishing configuration. Both creation paths share resource resolution, dependency expansion, task generation, Atlas installation, and the project renderer. Creation does not start the new runtime.

Schema version 1 exposes the Recommended, Minimal, Custom, and Skip Atlas presets. Custom shows `atlas-agents` and `atlas-surfaces`; pass their catalog keys as comma-separated values, for example `--atlas=custom --atlas-agents=codex --atlas-surfaces=guidelines,skills`. At least one agent and one integration are required. Inactive custom fields must be omitted from argv. The Demo profile applies the same MySQL and frontend constraints as the terminal wizard. A graphical review should make that profile explicit.

`--schema` emits JSON only and never initializes a directory. Noninteractive creation requires an explicit name, module, and destination, rejects a non-empty destination by default, and creates `.goforj.yml` exclusively. Do not offer `--allow-non-empty` in the graphical flow. A failed render leaves its partial files for diagnosis; clients must not automatically delete them or retry creation into the occupied directory.

Consumers must reject unknown schema versions and input kinds. A CLI without `new --schema` needs an update, not a fallback that attempts to drive its TUI. A schema probe does not grant permission to run arbitrary flags or commands.
