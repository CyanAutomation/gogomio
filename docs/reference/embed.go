// Package reference embeds the checked-in API specification and its index page.
package reference

import "embed"

// Files contains the OpenAPI documents and the API reference page.
//
//go:embed index.html swagger.json swagger.yaml
var Files embed.FS
