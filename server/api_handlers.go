package server

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/sorenisanerd/gotty/pkg/auth"
	"github.com/sorenisanerd/gotty/pkg/userconfig"
)

const MaxUploadSize = 512 << 20 // 512MB max upload size

// IdentityHeaders lists the proxy headers checked for user identity, in order of priority.
var IdentityHeaders = []string{
	"X-Forwarded-User",
	"X-Forwarded-Email",
	"Remote-User",
	"Remote-Email",
	"X-Email",
	"X-User",
	"X-Auth-Request-Email",
	"X-Auth-Request-User",
	"X-Forwarded-Preferred-Username",
}

// resolveRequestUser extracts the identity from proxy headers (when EnableProxyAuth is active)
// or falls back to DefaultUser.
func (server *Server) resolveRequestUser(r *http.Request) (string, string) {
	if server.options.EnableProxyAuth {
		if customHeader := strings.TrimSpace(server.options.ProxyAuthHeader); customHeader != "" {
			if val := strings.TrimSpace(r.Header.Get(customHeader)); val != "" {
				return val, customHeader
			}
		} else {
			for _, h := range IdentityHeaders {
				val := strings.TrimSpace(r.Header.Get(h))
				if val != "" {
					return val, h
				}
			}
		}
	}
	if server.options.DefaultUser != "" {
		return server.options.DefaultUser, "default-user"
	}
	return "", ""
}

// hasProxyIdentity returns true if proxy auth is enabled and an identity header is present in the request.
func (server *Server) hasProxyIdentity(r *http.Request) bool {
	if !server.options.EnableProxyAuth {
		return false
	}
	if customHeader := strings.TrimSpace(server.options.ProxyAuthHeader); customHeader != "" {
		return strings.TrimSpace(r.Header.Get(customHeader)) != ""
	}
	for _, h := range IdentityHeaders {
		if strings.TrimSpace(r.Header.Get(h)) != "" {
			return true
		}
	}
	return false
}

// authenticateRequest resolves the user against the server's passwd/group resolver.
// Writes 403 Forbidden and returns nil if missing, cross-origin, or unmapped.
func (server *Server) authenticateRequest(w http.ResponseWriter, r *http.Request) *auth.UserIdentity {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
		if !checkSameOrigin(r) {
			http.Error(w, "403 Forbidden: cross-origin request rejected", http.StatusForbidden)
			return nil
		}
	}

	identityVal, source := server.resolveRequestUser(r)
	if identityVal == "" {
		log.Printf("auth: missing identity (checked %v, default-user=%q)", IdentityHeaders, server.options.DefaultUser)
		http.Error(w, "403 Forbidden: unmapped or missing identity header (e.g. X-Forwarded-User, Remote-User, X-Forwarded-Email)", http.StatusForbidden)
		return nil
	}

	ident, err := server.authResolver.Resolve(identityVal)
	if err != nil {
		log.Printf("auth: unmapped identity %q from %s: %v", identityVal, source, err)
		http.Error(w, "403 Forbidden: unmapped user "+identityVal, http.StatusForbidden)
		return nil
	}
	return ident
}

// checkSameOrigin verifies that if an Origin header is present on state-changing requests,
// its host matches the request's Host or X-Forwarded-Host header.
func checkSameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	reqHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if reqHost == "" {
		reqHost = r.Host
	}
	if idx := strings.IndexByte(reqHost, ','); idx != -1 {
		reqHost = strings.TrimSpace(reqHost[:idx])
	}
	return strings.EqualFold(u.Host, reqHost)
}

// handleAPISessions handles GET /api/sessions and POST /api/sessions.
func (server *Server) handleAPISessions(w http.ResponseWriter, r *http.Request) {
	user := server.authenticateRequest(w, r)
	if user == nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		sessions := server.sessionManager.ListSessions(user.Username)
		if sessions == nil {
			sessions = []SessionInfo{}
		}
		_ = json.NewEncoder(w).Encode(sessions)

	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
			Mode string `json:"mode"`
		}
		if r.Body != nil {
			_ = json.NewDecoder(r.Body).Decode(&req)
		}

		cfg, err := userconfig.Load(user.HomeDir)
		if err != nil {
			cfg = userconfig.Default()
		}

		mode := req.Mode
		if mode == "" {
			mode = cfg.Session.DefaultMode
		}
		if mode == "" {
			mode = server.options.DefaultMode
		}

		sess, err := server.sessionManager.CreateSession(user, req.Name, mode, cfg.ReapDuration())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		resp := map[string]interface{}{
			"session_id":     sess.ID,
			"id":             sess.ID,
			"name":           sess.Name,
			"username":       sess.Username,
			"mode":           sess.Mode,
			"is_host_access": sess.IsHostAccess,
			"created_at":     sess.CreatedAt,
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPISessionByID handles DELETE /api/sessions/:id, PATCH /api/sessions/:id, and POST /api/sessions/:id/upload.
func (server *Server) handleAPISessionByID(w http.ResponseWriter, r *http.Request, basePrefix string) {
	user := server.authenticateRequest(w, r)
	if user == nil {
		return
	}

	// Extract ":id" and optional "/upload" suffix from URL path
	rel := strings.TrimPrefix(r.URL.Path, basePrefix)
	rel = strings.TrimPrefix(rel, "/")
	parts := strings.Split(rel, "/")
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Missing session ID", http.StatusBadRequest)
		return
	}
	sessionID := SessionID(parts[0])

	if len(parts) >= 2 && parts[1] == "upload" {
		server.handleAPISessionUpload(w, r, user, sessionID)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodDelete:
		if err := server.sessionManager.DeleteSession(sessionID, user.Username); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "terminated"})

	case http.MethodPatch, http.MethodPut:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Name) == "" {
			http.Error(w, "Invalid session name", http.StatusBadRequest)
			return
		}
		if err := server.sessionManager.RenameSession(sessionID, user.Username, strings.TrimSpace(req.Name)); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "name": strings.TrimSpace(req.Name)})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAPISessionUpload handles POST /api/sessions/:id/upload.
