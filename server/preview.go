package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
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

// PreviewManager manages the Web Preview reverse proxy with per-user target state
// and container network-namespace isolation for sandboxed users.
type PreviewManager struct {
	mu          sync.RWMutex
	current     PreviewTargetConfig
	userTargets map[string]PreviewTargetConfig
	endpoints   []PreviewEndpoint
	transport   *http.Transport
}

// NewPreviewManager creates a new PreviewManager initialized to forward to localhost:3000
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

	defaultCfg := PreviewTargetConfig{
		TargetHost:      "localhost",
		TargetPort:      3000,
		Scheme:          "auto",
		ForwarderPort:   3000,
		ForwarderActive: false,
		PreviewURL:      "./preview/localhost:3000/",
	}

	pm := &PreviewManager{
		current:     defaultCfg,
		userTargets: make(map[string]PreviewTargetConfig),
		endpoints:   discoverPreviewEndpoints(),
		transport:   tr,
	}
	return pm
}

// Close cleans up idle connections on the preview transport.
func (pm *PreviewManager) Close() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if pm.transport != nil {
		pm.transport.CloseIdleConnections()
	}
	pm.current.ForwarderActive = false
}

// SetTarget updates the default preview target configuration.
func (pm *PreviewManager) SetTarget(host string, port int, scheme string) (PreviewTargetConfig, error) {
	return pm.SetTargetForUser("", host, port, scheme)
}

// SetTargetForUser updates the active preview target for a specific authenticated username.
func (pm *PreviewManager) SetTargetForUser(username, host string, port int, scheme string) (PreviewTargetConfig, error) {
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

	cfg := PreviewTargetConfig{
		TargetHost:      host,
		TargetPort:      port,
		Scheme:          scheme,
		ForwarderPort:   port,
		ForwarderActive: false,
		PreviewURL:      previewURL,
	}

	pm.current = cfg
	if username != "" {
		pm.userTargets[username] = cfg
	}

	return cfg, nil
}

// GetState returns the current target config and all introspected local endpoints.
func (pm *PreviewManager) GetState() (PreviewTargetConfig, []PreviewEndpoint) {
	return pm.GetStateForUser("")
}

