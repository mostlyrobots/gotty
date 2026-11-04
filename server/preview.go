package server

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sorenisanerd/gotty/pkg/auth"
)

const previewTargetCookie = "gotty_preview_target"

// PreviewEndpoint describes a selectable local IP/hostname on the remote system.
type PreviewEndpoint struct {
	Host  string `json:"host"`
	Label string `json:"label"`
	Iface string `json:"iface,omitempty"`
}

// PreviewTargetConfig holds the active preview target configuration.
type PreviewTargetConfig struct {
	TargetHost      string `json:"target_host"`
	TargetPort      int    `json:"target_port"`
	Scheme          string `json:"scheme"` // "http" | "https" | "auto"
	ForwarderPort   int    `json:"forwarder_port"`
	ForwarderActive bool   `json:"forwarder_active"`
	PreviewURL      string `json:"preview_url"`
}

// PreviewManager manages the Web Preview reverse proxy and dynamic port 8080 forwarder.
type PreviewManager struct {
	mu        sync.RWMutex
	current   PreviewTargetConfig
	endpoints []PreviewEndpoint
	listener  net.Listener
	srv       *http.Server
	transport *http.Transport
}

// NewPreviewManager creates a new PreviewManager initialized to forward to localhost:8080
// and dynamically introspects local network interfaces at startup.
func NewPreviewManager() *PreviewManager {
	tr := &http.Transport{
		Proxy: nil,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSClientConfig:       &tls.Config{InsecureSkipVerify: true}, // Allow self-signed local certs
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	pm := &PreviewManager{
		current: PreviewTargetConfig{
			TargetHost:    "localhost",
			TargetPort:    8080,
			Scheme:        "auto",
			ForwarderPort: 8080,
			PreviewURL:    "./preview/localhost:8080/",
		},
		endpoints: discoverPreviewEndpoints(),
		transport: tr,
	}
	return pm
}

// Close stops the dynamic port 8080 forwarder if running.
func (pm *PreviewManager) Close() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.srv != nil {
		_ = pm.srv.Close()
		pm.srv = nil
	}
	if pm.listener != nil {
		_ = pm.listener.Close()
		pm.listener = nil
	}
	pm.current.ForwarderActive = false
}

// SetTarget updates the active preview target and reconfigures the port 8080 local forwarder.
func (pm *PreviewManager) SetTarget(host string, port int, scheme string) (PreviewTargetConfig, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		host = "localhost"
	}
	if !isAllowedPreviewHost(host) {
		return PreviewTargetConfig{}, fmt.Errorf("target host %q is not a permitted local/private address", host)
	}
	if port < 1 || port > 65535 {
		return PreviewTargetConfig{}, fmt.Errorf("invalid port %d (must be 1-65535)", port)
	}
	scheme = strings.ToLower(strings.TrimSpace(scheme))
	if scheme != "http" && scheme != "https" {
		scheme = "auto"
	}

	pm.mu.Lock()
	defer pm.mu.Unlock()

	hostPort := net.JoinHostPort(host, strconv.Itoa(port))
	previewURL := fmt.Sprintf("./preview/%s/", hostPort)
	if scheme == "https" {
		previewURL = fmt.Sprintf("./preview/https/%s/", hostPort)
	}

	pm.current.TargetHost = host
	pm.current.TargetPort = port
	pm.current.Scheme = scheme
	pm.current.PreviewURL = previewURL

	// If target is port 8080 on localhost/loopback, do not bind 8080 to avoid a self-loop
	isLoopback8080 := port == 8080 && (host == "localhost" || host == "127.0.0.1" || host == "::1")
	if isLoopback8080 {
		if pm.srv != nil {
			_ = pm.srv.Close()
			pm.srv = nil
			pm.listener = nil
		}
		pm.current.ForwarderActive = false
		return pm.current, nil
	}

	// Ensure port 8080 dynamic forwarder is running if port 8080 is available
	if pm.srv == nil {
		ln, err := net.Listen("tcp", "127.0.0.1:8080")
		if err == nil {
			pm.listener = ln
			pm.srv = &http.Server{
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					pm.mu.RLock()
					cfg := pm.current
					pm.mu.RUnlock()
					pm.serveReverseProxy(w, r, cfg.TargetHost, cfg.TargetPort, cfg.Scheme, "")
				}),
			}
			pm.current.ForwarderActive = true
			go func(s *http.Server, l net.Listener) {
				_ = s.Serve(l)
			}(pm.srv, ln)
		} else {
			pm.current.ForwarderActive = false
		}
	}

	return pm.current, nil
}

