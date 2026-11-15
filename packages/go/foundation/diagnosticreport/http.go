package diagnosticreport

import "net/http"

// Readiness observes only the existing fixed readiness response. All other
// routes receive the original writer, preserving streaming/upgrade semantics.
func Readiness(next http.Handler, sink Sink) http.Handler {
	sink = OrNoop(sink)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/readyz" {
			next.ServeHTTP(w, r)
			return
		}
		writer := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(writer, r)
		sink.Dependency("readiness", writer.status >= 200 && writer.status < 300, map[bool]string{true: "", false: "dependency_unavailable"}[writer.status >= 200 && writer.status < 300])
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
