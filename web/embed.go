package web

import "embed"

//go:embed index.html app.js settings.js style.css vendor
var FS embed.FS
