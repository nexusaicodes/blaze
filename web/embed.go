// Package web holds the browser UI Blaze serves: plain HTML, CSS and ES
// modules with no build step.
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var files embed.FS

// Static returns the UI files rooted at static/.
func Static() fs.FS {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		// static is embedded at compile time, so this cannot fail at run
		// time.
		panic(err)
	}
	return sub
}
