package httpapi

import (
	"embed"
	"io/fs"
)

//go:embed webdist/*
var webAssets embed.FS

func loadEmbeddedIndexHTML() ([]byte, error) {
	return fs.ReadFile(webAssets, "webdist/index.html")
}
