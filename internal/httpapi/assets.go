package httpapi

import "net/http"

func serveAsset(w http.ResponseWriter, name, contentType string) {
	b, err := webFiles.ReadFile("web/" + name)
	if err != nil {
		http.Error(w, "资源不存在", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(b)
}

func (s *Server) HandleIndex(w http.ResponseWriter, r *http.Request) {
	serveAsset(w, "index.html", "text/html; charset=utf-8")
}
func (s *Server) HandleCSS(w http.ResponseWriter, r *http.Request) {
	serveAsset(w, "app.css", "text/css; charset=utf-8")
}
func (s *Server) HandleJS(w http.ResponseWriter, r *http.Request) {
	serveAsset(w, "app.js", "text/javascript; charset=utf-8")
}
