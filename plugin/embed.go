// Package plugin holds the files installed into assistants.
package plugin

import "embed"

// OpenCodeFiles is the OpenCode TUI plugin.
//
//go:embed opencode
var OpenCodeFiles embed.FS
