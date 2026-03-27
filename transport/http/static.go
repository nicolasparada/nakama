package http

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"

	"github.com/nakamauwu/nakama/web"
)

func (h *handler) staticHandler() http.Handler {
	var root http.FileSystem
	if h.embedStaticFiles {
		sub, err := fs.Sub(web.StaticFiles, "dist")
		if err != nil {
			_ = h.logger.Log("err", fmt.Errorf("could not embed static files: %w", err))
			os.Exit(1)
		}
		root = http.FS(sub)
	} else {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			_ = h.logger.Log("err", "could not get runtime caller")
			os.Exit(1)
		}
		root = http.Dir(filepath.Join(path.Dir(file), "..", "..", "web", "dist"))
	}
	return http.FileServer(&spaFileSystem{root: root})
}

type spaFileSystem struct {
	root http.FileSystem
}

func (fs *spaFileSystem) Open(name string) (http.File, error) {
	f, err := fs.root.Open(name)
	if os.IsNotExist(err) {
		return fs.root.Open("index.html")
	}
	return f, err
}
