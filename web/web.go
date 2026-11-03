// Package web embeds the templates and static files, so the binary serves them without a build step.
package web

import "embed"

//go:embed templates static
var FS embed.FS
