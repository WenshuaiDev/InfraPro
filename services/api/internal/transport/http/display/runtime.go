package displayhttp

import (
	"encoding/json"
	"fmt"
	"github.com/WenshuaiDev/InfraPro/packages/go/platform/display"
	"io"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func readBody(w http.ResponseWriter, r *http.Request, v any) error {
	b, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if e != nil {
		return &display.Error{Code: "query_invalid", Message: "请求超过限制。", Status: 413}
	}
	return display.Decode(b, v)
}
func setRuntimeCookie(w http.ResponseWriter, o display.Options, session display.Session, token string) {
	http.SetCookie(w, &http.Cookie{Name: "infrapro_display", Value: token, Path: "/runtime/" + session.ID + "/", HttpOnly: true, Secure: o.Secure, SameSite: http.SameSiteStrictMode, MaxAge: 7200})
}
func setTerminalCookie(w http.ResponseWriter, o display.Options, token string) {
	http.SetCookie(w, &http.Cookie{Name: "infrapro_terminal", Value: token, Path: "/runtime/terminal/", HttpOnly: true, Secure: o.Secure, SameSite: http.SameSiteStrictMode, MaxAge: 366 * 86400})
}
func terminalCookie(r *http.Request) string {
	c, e := r.Cookie("infrapro_terminal")
	if e != nil {
		return ""
	}
	return c.Value
}
func runtimeToken(r *http.Request) string {
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	c, e := r.Cookie("infrapro_display")
	if e != nil {
		return ""
	}
	return c.Value
}
func runtimeError(code, message string, status int) error {
	return &display.Error{Code: code, Message: message, Status: status}
}
func baseHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
}

