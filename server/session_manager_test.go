package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sorenisanerd/gotty/pkg/auth"
	"github.com/sorenisanerd/gotty/pkg/userconfig"
)

type pipeSlave struct {
	pr     *io.PipeReader
	pw     *io.PipeWriter
	inBuf  bytes.Buffer
	cwd    string
	closed bool
}

func newPipeSlave(cwd string) *pipeSlave {
	pr, pw := io.Pipe()
	return &pipeSlave{pr: pr, pw: pw, cwd: cwd}
}

func (p *pipeSlave) Read(b []byte) (int, error)                   { return p.pr.Read(b) }
func (p *pipeSlave) Write(b []byte) (int, error)                  { return p.inBuf.Write(b) }
func (p *pipeSlave) Close() error                                 { p.closed = true; _ = p.pw.Close(); return p.pr.Close() }
func (p *pipeSlave) WindowTitleVariables() map[string]interface{} { return nil }
func (p *pipeSlave) ResizeTerminal(w, h int) error                { return nil }
func (p *pipeSlave) ResolveCWD() (string, error) {
	if p.cwd == "" {
		return "", io.EOF
	}
	return p.cwd, nil
}

func TestCircularBuffer_WrapAround(t *testing.T) {
	cb := NewCircularBuffer(8)
	_, _ = cb.Write([]byte("12345"))
	if got := string(cb.Bytes()); got != "12345" {
		t.Fatalf("got %q, want 12345", got)
	}

	_, _ = cb.Write([]byte("67890"))
	if got := string(cb.Bytes()); got != "34567890" {
		t.Fatalf("got %q, want 34567890", got)
	}
}

func TestSessionManager_ReplayAndMultiAttach(t *testing.T) {
	ps := newPipeSlave("")
	defer ps.Close()

	sm := NewSessionManager(func(u *auth.UserIdentity, id SessionID, mode string) (Slave, error) {
		return ps, nil
	})

	user := &auth.UserIdentity{Username: "devuser", UID: 1000, GID: 1000}
	sess, err := sm.CreateSession(user, "Main Shell", "host", 0)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Write output while detached (stored in RingBuffer)
	_, _ = ps.pw.Write([]byte("boot_banner\n"))
	time.Sleep(30 * time.Millisecond)

	// Attach Client 1 and Client 2 concurrently
	c1, err := sm.Attach(sess.ID, "devuser")
	if err != nil {
		t.Fatalf("Attach c1 failed: %v", err)
	}
	defer c1.Close()

	c2, err := sm.Attach(sess.ID, "devuser")
	if err != nil {
		t.Fatalf("Attach c2 failed: %v", err)
	}
	defer c2.Close()

	// Both clients should immediately read the replayed RingBuffer
	buf1 := make([]byte, 64)
	n1, err := c1.Read(buf1)
	if err != nil || string(buf1[:n1]) != "boot_banner\n" {
		t.Fatalf("c1 replay got %q (err=%v), want boot_banner\\n", string(buf1[:n1]), err)
	}

	buf2 := make([]byte, 64)
	n2, err := c2.Read(buf2)
	if err != nil || string(buf2[:n2]) != "boot_banner\n" {
		t.Fatalf("c2 replay got %q (err=%v), want boot_banner\\n", string(buf2[:n2]), err)
	}

	// Write live output from PTY and verify both clients receive synchronized broadcast
	_, _ = ps.pw.Write([]byte("live_line\n"))

	n1, err = c1.Read(buf1)
	if err != nil || string(buf1[:n1]) != "live_line\n" {
		t.Fatalf("c1 live read got %q (err=%v), want live_line\\n", string(buf1[:n1]), err)
	}

	n2, err = c2.Read(buf2)
	if err != nil || string(buf2[:n2]) != "live_line\n" {
		t.Fatalf("c2 live read got %q (err=%v), want live_line\\n", string(buf2[:n2]), err)
	}
}

func TestSessionManager_ReapTimer(t *testing.T) {
	ps := newPipeSlave("")
	sm := NewSessionManager(func(u *auth.UserIdentity, id SessionID, mode string) (Slave, error) {
		return ps, nil
	})

	user := &auth.UserIdentity{Username: "devuser", UID: 1000, GID: 1000}
	sess, err := sm.CreateSession(user, "Temp Session", "host", 80*time.Millisecond)
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	// Attach and then detach to trigger reap timer
	c, _ := sm.Attach(sess.ID, "devuser")
	_ = c.Close()

	time.Sleep(150 * time.Millisecond)
	if _, err := sm.GetSession(sess.ID, "devuser"); err == nil {
		t.Fatalf("expected session to be reaped after timeout")
	}
}

