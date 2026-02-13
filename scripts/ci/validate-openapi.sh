#!/usr/bin/env bash
set -euo pipefail

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

cat <<'EOF' > "$TMP_DIR/go.mod"
module example.com/openapi-fixture

go 1.24
EOF

cat <<'EOF' > "$TMP_DIR/.env"
APP_NAME=OpenAPI Validation Fixture
EOF

mkdir -p "$TMP_DIR/internal/hello"
cat <<'EOF' > "$TMP_DIR/internal/hello/controller.go"
package hello

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type payload struct {
	Name string `json:"name"`
}

type Controller struct{}

func (c *Controller) Routes() []any {
	return []any{
		http.NewRoute(http.MethodGet, "/items/:id", c.Get),
		http.NewRoute(http.MethodPost, "/items", c.Create),
	}
}

func (c *Controller) Get(ctx echo.Context) error {
	if ctx.QueryParam("full") == "1" {
		return ctx.JSON(http.StatusOK, map[string]any{"ok": true, "id": ctx.Param("id"), "mode": "full"})
	}
	return ctx.JSON(http.StatusOK, map[string]any{"ok": true, "id": ctx.Param("id")})
}

func (c *Controller) Create(ctx echo.Context) error {
	var in payload
	if err := ctx.Bind(&in); err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]any{"ok": false, "error": "bad payload"})
	}
	return ctx.JSON(http.StatusCreated, map[string]any{"ok": true})
}
EOF

forj build:api-index \
	--root "$TMP_DIR" \
	--out "$TMP_DIR/build/api_index.json" \
	--diagnostics "$TMP_DIR/build/api_index.diagnostics.json" \
	--open-api "$TMP_DIR/build/openapi.json"

docker run --rm \
	-v "$TMP_DIR/build:/work" \
	openapitools/openapi-generator-cli:v7.6.0 \
	validate -i /work/openapi.json

echo "OpenAPI validation passed"
