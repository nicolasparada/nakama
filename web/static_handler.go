package web

import (
	"embed"
	"net/http"
)

//go:embed all:static
var staticFS embed.FS

func staticHandler() http.Handler {
	return http.FileServer(http.FS(staticFS))
}