func TestBuildContainerArgs_Sandboxed(t *testing.T) {
	guest := &auth.UserIdentity{
		Username:     "guest",
		UID:          1002,
		GID:          1002,
		HomeDir:      "/home/guest",
		Shell:        "/bin/sh",
		IsHostAccess: false,
	}
	guestArgs := strings.Join(BuildContainerArgs(guest, "gotty-guest-1", "gotty-sandbox:latest", ""), " ")
	for _, flag := range []string{
		"--security-opt label=disable",
		"--pids-limit=512",
		"--cap-drop=MKNOD",
		"--cap-drop=SYS_ADMIN",
		"--cap-drop=NET_ADMIN",
		"-v /home/guest:/home/guest",
	} {
		if !strings.Contains(guestArgs, flag) {
			t.Errorf("sandboxed container args missing %q: %s", flag, guestArgs)
		}
	}
	if strings.Contains(guestArgs, "--privileged") {
		t.Errorf("sandboxed container must not contain --privileged: %s", guestArgs)
	}
}

func TestRESTAPI_AuthSessionsUploadAndConfig(t *testing.T) {
	tmp := t.TempDir()
	homeDir := filepath.Join(tmp, "home", "devuser")
	cwdDir := filepath.Join(homeDir, "projects")
	_ = os.MkdirAll(cwdDir, 0755)

	passwdFile := filepath.Join(tmp, "passwd")
	groupFile := filepath.Join(tmp, "group")
	_ = os.WriteFile(passwdFile, []byte("devuser:x:1000:1000:Dev User,devuser@example.com,,,:"+homeDir+":/bin/bash\n"), 0644)
	_ = os.WriteFile(groupFile, []byte("wheel:x:10:devuser\n"), 0644)

	opts := &Options{
		Address:         "127.0.0.1",
		Port:            "0",
		Path:            "/",
		Quiet:           true,
		EnableProxyAuth: true,
		PasswdFile:      passwdFile,
		GroupFile:       groupFile,
	}

	srv, err := New(&mockFactory{}, opts)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	var activeSlave *pipeSlave
	srv.sessionManager = NewSessionManager(func(u *auth.UserIdentity, id SessionID, mode string) (Slave, error) {
		activeSlave = newPipeSlave(cwdDir)
		return activeSlave, nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handler := srv.setupHandlers(ctx, cancel, "/", newCounter(0))
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// 1. Unmapped user -> 403 Forbidden
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/sessions", nil)
	req.Header.Set("X-Forwarded-User", "unknown@example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for unmapped user, got %d", resp.StatusCode)
	}

	// 2. Create session via POST /api/sessions with GECOS email via Remote-User header
	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/sessions", strings.NewReader(`{"name":"Router CLI"}`))
	req.Header.Set("Remote-User", "devuser@example.com")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created via Remote-User, got %d", resp.StatusCode)
	}
	var createResp struct {
		SessionID    string `json:"session_id"`
		Name         string `json:"name"`
		IsHostAccess bool   `json:"is_host_access"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&createResp)
	resp.Body.Close()

	if createResp.SessionID == "" || !createResp.IsHostAccess {
		t.Fatalf("unexpected create response: %+v", createResp)
	}

	// 2b. Verify default-user fallback when headers are missing
	srv.options.DefaultUser = "devuser"
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/sessions", nil)
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK via DefaultUser fallback, got %d (err=%v)", resp.StatusCode, err)
	}
	resp.Body.Close()
	srv.options.DefaultUser = ""

	// 3. Upload file to active $PWD via POST /api/sessions/:id/upload
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "router.conf")
	_, _ = fw.Write([]byte("hostname=router"))
	_ = mw.Close()

	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/sessions/"+createResp.SessionID+"/upload", &body)
	req.Header.Set("X-Forwarded-User", "devuser@example.com")
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload failed with status %d", resp.StatusCode)
	}
	var uploadResp struct {
		Status string `json:"status"`
		Path   string `json:"path"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&uploadResp)
	resp.Body.Close()

	expectedPath := filepath.Join(cwdDir, "router.conf")
	if uploadResp.Path != expectedPath {
		t.Fatalf("upload path got %q, want %q", uploadResp.Path, expectedPath)
	}

	// 4. Fallback to $HOME when CWD resolution fails
	activeSlave.cwd = "/nonexistent/unwritable/dir"
	body.Reset()
	mw = multipart.NewWriter(&body)
	fw, _ = mw.CreateFormFile("file", "fallback.txt")
	_, _ = fw.Write([]byte("fallback data"))
	_ = mw.Close()

	req, _ = http.NewRequest(http.MethodPost, ts.URL+"/api/sessions/"+createResp.SessionID+"/upload", &body)
	req.Header.Set("X-Forwarded-User", "devuser@example.com")
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, _ = http.DefaultClient.Do(req)
	_ = json.NewDecoder(resp.Body).Decode(&uploadResp)
	resp.Body.Close()

	expectedFallback := filepath.Join(homeDir, "fallback.txt")
	if uploadResp.Path != expectedFallback {
		t.Fatalf("fallback upload path got %q, want %q", uploadResp.Path, expectedFallback)
	}

	// 5. PUT and GET /api/user/config
	cfg := userconfig.Default()
	cfg.Terminal.Theme = "nord"
	cfg.Session.ReapTimeout = "8h"
	cfgBytes, _ := json.Marshal(cfg)

	req, _ = http.NewRequest(http.MethodPut, ts.URL+"/api/user/config", bytes.NewReader(cfgBytes))
	req.Header.Set("X-Forwarded-User", "devuser@example.com")
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/user/config failed with %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify ~/.gotty/config.yaml was written on disk
	diskCfg, err := userconfig.Load(homeDir)
	if err != nil || diskCfg.Terminal.Theme != "nord" || diskCfg.Session.ReapTimeout != "8h" {
		t.Fatalf("unexpected disk config: %+v (err=%v)", diskCfg, err)
	}

	// 6. Test Web Preview API (GET /api/preview, POST /api/preview) and Reverse Proxy (/preview/...)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("preview-ok:" + r.URL.Path))
	}))
	defer upstream.Close()
	_, upstreamPortStr, _ := strings.Cut(strings.TrimPrefix(upstream.URL, "http://"), ":")

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/api/preview", nil)
	req.Header.Set("X-Forwarded-User", "devuser@example.com")
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/preview failed: %v", err)
	}
	resp.Body.Close()

	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/preview/127.0.0.1:"+upstreamPortStr+"/hello", nil)
	req.Header.Set("X-Forwarded-User", "devuser@example.com")
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /preview/127.0.0.1:%s/hello failed: status=%d err=%v", upstreamPortStr, resp.StatusCode, err)
	}
	previewBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(previewBody) != "preview-ok:/hello" {
		t.Fatalf("unexpected preview response %q, want preview-ok:/hello", string(previewBody))
	}

	// Verify SSRF protection rejects public internet IPs
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/preview/8.8.8.8:80/", nil)
	req.Header.Set("X-Forwarded-User", "devuser@example.com")
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for external IP 8.8.8.8, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestSecurity_ProxyAuthDisabledAndSandboxPreviewScope(t *testing.T) {
	tmp := t.TempDir()
	guestHome := filepath.Join(tmp, "home", "guest")
	_ = os.MkdirAll(guestHome, 0755)

	passwdFile := filepath.Join(tmp, "passwd")
	groupFile := filepath.Join(tmp, "group")
	_ = os.WriteFile(passwdFile, []byte(
		"devuser:x:1000:1000:Dev User,devuser@example.com,,,:/home/devuser:/bin/bash\n"+
			"guest:x:1002:1002:Guest User,guest@example.com,,,:"+guestHome+":/bin/bash\n",
	), 0644)
	_ = os.WriteFile(groupFile, []byte("custom-admins:x:10:devuser\n"), 0644)

	// 1. When EnableProxyAuth is false, spoofed X-Forwarded-User headers must NOT be trusted
	standaloneSrv, err := New(&mockFactory{}, &Options{
		Address:         "127.0.0.1",
		Port:            "0",
		Path:            "/",
		Quiet:           true,
		EnableProxyAuth: false,
		PasswdFile:      passwdFile,
		GroupFile:       groupFile,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	standaloneTS := httptest.NewServer(standaloneSrv.setupHandlers(ctx, cancel, "/", newCounter(0)))
	defer standaloneTS.Close()

	req, _ := http.NewRequest(http.MethodPost, standaloneTS.URL+"/api/sessions", strings.NewReader(`{"name":"Spoofed"}`))
	req.Header.Set("X-Forwarded-User", "devuser@example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when spoofing X-Forwarded-User with EnableProxyAuth=false, got %d", resp.StatusCode)
	}

	// Read-only standalone server must also reject POST /api/preview
	req, _ = http.NewRequest(http.MethodPost, standaloneTS.URL+"/api/preview", strings.NewReader(`{"target_host":"localhost","target_port":3000}`))
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for POST /api/preview on read-only standalone server, got %d", resp.StatusCode)
	}

	// 2. Test ProxyAuthHeader & HostGroups customization + Sandboxed user preview restriction
	proxySrv, err := New(&mockFactory{}, &Options{
		Address:         "127.0.0.1",
		Port:            "0",
		Path:            "/",
		Quiet:           true,
		EnableProxyAuth: true,
		ProxyAuthHeader: "X-Custom-Auth-User",
		HostGroups:      "custom-admins",
		PasswdFile:      passwdFile,
		GroupFile:       groupFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	proxyTS := httptest.NewServer(proxySrv.setupHandlers(ctx, cancel, "/", newCounter(0)))
	defer proxyTS.Close()

	// Standard X-Forwarded-User should be ignored when ProxyAuthHeader="X-Custom-Auth-User"
	req, _ = http.NewRequest(http.MethodGet, proxyTS.URL+"/api/sessions", nil)
	req.Header.Set("X-Forwarded-User", "devuser@example.com")
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 when using non-configured proxy header, got %d", resp.StatusCode)
	}

	// Sandboxed user (guest) can only see localhost in GET /api/preview and cannot proxy to non-loopback LAN IPs
	req, _ = http.NewRequest(http.MethodGet, proxyTS.URL+"/api/preview", nil)
	req.Header.Set("X-Custom-Auth-User", "guest@example.com")
	resp, err = http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/preview for guest failed: %v", err)
	}
	var previewState struct {
		Endpoints []PreviewEndpoint `json:"endpoints"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&previewState)
	resp.Body.Close()
	for _, ep := range previewState.Endpoints {
		if !isLoopbackHost(ep.Host) {
			t.Fatalf("sandboxed user received non-loopback endpoint %+v", ep)
		}
	}

	// Sandboxed user attempting to preview a private LAN IP (e.g. 192.168.1.1) must get 403 Forbidden
	req, _ = http.NewRequest(http.MethodPost, proxyTS.URL+"/api/preview", strings.NewReader(`{"target_host":"192.168.1.1","target_port":80}`))
	req.Header.Set("X-Custom-Auth-User", "guest@example.com")
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for sandboxed user targeting LAN IP, got %d", resp.StatusCode)
	}

	// Dynamic interface introspection must not contain hardcoded unconfigured IPs
	pm := NewPreviewManager()
	defer pm.Close()
	_, eps := pm.GetState()
	if len(eps) == 0 || eps[0].Host != "localhost" {
		t.Fatalf("expected localhost as first introspected endpoint, got %+v", eps)
	}

	// Sandboxed user attempting to bypass LAN IP restriction via tryPreviewFallback cookie or Referer must get 403
	req, _ = http.NewRequest(http.MethodGet, proxyTS.URL+"/subresource.js", nil)
	req.Header.Set("X-Custom-Auth-User", "guest@example.com")
	req.AddCookie(&http.Cookie{Name: previewTargetCookie, Value: "http|192.168.1.1|80"})
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for sandboxed user cookie fallback to LAN IP, got %d", resp.StatusCode)
	}

	// Previewed web app attempting to access GoTTY control plane (/api/sessions) via Referer must be blocked
	req, _ = http.NewRequest(http.MethodGet, proxyTS.URL+"/api/sessions", nil)
	req.Header.Set("X-Custom-Auth-User", "devuser@example.com")
	req.Header.Set("Referer", proxyTS.URL+"/preview/localhost:3000/index.html")
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden when accessing /api/sessions from /preview/ Referer, got %d", resp.StatusCode)
	}
}

func TestOSCRingStripper_AndConflictingHeaders(t *testing.T) {
	var stripper oscRingStripper
	// Feed an OSC 7777 sequence split across multiple chunks surrounded by normal text
	c1 := stripper.Filter([]byte("before\n\x1b]77"))
	c2 := stripper.Filter([]byte("77;download;filename=dGVzdA==;data=AAAA"))
	c3 := stripper.Filter([]byte("BBBB\x07after\n\x1b]52;c;aGVsbG8=\x07end"))
	combined := string(c1) + string(c2) + string(c3)
	if combined != "before\nafter\nend" {
		t.Fatalf("unexpected OSC stripped output %q, want %q", combined, "before\nafter\nend")
	}

	// Verify conflicting identity headers are rejected when ProxyAuthHeader is unset
	tmp := t.TempDir()
	passwdFile := filepath.Join(tmp, "passwd")
	groupFile := filepath.Join(tmp, "group")
	_ = os.WriteFile(passwdFile, []byte("devuser:x:1000:1000:Dev:/home/devuser:/bin/bash\nguest:x:1002:1002:Guest:/home/guest:/bin/bash\n"), 0644)
	_ = os.WriteFile(groupFile, []byte("wheel:x:10:devuser\n"), 0644)

	srv, err := New(&mockFactory{}, &Options{
		Address:         "127.0.0.1",
		Port:            "0",
		Path:            "/",
		Quiet:           true,
		EnableProxyAuth: true,
		PasswdFile:      passwdFile,
		GroupFile:       groupFile,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ts := httptest.NewServer(srv.setupHandlers(ctx, cancel, "/", newCounter(0)))
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/sessions", nil)
	req.Header.Set("X-Forwarded-User", "devuser")
	req.Header.Set("Remote-User", "guest")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for conflicting identity headers, got %d", resp.StatusCode)
	}
}
