package templates

import "embed"

// FS contains the templates used to render GoForj projects.
//
//go:embed all:*
var FS embed.FS