// GetState returns the current target config and all introspected local endpoints.
func (pm *PreviewManager) GetState() (PreviewTargetConfig, []PreviewEndpoint) {
	pm.mu.RLock()
	cfg := pm.current
	cached := pm.endpoints
	pm.mu.RUnlock()

	live := discoverPreviewEndpoints()
	if len(live) > 0 {
		pm.mu.Lock()
		pm.endpoints = live
		pm.mu.Unlock()
		return cfg, live
	}
	return cfg, cached
}

// discoverPreviewEndpoints dynamically introspects active network interfaces on the system.
func discoverPreviewEndpoints() []PreviewEndpoint {
	seen := make(map[string]bool)
	var out []PreviewEndpoint

	add := func(host, label, iface string) {
		if seen[host] {
			return
		}
		seen[host] = true
		out = append(out, PreviewEndpoint{
			Host:  host,
			Label: label,
			Iface: iface,
		})
	}

	add("localhost", "localhost (127.0.0.1)", "lo")

	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip4 := ip.To4()
			if ip4 == nil {
				continue
			}
			// Only include private / CGNAT interface IPs (do not leak public WAN IPs)
			if isPrivateOrLocalIP(ip4) {
				ipStr := ip4.String()
				add(ipStr, fmt.Sprintf("%s (%s)", ipStr, iface.Name), iface.Name)
			}
		}
	}

	return out
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func isAllowedPreviewHostForUser(host string, user *auth.UserIdentity) bool {
	if !isAllowedPreviewHost(host) {
		return false
	}
	if user != nil && !user.IsHostAccess {
		return isLoopbackHost(host)
	}
	return true
}

func isPrivateOrLocalIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() {
		return true
	}
	// Allow CGNAT / Tailscale range 100.64.0.0/10
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
	}
	return false
}

// isAllowedPreviewHost validates that host is localhost or a private/local IP on the remote machine.
func isAllowedPreviewHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	// Block cloud link-local metadata 169.254.169.254
	if ip.IsLinkLocalUnicast() {
		return false
	}
	return isPrivateOrLocalIP(ip)
}

// resolvePreviewScheme determines whether the target endpoint speaks HTTPS or HTTP.
func (pm *PreviewManager) resolvePreviewScheme(host string, port int, requestedScheme string) string {
	if requestedScheme == "http" || requestedScheme == "https" {
		return requestedScheme
	}
	if port == 443 || port == 8443 {
		return "https"
	}
	// Quick TLS probe if auto
	targetAddr := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: 300 * time.Millisecond}
	conn, err := tls.DialWithDialer(dialer, "tcp", targetAddr, &tls.Config{InsecureSkipVerify: true})
	if err == nil {
		_ = conn.Close()
		return "https"
	}
	return "http"
}

