//go:build diagnostics_release

package web

import "embed"

//go:embed assets/index.html assets/build.json assets/assets/*
var files embed.FS
