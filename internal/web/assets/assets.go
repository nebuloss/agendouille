// Package assets embeds the static files and the two TypeScript bundles,
// and serves each bundle only on the surface it belongs to.
//
// The family web app's bundle (static/app/app.js) is served where the
// public surfaces are, and the admin UI's (static/admin/admin.js) where the
// admin surface is. A listener that serves the public surfaces only never
// hands out the admin code (PLAN.md 9.1).
package assets

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"html/template"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed static templates
var files embed.FS

// Templates parses every .html under templates/, each named by its path
// relative to templates/ ("layout.html", "app/signin.html").
func Templates(funcs template.FuncMap) (*template.Template, error) {
	root := template.New("").Funcs(funcs)
	err := fs.WalkDir(files, "templates", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Ext(p) != ".html" {
			return err
		}
		body, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		_, err = root.New(strings.TrimPrefix(p, "templates/")).Parse(string(body))
		return err
	})
	return root, err
}

// Bundle names one of the two TypeScript bundles.
type Bundle string

const (
	App   Bundle = "app"   // the family web app
	Admin Bundle = "admin" // the admin UI
)

// Path is the bundle's file inside the embedded tree.
func (b Bundle) Path() string { return "static/" + string(b) + "/" + string(b) + ".js" }

// URL is where a page loads the bundle from.
func (b Bundle) URL() string { return "/" + b.Path() }

// built records, at start, which bundles were present at compile time, so
// a layout skips a script tag that would only 404.
var built = map[Bundle]bool{App: present(App.Path()), Admin: present(Admin.Path())}

func present(p string) bool {
	info, err := fs.Stat(files, p)
	return err == nil && info.Size() > 0
}

// Built reports whether this binary carries bundle b.
func Built(b Bundle) bool { return built[b] }

type asset struct {
	body  []byte
	etag  string
	ctype string
}

// Register serves GET /static/<bundle>/... and /static/shared/... on mux.
// Nothing of the other bundle is reachable through it.
func Register(mux *http.ServeMux, b Bundle) error {
	byPath := map[string]asset{}
	err := fs.WalkDir(files, "static", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Base(p) == "README.txt" {
			return err
		}
		if !strings.HasPrefix(p, "static/"+string(b)+"/") && !strings.HasPrefix(p, "static/shared/") {
			return nil
		}
		body, err := files.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		ctype := mime.TypeByExtension(path.Ext(p))
		if path.Ext(p) == ".webmanifest" {
			ctype = "application/manifest+json"
		}
		if ctype == "" {
			ctype = "application/octet-stream"
		}
		byPath[p] = asset{body: body, etag: `"` + hex.EncodeToString(sum[:12]) + `"`, ctype: ctype}
		return nil
	})
	if err != nil {
		return err
	}
	handler := func(w http.ResponseWriter, r *http.Request) {
		serve(w, r, byPath, strings.TrimPrefix(r.URL.Path, "/"))
	}
	mux.HandleFunc("GET /static/"+string(b)+"/", handler)
	mux.HandleFunc("GET /static/shared/", handler)
	return nil
}

func serve(w http.ResponseWriter, r *http.Request, byPath map[string]asset, p string) {
	a, ok := byPath[p]
	if !ok || len(a.body) == 0 {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", a.ctype)
	h.Set("ETag", a.etag)
	// The bundles have fixed names, so every load revalidates; the ETag
	// makes that a 304.
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, path.Base(p), time.Time{}, bytes.NewReader(a.body))
}

// Icon is the andouille, as SVG: the favicon of every surface and the
// brand in the top bars.
func Icon() []byte {
	b, _ := files.ReadFile("static/shared/icon.svg")
	return b
}