// serveReverseProxy proxies HTTP, HTTPS, and WebSocket requests to host:port.
func (pm *PreviewManager) serveReverseProxy(w http.ResponseWriter, r *http.Request, host string, port int, scheme string, stripPrefix string) {
	resolvedScheme := pm.resolvePreviewScheme(host, port, scheme)
	targetAddr := net.JoinHostPort(host, strconv.Itoa(port))

	targetURL := &url.URL{
		Scheme: resolvedScheme,
		Host:   targetAddr,
	}

	proxy := &httputil.ReverseProxy{
		Transport: pm.transport,
		Director: func(req *http.Request) {
			req.URL.Scheme = targetURL.Scheme
			req.URL.Host = targetURL.Host
			if stripPrefix != "" {
				trimmed := strings.TrimPrefix(req.URL.Path, stripPrefix)
				if !strings.HasPrefix(trimmed, "/") {
					trimmed = "/" + trimmed
				}
				req.URL.Path = trimmed
				if req.URL.RawPath != "" {
					rawTrimmed := strings.TrimPrefix(req.URL.RawPath, stripPrefix)
					if !strings.HasPrefix(rawTrimmed, "/") {
						rawTrimmed = "/" + rawTrimmed
					}
					req.URL.RawPath = rawTrimmed
				}
			}
			req.Host = targetURL.Host
			// Remove Origin/Referer host mismatches so strict local web apps accept requests
			if orig := req.Header.Get("Origin"); orig != "" {
				req.Header.Set("Origin", fmt.Sprintf("%s://%s", resolvedScheme, targetAddr))
			}
		},
		ModifyResponse: func(resp *http.Response) error {
			// Allow embedding or cross-tab previewing cleanly
			resp.Header.Del("X-Frame-Options")
			// Rewrite root-relative redirect Location headers to stay inside /preview/...
			if loc := resp.Header.Get("Location"); loc != "" && stripPrefix != "" {
				if strings.HasPrefix(loc, "/") && !strings.HasPrefix(loc, "//") && !strings.HasPrefix(loc, stripPrefix) {
					resp.Header.Set("Location", strings.TrimSuffix(stripPrefix, "/")+loc)
				}
			}
			return nil
		},
		ErrorHandler: func(rw http.ResponseWriter, req *http.Request, err error) {
			rw.Header().Set("Content-Type", "text/html; charset=utf-8")
			rw.WriteHeader(http.StatusBadGateway)
			errText := "connection refused"
			if err != nil {
				errText = err.Error()
			}
			_, _ = fmt.Fprintf(rw, `<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <title>Preview Unavailable — %s</title>
  <style>
    body { background: #11111b; color: #cdd6f4; font-family: system-ui, -apple-system, sans-serif; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; }
    .card { background: #1e1e2e; border: 1px solid #313244; border-radius: 12px; padding: 28px 32px; max-width: 480px; box-shadow: 0 20px 40px rgba(0,0,0,0.4); }
    h2 { margin: 0 0 8px 0; font-size: 18px; color: #f38ba8; }
    p { margin: 0 0 16px 0; font-size: 13px; color: #a6adc8; line-height: 1.5; }
    code { background: #181825; padding: 3px 6px; border-radius: 4px; color: #89b4fa; font-family: monospace; }
    button { background: #89b4fa; color: #11111b; border: none; border-radius: 6px; padding: 8px 16px; font-size: 13px; font-weight: 600; cursor: pointer; }
    button:hover { opacity: 0.9; }
  </style>
</head>
<body>
  <div class="card">
    <h2>Could not connect to %s://%s</h2>
    <p>No HTTP/S service responded on <code>%s</code> (%s).</p>
    <p>Make sure your server or application is listening on port <code>%d</code> and try refreshing.</p>
    <button onclick="window.location.reload()">Retry Preview</button>
  </div>
</body>
</html>`, htmlEscape(targetAddr), htmlEscape(resolvedScheme), htmlEscape(targetAddr), htmlEscape(targetAddr), htmlEscape(errText), port)
		},
	}

	proxy.ServeHTTP(w, r)
}

func htmlEscape(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&quot;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// handleAPIPreview handles GET /api/preview and POST /api/preview.
func (server *Server) handleAPIPreview(w http.ResponseWriter, r *http.Request) {
	var user *auth.UserIdentity
	if server.options.EnableProxyAuth || server.options.DefaultUser != "" {
		user = server.authenticateRequest(w, r)
		if user == nil {
			return
		}
	} else if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
		if !checkSameOrigin(r) {
			http.Error(w, "403 Forbidden: cross-origin request rejected", http.StatusForbidden)
			return
		}
		if !server.options.PermitWrite && !server.options.EnableBasicAuth {
			http.Error(w, "403 Forbidden: preview configuration requires --permit-write or authentication", http.StatusForbidden)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		cfg, endpoints := server.previewManager.GetState()
		if user != nil && !user.IsHostAccess {
			var filtered []PreviewEndpoint
			for _, ep := range endpoints {
				if isLoopbackHost(ep.Host) {
					filtered = append(filtered, ep)
				}
			}
			endpoints = filtered
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"active":    cfg,
			"endpoints": endpoints,
		})

	case http.MethodPost:
		var req struct {
			TargetHost string `json:"target_host"`
			TargetPort int    `json:"target_port"`
			Scheme     string `json:"scheme"`
		}
		if r.Body != nil {
			bodyBytes, _ := io.ReadAll(io.LimitReader(r.Body, 64*1024))
			_ = json.Unmarshal(bodyBytes, &req)
		}
		if req.TargetPort == 0 {
			req.TargetPort = 8080
		}
		targetHost := strings.TrimSpace(req.TargetHost)
		if targetHost == "" {
			targetHost = "localhost"
		}
		if !isAllowedPreviewHostForUser(targetHost, user) {
			http.Error(w, "403 Forbidden: target host is not permitted for your privilege level", http.StatusForbidden)
			return
		}
		cfg, err := server.previewManager.SetTarget(targetHost, req.TargetPort, req.Scheme)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "ok",
			"active": cfg,
		})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handlePreviewProxy handles requests to /preview/<host>:<port>/... or /preview/<scheme>/<host>:<port>/...
