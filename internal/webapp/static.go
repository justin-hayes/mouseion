package webapp

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
)

//go:embed static/*
var staticFiles embed.FS

func StaticHandler() http.Handler {
	root, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(fmt.Sprintf("embedded static files are invalid: %v", err))
	}
	return http.StripPrefix("/static/", http.FileServer(http.FS(root)))
}
