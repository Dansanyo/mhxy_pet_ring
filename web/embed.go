package webui

import "embed"

//go:embed index.html styles.css app.mjs model.mjs storage.mjs
var Assets embed.FS
