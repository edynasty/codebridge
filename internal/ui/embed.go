package ui

import _ "embed"

// indexPage is the single-file configuration interface; __TOKEN__ is replaced
// with the per-run UI token before serving.
//
//go:embed index.html
var indexPage string