// GetStateForUser returns the active target config for username and all introspected local endpoints.
func (pm *PreviewManager) GetStateForUser(username string) (PreviewTargetConfig, []PreviewEndpoint) {
	pm.mu.RLock()
	cfg := pm.current
	if username != "" {
		if uCfg, ok := pm.userTargets[username]; ok {
			cfg = uCfg
		}
	}
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
func (pm *PreviewManager) resolvePreviewScheme(host string, port int, requestedScheme string, containerPID int) string {
	if requestedScheme == "http" || requestedScheme == "https" {
		return requestedScheme
	}
	if port == 443 || port == 8443 {
		return "https"
	}
	// Quick TLS probe if auto
	targetAddr := net.JoinHostPort(host, strconv.Itoa(port))
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	var rawConn net.Conn
	var err error
	if containerPID > 0 {
		rawConn, err = dialInContainerNetNS(ctx, containerPID, "tcp", targetAddr)
	} else {
		dialer := &net.Dialer{Timeout: 300 * time.Millisecond}
		rawConn, err = dialer.DialContext(ctx, "tcp", targetAddr)
	}
	if err != nil {
		return "http"
	}
	tlsConn := tls.Client(rawConn, &tls.Config{InsecureSkipVerify: true})
	_ = tlsConn.SetDeadline(time.Now().Add(300 * time.Millisecond))
	if err := tlsConn.Handshake(); err == nil {
		_ = tlsConn.Close()
		return "https"
	}
	_ = rawConn.Close()
	return "http"
}

func sysSetnsNum() uintptr {
	switch runtime.GOARCH {
	case "amd64":
		return 308
	case "arm64", "riscv64":
		return 268
	case "386":
		return 346
	case "arm":
		return 375
	default:
		return 308
	}
}

// dialInContainerNetNS opens a TCP socket to addr inside /proc/<containerPID>/ns/net
// so sandboxed users preview services bound to 127.0.0.1 inside their container rather than the host.
func dialInContainerNetNS(ctx context.Context, containerPID int, network, addr string) (net.Conn, error) {
	if runtime.GOOS != "linux" || containerPID <= 0 {
		return nil, fmt.Errorf("container network namespace unavailable (pid=%d)", containerPID)
	}

	type dialResult struct {
		conn net.Conn
		err  error
	}
	ch := make(chan dialResult, 1)

	go func() {
		runtime.LockOSThread()
		tid := syscall.Gettid()
		origNSPath := fmt.Sprintf("/proc/self/task/%d/ns/net", tid)
		origNS, err := os.Open(origNSPath)
		if err != nil {
			runtime.UnlockOSThread()
			ch <- dialResult{nil, fmt.Errorf("failed to open host netns: %w", err)}
			return
		}
		defer origNS.Close()

		targetNSPath := fmt.Sprintf("/proc/%d/ns/net", containerPID)
		targetNS, err := os.Open(targetNSPath)
		if err != nil {
			runtime.UnlockOSThread()
			ch <- dialResult{nil, fmt.Errorf("failed to open container netns %s: %w", targetNSPath, err)}
			return
		}
		defer targetNS.Close()

		const cloneNewNet = 0x40000000 // syscall.CLONE_NEWNET
		if _, _, errno := syscall.Syscall(sysSetnsNum(), targetNS.Fd(), cloneNewNet, 0); errno != 0 {
			runtime.UnlockOSThread()
			ch <- dialResult{nil, fmt.Errorf("setns to container %d failed: %v", containerPID, errno)}
			return
		}

		dialer := &net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		conn, dialErr := dialer.DialContext(ctx, network, addr)

		// Restore host netns before unlocking OS thread; if restore fails, do NOT unlock so the thread terminates.
		if _, _, errno := syscall.Syscall(sysSetnsNum(), origNS.Fd(), cloneNewNet, 0); errno != 0 {
			if conn != nil {
				_ = conn.Close()
			}
			ch <- dialResult{nil, fmt.Errorf("failed to restore host netns: %v", errno)}
			return
		}
		runtime.UnlockOSThread()
		ch <- dialResult{conn, dialErr}
	}()

	res := <-ch
	return res.conn, res.err
}

// serveReverseProxy proxies HTTP, HTTPS, and WebSocket requests to host:port.
// When containerPID > 0, connections are dialed inside the container's network namespace.
func (pm *PreviewManager) serveReverseProxy(w http.ResponseWriter, r *http.Request, host string, port int, scheme string, stripPrefix string, containerPID int) {
	resolvedScheme := pm.resolvePreviewScheme(host, port, scheme, containerPID)
	targetAddr := net.JoinHostPort(host, strconv.Itoa(port))

	targetURL := &url.URL{
		Scheme: resolvedScheme,
		Host:   targetAddr,
	}

	transport := pm.transport
	if containerPID > 0 {
		transport = &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialInContainerNetNS(ctx, containerPID, network, addr)
			},
			TLSClientConfig:       &tls.Config{InsecureSkipVerify: true},
			MaxIdleConns:          16,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		}
	}

	proxy := &httputil.ReverseProxy{
		Transport: transport,
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
			// Ensure browser sends Referer on sub-requests so GoTTY can route preview assets
			// and block preview scripts from invoking GoTTY control plane endpoints.
			resp.Header.Set("Referrer-Policy", "same-origin")
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

	var username string
	if user != nil {
		username = user.Username
	}

	switch r.Method {
	case http.MethodGet:
		cfg, endpoints := server.previewManager.GetStateForUser(username)
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
			req.TargetPort = 3000
		}
		targetHost := strings.TrimSpace(req.TargetHost)
		if targetHost == "" {
			targetHost = "localhost"
		}
		if !isAllowedPreviewHostForUser(targetHost, user) {
			http.Error(w, "403 Forbidden: target host is not permitted for your privilege level", http.StatusForbidden)
			return
		}
		cfg, err := server.previewManager.SetTargetForUser(username, targetHost, req.TargetPort, req.Scheme)
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

// resolvePreviewContainerPID returns the container PID to dial inside when previewing a target.
// For sandboxed users (!user.IsHostAccess), host loopback is never exposed when container runtime
// or a container session is present.
func (server *Server) resolvePreviewContainerPID(user *auth.UserIdentity, host string) (int, error) {
	if user == nil || server.sessionManager == nil {
		return 0, nil
	}
	pid, hasContainer := server.sessionManager.ResolveSandboxContainerPID(user.Username)
	if !user.IsHostAccess {
		if pid > 0 {
			return pid, nil
		}
		if hasContainer || server.options.ContainerRuntime != "" {
			return 0, fmt.Errorf("no active sandbox container found for user %s", user.Username)
		}
		return 0, nil
	}
	// Host-authorized user: if previewing localhost and they have an active sandbox container,
	// prefer host loopback if listening, otherwise fall back to their sandbox container's netns.
	if pid > 0 && isLoopbackHost(host) {
		return 0, nil
	}
	return 0, nil
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

	var username string
	if user != nil {
		username = user.Username
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
			// If only a port number was given (e.g. /preview/3000/), assume localhost
			if p, err := strconv.Atoi(hostPortToken); err == nil && p >= 1 && p <= 65535 {
				host = "localhost"
				port = p
			} else if isAllowedPreviewHost(hostPortToken) {
				host = hostPortToken
				port = 3000
			}
		}
		if host != "" {
			prefixParts := parts[:idx+1]
			stripPrefix = strings.TrimSuffix(basePrefix, "/") + "/" + strings.Join(prefixParts, "/")
		}
	}

	// If no explicit host:port in URL (e.g. /preview/), use current active target from PreviewManager
	if host == "" {
		cfg, _ := server.previewManager.GetStateForUser(username)
		host = cfg.TargetHost
		port = cfg.TargetPort
		scheme = cfg.Scheme
		stripPrefix = strings.TrimSuffix(basePrefix, "/")
	}

	if !isAllowedPreviewHostForUser(host, user) {
		http.Error(w, "403 Forbidden: preview host is not a permitted local/private address", http.StatusForbidden)
		return
	}

	containerPID, err := server.resolvePreviewContainerPID(user, host)
	if err != nil {
		http.Error(w, "502 Bad Gateway: "+err.Error(), http.StatusBadGateway)
		return
	}

	// Redirect /preview/localhost:3000 -> /preview/localhost:3000/ so relative links resolve properly
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

	log.Printf("preview: %s %s -> %s (%s:%d, netns_pid=%d)", r.Method, r.URL.Path, scheme, host, port, containerPID)
	server.previewManager.serveReverseProxy(w, r, host, port, scheme, stripPrefix, containerPID)
}

