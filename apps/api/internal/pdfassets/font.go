package pdfassets

import _ "embed"

//go:embed inter-400.ttf
var inter []byte

// Inter returns the bundled report and proposal font.
func Inter() []byte { return inter }
