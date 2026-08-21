package webapp

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

func StaticHandler() http.Handler {
	root, _ := fs.Sub(staticFiles, "static")
	return http.StripPrefix("/static/", http.FileServer(http.FS(root)))
}
