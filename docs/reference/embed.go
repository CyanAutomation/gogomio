// Package reference embeds the checked-in OpenAPI specifications and docs page.
package reference

import "embed"

// Files contains the OpenAPI documents and the API reference page.
//
//go:embed index.html openapi.json openapi.yaml
var Files embed.FS
