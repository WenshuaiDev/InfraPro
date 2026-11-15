package web

import (
	"encoding/json"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
)

var BuildID = "dev"

// VerifyRelease is an offline release gate, also usable inside a final image.
func VerifyRelease() bool {
	var metadata struct {
		BuildID  string `json:"build_id"`
		Manifest map[string]struct {
			File   string   `json:"file"`
			CSS    []string `json:"css"`
			Assets []string `json:"assets"`
		} `json:"asset_manifest"`
	}
	raw, err := files.ReadFile("assets/build.json")
	if err != nil || json.Unmarshal(raw, &metadata) != nil || BuildID == "dev" || metadata.BuildID != BuildID {
		return false
	}
	html, err := files.ReadFile("assets/index.html")
	if err != nil || strings.Contains(string(html), "ui-not-built") || strings.Contains(string(html), "@vite/client") {
		return false
	}
	for _, entry := range metadata.Manifest {
		for _, name := range append(append([]string{entry.File}, entry.CSS...), entry.Assets...) {
			if strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
				return false
			}
			if _, err := files.ReadFile("assets/" + name); err != nil {
				return false
			}
		}
	}
	return len(metadata.Manifest) > 0
}

func Ready() bool { _, e := files.ReadFile("assets/index.html"); return e == nil }
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "." {
			name = ""
		}
		if strings.HasPrefix(name, "api/") || strings.HasPrefix(name, "observer/") {
			http.NotFound(w, r)
			return
		}
		if name == "" || name == "login" || name == "components" || name == "nodes" || name == "history" || name == "self" || strings.HasPrefix(name, "components/") || strings.HasPrefix(name, "nodes/") || strings.HasPrefix(name, "replicas/") {
			name = "index.html"
			if !Ready() {
				name = "ui-not-built.html"
			}
		}
		data, e := fs.ReadFile(files, "assets/"+name)
		if e != nil {
			http.NotFound(w, r)
			return
		}
		typ := mime.TypeByExtension(path.Ext(name))
		if typ == "" {
			typ = "application/octet-stream"
		}
		w.Header().Set("Content-Type", typ)
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.Method == http.MethodGet {
			_, _ = w.Write(data)
		}
	})
}
