# Upgrade Notes

Review the notes for the changes you are adopting before upgrading an existing project.

## Unreleased

### Swagger terminology is now OpenAPI

The generated API reference uses Scalar to display OpenAPI documents. Framework labels, generated filenames, and canonical routes now use OpenAPI terminology.

After installing a CLI version that includes this change, run the targeted upgrade from your project root:

```sh
forj render --only openapi
```

This updates only the two generated OpenAPI files and the documentation route call in `internal/http/server.go`, then removes the old generated filenames. Dependencies, Wire output, environment files, and other framework code remain untouched. The command supports existing WebAPI and WebUI Apps, including named Apps. Unrecognized server wiring is reported before files change. Repeating the command is safe.

| Previous name | Canonical name |
| --- | --- |
| `internal/http/swagger.go` | `internal/http/openapi.go` |
| `internal/http/swagger_test.go` | `internal/http/openapi_test.go` |
| `/swagger` and `/swagger/` | `/openapi` and `/openapi/` |
| `/swagger/doc.json` | `/openapi/doc.json` |
| `API_SWAGGER_ENABLED` | `API_OPENAPI_ENABLED` |
| CLI resource ID `swagger` | CLI resource ID `openapi` |

Existing `/swagger` URLs remain working aliases. Settings resolve in this order: `API_OPENAPI_ENABLED`, `API_SWAGGER_ENABLED`, then `SWAGGER_ENABLED`. Existing disabled settings stay effective when the canonical setting is absent. There is no need to rename external deployment settings at the same time; use the canonical name for new configuration.

Update any scripts that select the CLI documentation resource by its old `swagger` ID to use `openapi`. The OpenAPI document artifacts remain at `build/openapi.json` and `build/<app>/openapi.json`.

Review the resulting diff and rebuild your App binaries using your normal build workflow. A full `forj render` includes this migration too, when a complete framework refresh is intended.

## Earlier beta project layouts

Projects created before the App composition layout should also review the [single-app structure migration guide](migration.md).
