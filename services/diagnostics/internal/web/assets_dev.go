//go:build !diagnostics_release

package web

import "embed"

//go:embed assets/*
var files embed.FS