// tryPreviewFallback checks if an unmatched non-root request came from an active Preview tab
// (via Referer header or gotty_preview_target cookie) and proxies it to the target while enforcing
// the user's privilege and network-namespace boundaries.
func (server *Server) tryPreviewFallback(w http.ResponseWriter, r *http.Request, pathPrefix string, user *auth.UserIdentity) bool {
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
					if p, err := strconv.Atoi(pStr); err == nil && p >= 1 && p <= 65535 {
						if !isAllowedPreviewHostForUser(h, user) {
							http.Error(w, "403 Forbidden: preview host is not permitted for your privilege level", http.StatusForbidden)
							return true
						}
						containerPID, err := server.resolvePreviewContainerPID(user, h)
						if err != nil {
							http.Error(w, "502 Bad Gateway: "+err.Error(), http.StatusBadGateway)
							return true
						}
						server.previewManager.serveReverseProxy(w, r, h, p, scheme, "", containerPID)
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
			if port, err := strconv.Atoi(fields[2]); err == nil && port >= 1 && port <= 65535 {
				if !isAllowedPreviewHostForUser(host, user) {
					http.Error(w, "403 Forbidden: preview host is not permitted for your privilege level", http.StatusForbidden)
					return true
				}
				containerPID, err := server.resolvePreviewContainerPID(user, host)
				if err != nil {
					http.Error(w, "502 Bad Gateway: "+err.Error(), http.StatusBadGateway)
					return true
				}
				server.previewManager.serveReverseProxy(w, r, host, port, scheme, "", containerPID)
				return true
			}
		}
	}

	return false
}