func (server *Server) handlePreviewProxy(w http.ResponseWriter, r *http.Request, basePrefix string) {
	var user *auth.UserIdentity
	if server.options.EnableProxyAuth || server.options.DefaultUser != "" {
		user = server.authenticateRequest(w, r)
		if user == nil {
			return
		}
	}

	// Parse URL structure: /preview/[http|https/]<host>[:<port>]/...
	rel := strings.TrimPrefix(r.URL.Path, basePrefix)
	rel = strings.TrimPrefix(rel, "/")
	parts := strings.Split(rel, "/")

	scheme := "auto"
	idx := 0
	if len(parts) > idx && (parts[idx] == "http" || parts[idx] == "https" || parts[idx] == "auto") {
		scheme = parts[idx]
		idx++
	}

	var host string
	var port int
	var stripPrefix string

	if len(parts) > idx && parts[idx] != "" {
		hostPortToken := parts[idx]
		h, pStr, err := net.SplitHostPort(hostPortToken)
		if err == nil {
			host = h
			p, err := strconv.Atoi(pStr)
			if err != nil || p < 1 || p > 65535 {
				http.Error(w, "Invalid preview port", http.StatusBadRequest)
				return
			}
			port = p
		} else {
			// If only a port number was given (e.g. /preview/8080/), assume localhost
			if p, err := strconv.Atoi(hostPortToken); err == nil && p >= 1 && p <= 65535 {
				host = "localhost"
				port = p
			} else if isAllowedPreviewHost(hostPortToken) {
				host = hostPortToken
				port = 8080
			}
		}
		if host != "" {
			prefixParts := parts[:idx+1]
			stripPrefix = strings.TrimSuffix(basePrefix, "/") + "/" + strings.Join(prefixParts, "/")
		}
	}

	// If no explicit host:port in URL (e.g. /preview/), use current active target from PreviewManager
	if host == "" {
		cfg, _ := server.previewManager.GetState()
		host = cfg.TargetHost
		port = cfg.TargetPort
		scheme = cfg.Scheme
		stripPrefix = strings.TrimSuffix(basePrefix, "/")
	}

	if !isAllowedPreviewHostForUser(host, user) {
		http.Error(w, "403 Forbidden: preview host is not a permitted local/private address", http.StatusForbidden)
		return
	}

	// Redirect /preview/localhost:8080 -> /preview/localhost:8080/ so relative links resolve properly
	if r.URL.Path == stripPrefix {
		targetPath := stripPrefix + "/"
		if r.URL.RawQuery != "" {
			targetPath += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, targetPath, http.StatusTemporaryRedirect)
		return
	}

	// Set cookie so root-relative subresource requests (/assets/..., /api/...) from the previewed page can be routed
	cookieVal := fmt.Sprintf("%s|%s|%d", scheme, host, port)
	http.SetCookie(w, &http.Cookie{
		Name:     previewTargetCookie,
		Value:    cookieVal,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	log.Printf("preview: %s %s -> %s (%s:%d)", r.Method, r.URL.Path, scheme, host, port)
	server.previewManager.serveReverseProxy(w, r, host, port, scheme, stripPrefix)
}

// tryPreviewFallback checks if an unmatched non-root request came from an active Preview tab
// (via Referer header or gotty_preview_target cookie) and proxies it to the target.
func (server *Server) tryPreviewFallback(w http.ResponseWriter, r *http.Request, pathPrefix string) bool {
	if r.URL.Path == pathPrefix || r.URL.Path == "/" {
		return false
	}

	previewPrefix := pathPrefix + "preview/"

	// 1. Check Referer header for /preview/[scheme/]<host>:<port>/
	if ref := r.Header.Get("Referer"); ref != "" {
		if u, err := url.Parse(ref); err == nil && strings.HasPrefix(u.Path, previewPrefix) {
			rel := strings.TrimPrefix(u.Path, previewPrefix)
			parts := strings.Split(rel, "/")
			scheme := "auto"
			idx := 0
			if len(parts) > idx && (parts[idx] == "http" || parts[idx] == "https" || parts[idx] == "auto") {
				scheme = parts[idx]
				idx++
			}
			if len(parts) > idx && parts[idx] != "" {
				if h, pStr, err := net.SplitHostPort(parts[idx]); err == nil {
					if p, err := strconv.Atoi(pStr); err == nil && isAllowedPreviewHost(h) {
						server.previewManager.serveReverseProxy(w, r, h, p, scheme, "")
						return true
					}
				}
			}
		}
	}

	// 2. Check gotty_preview_target cookie
	if c, err := r.Cookie(previewTargetCookie); err == nil && c.Value != "" {
		fields := strings.SplitN(c.Value, "|", 3)
		if len(fields) == 3 {
			scheme := fields[0]
			host := fields[1]
			if port, err := strconv.Atoi(fields[2]); err == nil && isAllowedPreviewHost(host) {
				server.previewManager.serveReverseProxy(w, r, host, port, scheme, "")
				return true
			}
		}
	}

	return false
}