// Resolves CWD via /proc/<pid>/cwd and falls back to user.HomeDir on failure.
func (server *Server) handleAPISessionUpload(w http.ResponseWriter, r *http.Request, user *auth.UserIdentity, sessionID SessionID) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sess, err := server.sessionManager.GetSession(sessionID, user.Username)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, MaxUploadSize)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "Failed to parse multipart upload: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Missing 'file' field in multipart upload", http.StatusBadRequest)
		return
	}
	defer file.Close()

	filename := filepath.Base(header.Filename)
	if filename == "" || filename == "." || filename == "/" {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	// 1. Try session's active CWD via /proc/<pid>/cwd (validated against symlink traversal)
	fallbackDir := user.HomeDir
	if fallbackDir == "" {
		fallbackDir = filepath.Join("/home", user.Username)
	}

	var writtenPath string
	if cwd, err := sess.ResolveCWD(); err == nil && cwd != "" {
		if isSafeUploadDir(cwd, fallbackDir, sess.IsHostAccess) {
			if candidate, err := writeUploadedFileInDir(cwd, filename, file, user); err == nil {
				writtenPath = candidate
			} else {
				_, _ = file.Seek(0, io.SeekStart)
			}
		}
	}

	// 2. Fallback to user's HomeDir (/home/<username>/)
	if writtenPath == "" {
		_, statErr := os.Stat(fallbackDir)
		if err := os.MkdirAll(fallbackDir, 0755); err == nil && os.IsNotExist(statErr) {
			if user != nil && os.Geteuid() == 0 && user.UID > 0 {
				_ = os.Chown(fallbackDir, int(user.UID), int(user.GID))
			}
		}
		candidate, err := writeUploadedFileInDir(fallbackDir, filename, file, user)
		if err != nil {
			http.Error(w, "Failed to write file to CWD or HOME: "+err.Error(), http.StatusInternalServerError)
			return
		}
		writtenPath = candidate
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"path":   writtenPath,
	})
}

func isSafeUploadDir(cwd, homeDir string, isHostAccess bool) bool {
	realCWD, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return false
	}
	if isHostAccess {
		return true
	}
	realHome, err := filepath.EvalSymlinks(homeDir)
	if err != nil {
		realHome = filepath.Clean(homeDir)
	}
	rel, err := filepath.Rel(realHome, realCWD)
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

// writeUploadedFileInDir writes filename inside targetDir using os.OpenRoot to prevent
// symlink and TOCTOU directory traversal attacks.
func writeUploadedFileInDir(targetDir, filename string, src io.Reader, user *auth.UserIdentity) (string, error) {
	root, err := os.OpenRoot(targetDir)
	if err != nil {
		return "", err
	}
	defer root.Close()

	out, err := root.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|syscall.O_NOFOLLOW, 0644)
	if err != nil {
		return "", err
	}
	defer out.Close()

	if _, err := io.Copy(out, src); err != nil {
		return "", err
	}
	if user != nil && os.Geteuid() == 0 && user.UID > 0 {
		_ = out.Chown(int(user.UID), int(user.GID))
	}
	return filepath.Join(targetDir, filename), nil
}

// handleAPIUserConfig handles GET /api/user/config and PUT /api/user/config.
func (server *Server) handleAPIUserConfig(w http.ResponseWriter, r *http.Request) {
	user := server.authenticateRequest(w, r)
	if user == nil {
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		cfg, err := userconfig.Load(user.HomeDir)
		if err != nil {
			cfg = userconfig.Default()
		}
		_ = json.NewEncoder(w).Encode(cfg)

	case http.MethodPut:
		cfg := userconfig.Default()
		if err := json.NewDecoder(r.Body).Decode(cfg); err != nil {
			http.Error(w, "Invalid JSON config payload: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := userconfig.Save(user.HomeDir, cfg); err != nil {
			http.Error(w, "Failed to save user config: "+err.Error(), http.StatusInternalServerError)
			return
		}
		server.sessionManager.UpdateUserReapDuration(user.Username, cfg.ReapDuration())
		_ = json.NewEncoder(w).Encode(cfg)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