// RuntimeHandler is intentionally separate from the management router. It
// cannot route a package call to a personnel API, upload, or business mutation.
func RuntimeHandler(s *display.Service) http.Handler {
	o := s.Options()
	origin, _ := url.Parse(o.RuntimeOrigin)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if e := s.Readiness(r.Context()); e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 200, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /internal/metrics", func(w http.ResponseWriter, r *http.Request) {
		host, _, e := net.SplitHostPort(r.RemoteAddr)
		ip := net.ParseIP(host)
		if e != nil || ip == nil || !ip.IsLoopback() {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, 200, s.Metrics())
	})
	mux.HandleFunc("POST /tool/exchange", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Code     string `json:"code"`
			Verifier string `json:"verifier"`
			Audience string `json:"audience"`
		}
		if e := readBody(w, r, &in); e != nil {
			writeError(w, e)
			return
		}
		out, e := s.ExchangeTool(r.Context(), in.Code, in.Verifier, in.Audience)
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /tool/{session}/{action}", func(w http.ResponseWriter, r *http.Request) {
		action := r.PathValue("action")
		token := runtimeToken(r)
		if action == "part" {
			part, e := strconv.Atoi(r.URL.Query().Get("part"))
			if e != nil {
				writeError(w, runtimeError("query_invalid", "分片无效。", 422))
				return
			}
			controller := http.NewResponseController(w)
			_ = controller.SetReadDeadline(time.Now().Add(2 * time.Minute))
			_ = controller.SetWriteDeadline(time.Now().Add(3 * time.Minute))
			data, e := io.ReadAll(http.MaxBytesReader(w, r.Body, display.PartSize))
			if e == nil {
				e = s.ToolUploadPart(r.Context(), r.PathValue("session"), token, r.URL.Query().Get("upload"), part, r.Header.Get("X-Content-SHA256"), data)
			}
			if e != nil {
				writeError(w, e)
				return
			}
			writeJSON(w, 200, map[string]bool{"received": true})
			return
		}
		var in display.Command
		if e := readBody(w, r, &in); e != nil {
			writeError(w, e)
			return
		}
		out, e := s.ToolCommand(r.Context(), r.PathValue("session"), token, action, r.URL.Query().Get("target"), in)
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /runtime/exchange", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Code     string `json:"code"`
			Verifier string `json:"verifier"`
			Audience string `json:"audience"`
			Tool     bool   `json:"tool"`
		}
		if e := readBody(w, r, &in); e != nil {
			writeError(w, e)
			return
		}
		token, asset, session, e := s.Exchange(r.Context(), in.Code, in.Verifier, in.Audience)
		if e != nil {
			writeError(w, e)
			return
		}
		setRuntimeCookie(w, o, session, token)
		boot, e := s.Bootstrap(r.Context(), session, asset)
		if e != nil {
			writeError(w, e)
			return
		}
		if in.Tool {
			if in.Audience == o.RuntimeOrigin {
				writeError(w, runtimeError("query_invalid", "工具兑换必须使用本机配对来源。", 422))
				return
			}
			boot["tool_token"] = token
		}
		writeJSON(w, 200, boot)
	})
	mux.HandleFunc("POST /runtime/terminal/pair", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Code string `json:"code"`
		}
		if e := readBody(w, r, &in); e != nil {
			writeError(w, e)
			return
		}
		token, e := s.PairTerminal(r.Context(), in.Code)
		if e != nil {
			writeError(w, e)
			return
		}
		setTerminalCookie(w, o, token)
		writeJSON(w, 200, map[string]bool{"paired": true})
	})
	mux.HandleFunc("POST /runtime/terminal/start", func(w http.ResponseWriter, r *http.Request) {
		token, asset, renewal, session, e := s.StartTerminal(r.Context(), terminalCookie(r))
		if e != nil {
			writeError(w, e)
			return
		}
		setRuntimeCookie(w, o, session, token)
		setTerminalCookie(w, o, renewal)
		boot, e := s.Bootstrap(r.Context(), session, asset)
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 200, boot)
	})
	mux.HandleFunc("/runtime/{session}/{action}", func(w http.ResponseWriter, r *http.Request) {
		session, e := s.Session(r.Context(), runtimeToken(r))
		if e != nil {
			writeError(w, e)
			return
		}
		if session.ID != r.PathValue("session") {
			writeError(w, runtimeError("not_found", "会话不可访问。", 404))
			return
		}
		action := r.PathValue("action")
		if action == "events" && r.Method == "GET" {
			serveEvents(w, r, s, session)
			return
		}
		if r.Method != "POST" {
			writeError(w, runtimeError("method_not_allowed", "此操作需要 POST。", 405))
			return
		}
		var out any
		switch action {
		case "bootstrap":
			var asset string
			asset, e = s.IssueAsset(r.Context(), session)
			if e == nil {
				out, e = s.Bootstrap(r.Context(), session, asset)
			}
		case "renew":
			_, session, e = s.Renew(r.Context(), session)
			out = session
		case "switch":
			var token, asset string
			token, asset, session, e = s.Switch(r.Context(), session)
			if e == nil {
				setRuntimeCookie(w, o, session, token)
				out, e = s.Bootstrap(r.Context(), session, asset)
			}
		case "query":
			var in display.Call
			e = readBody(w, r, &in)
			if e == nil {
				out, e = s.Query(r.Context(), session, in)
			}
		case "bindings":
			var in struct {
				Name string `json:"name"`
			}
			e = readBody(w, r, &in)
			if e == nil {
				out, e = s.Bindings(r.Context(), session, in.Name)
			}
		case "subscribe":
			var in display.SubscriptionInput
			e = readBody(w, r, &in)
			if e == nil {
				var id string
				id, e = s.Subscribe(r.Context(), session, in)
				out = map[string]string{"id": id}
			}
		case "unsubscribe":
			var in struct {
				ID string `json:"id"`
			}
			e = readBody(w, r, &in)
			if e == nil {
				e = s.CancelSubscription(r.Context(), session, in.ID)
			}
			out = map[string]bool{"canceled": e == nil}
		case "ack":
			var in struct {
				ID         string `json:"id"`
				Generation string `json:"generation"`
				Sequence   string `json:"sequence"`
			}
			e = readBody(w, r, &in)
			if e == nil {
				e = s.Ack(r.Context(), session, in.ID, in.Generation, in.Sequence)
			}
			out = map[string]bool{"applied": e == nil}
		case "control":
			var in struct {
				Action string `json:"action"`
				Code   string `json:"code"`
			}
			e = readBody(w, r, &in)
			if e == nil {
				e = s.RuntimeControl(r.Context(), session, in.Action, in.Code)
			}
			out = map[string]bool{"received": e == nil}
		case "navigation":
			var in struct {
				Kind string `json:"kind"`
				ID   string `json:"id"`
			}
			e = readBody(w, r, &in)
			if e == nil {
				var target string
				target, e = s.Navigation(r.Context(), session, in.Kind, in.ID)
				out = map[string]string{"url": target}
			}
		default:
			e = runtimeError("not_found", "运行操作不存在。", 404)
		}
		if e != nil {
			writeError(w, e)
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("GET /runtime-config", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"console_url": o.ConsoleOrigin, "runtime_origin": o.RuntimeOrigin, "asset_origin": o.AssetOrigin, "protocol": 1, "switch_grace_ms": o.SwitchGrace.Milliseconds()})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" && r.Method != "HEAD" {
			writeError(w, runtimeError("method_not_allowed", "只允许读取运行壳。", 405))
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" || name == "terminal" || strings.HasPrefix(name, "display/") || name == "exchange" {
			name = "index.html"
		}
		if !display.SafePath(name) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-src "+o.AssetOrigin+"; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		http.ServeFile(w, r, filepath.Join(o.ShellDirectory, filepath.FromSlash(name)))
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		baseHeaders(w)
		if r.Host != origin.Host {
			writeError(w, runtimeError("origin_rejected", "运行地址与配置不匹配。", 403))
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if r.Header.Get("Origin") != o.RuntimeOrigin || r.Header.Get("X-Display-Protocol") != "1" {
				writeError(w, runtimeError("origin_rejected", "运行请求来源或协议无效。", 403))
				return
			}
		} else if r.Header.Get("Sec-Fetch-Site") == "cross-site" && strings.HasPrefix(r.URL.Path, "/runtime/") {
			writeError(w, runtimeError("origin_rejected", "不允许跨站读取。", 403))
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func serveEvents(w http.ResponseWriter, r *http.Request, s *display.Service, session display.Session) {
	started := time.Now()
	defer func() { s.Measure("stream_connections", started, false) }()
	if last := r.Header.Get("Last-Event-ID"); last != "" && !display.ParseEventID(last) {
		writeError(w, runtimeError("query_invalid", "事件游标无效。", 422))
		return
	}
	done, limitErr := s.OpenStream(session)
	if limitErr != nil {
		s.RecordDiagnostic(r.Context(), session, "budget_rejected", "stream_limit")
		writeError(w, limitErr)
		return
	}
	defer done()
	s.StreamConnected(r.Context(), session)
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, runtimeError("dependency_unavailable", "流式传输不可用。", 503))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	_, _ = io.WriteString(w, ": connected\n\n")
	flusher.Flush()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	connection := time.NewTimer(25 * time.Second)
	defer connection.Stop()
	lastStatus := ""
	for {
		select {
		case <-r.Context().Done():
			return
		case <-connection.C:
			return
		case <-tick.C:
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Second))
			// Session is reloaded for every delivery; a live TCP connection is no authority.
			current, e := s.Session(r.Context(), runtimeToken(r))
			if e != nil {
				p := PublicError(e)
				s.RecordDiagnostic(r.Context(), session, "session_ended", p.Code)
				raw := fmt.Sprintf(`{"kind":"revoked","code":%q}`, p.Code)
				_, _ = fmt.Fprintf(w, "event: control\ndata: %s\n\n", raw)
				flusher.Flush()
				return
			}
			session = current
			status := session.TargetRelease + ":" + session.State
			if status != lastStatus {
				_, _ = fmt.Fprintf(w, "event: control\ndata: {\"kind\":\"status\",\"target_release_id\":%q,\"actual_release_id\":%q,\"lease_until\":%q}\n\n", session.TargetRelease, session.ReleaseID, session.LeaseUntil.Format(time.RFC3339Nano))
				lastStatus = status
			}
			events, e := s.Poll(r.Context(), session)
			if e != nil {
				p := PublicError(e)
				_, _ = fmt.Fprintf(w, "event: control\ndata: {\"kind\":\"status\",\"code\":%q}\n\n", p.Code)
				flusher.Flush()
				return
			}
			for _, event := range events {
				if event.Result != nil {
					if at, err := time.Parse(time.RFC3339Nano, event.Result.ReadAt); err == nil {
						s.Measure("event_delivery", at, false)
					}
				}
				raw := marshal(event)
				_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Second))
				if _, e = fmt.Fprintf(w, "id: %s\nevent: data\ndata: %s\n\n", display.EventID(event), raw); e != nil {
					return
				}
			}
			flusher.Flush()
		}
	}
}
func AssetHandler(s *display.Service) http.Handler {
	o := s.Options()
	origin, _ := url.Parse(o.AssetOrigin)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		baseHeaders(w)
		if r.Host != origin.Host || r.Method != "GET" && r.Method != "HEAD" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/healthz" {
			writeJSON(w, 200, map[string]string{"status": "ok"})
			return
		}
		if r.URL.Path == "/readyz" {
			if e := s.Readiness(r.Context()); e != nil {
				writeError(w, e)
				return
			}
			writeJSON(w, 200, map[string]string{"status": "ready"})
			return
		}
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/assets/"), "/", 2)
		if !strings.HasPrefix(r.URL.Path, "/assets/") || len(parts) != 2 {
			http.NotFound(w, r)
			return
		}
		file, data, e := s.Asset(r.Context(), parts[0], parts[1])
		if e != nil {
			writeError(w, e)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
		w.Header().Set("Content-Type", file.MediaType)
		w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src "+o.AssetOrigin+" 'unsafe-inline' 'wasm-unsafe-eval'; style-src "+o.AssetOrigin+" 'unsafe-inline'; img-src "+o.AssetOrigin+" data: blob:; font-src "+o.AssetOrigin+" data:; connect-src "+o.AssetOrigin+"; worker-src blob:; media-src blob:; frame-ancestors "+o.RuntimeOrigin+"; base-uri 'none'; form-action 'none'; sandbox allow-scripts")
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Content-Length", fmt.Sprint(len(data)))
		if r.Method == "GET" {
			_, _ = w.Write(data)
		}
	})
}
func marshal(v any) []byte { b, _ := json.Marshal(v); return b }
