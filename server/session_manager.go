package server

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"

	"github.com/sorenisanerd/gotty/pkg/auth"
)

// SessionID uniquely identifies a server-side terminal session (UUID v4).
type SessionID string

const (
	// MaxSessionsPerUser bounds the number of concurrent sessions a single user can create.
	MaxSessionsPerUser = 16
	// MaxClientBufferSize bounds the unread byte buffer of an individual slow WebSocket client (4MB).
	MaxClientBufferSize = 4 * 1024 * 1024
)

// SessionInfo is the JSON representation of an active session for the REST API.
type SessionInfo struct {
	ID              SessionID `json:"id"`
	Name            string    `json:"name"`
	Username        string    `json:"username"`
	Mode            string    `json:"mode"`
	IsHostAccess    bool      `json:"is_host_access"`
	AttachedClients int       `json:"attached_clients"`
	CreatedAt       time.Time `json:"created_at"`
	LastDetachedAt  time.Time `json:"last_detached_at,omitempty"`
}

// Session represents a persistent server-side PTY/Container session with a circular replay buffer.
type Session struct {
	ID              SessionID                        `json:"id"`
	Name            string                           `json:"name"`
	Username        string                           `json:"username"`
	Mode            string                           `json:"mode"`
	UID             uint32                           `json:"uid"`
	GID             uint32                           `json:"gid"`
	HomeDir         string                           `json:"home_dir"`
	IsHostAccess    bool                             `json:"is_host_access"`
	ContainerID     string                           `json:"container_id"`
	PTYMaster       Slave                            `json:"-"`
	RingBuffer      *CircularBuffer                  `json:"-"`
	AttachedClients map[*sessionClientSlave]struct{} `json:"-"`
	WSMutex         sync.RWMutex                     `json:"-"`
	ReapTimer       *time.Timer                      `json:"-"`
	ReapDuration    time.Duration                    `json:"-"`
	CreatedAt       time.Time                        `json:"created_at"`
	LastDetachedAt  time.Time                        `json:"last_detached_at"`
	closed          bool
	onReap          func(SessionID)
	oscStripper     oscRingStripper
}

// oscRingStripper filters transient OSC 7777 (godl download) and OSC 52 (clipboard write)
// sequences out of the 1MB reconnect RingBuffer so page reloads do not re-trigger downloads
// or evict terminal scrollback history, while live attached clients still receive them.
type oscRingStripper struct {
	suppressing bool
	escInOSC    bool
	prefix      []byte
}

var (
	osc7777Prefix = []byte("\x1b]7777;")
	osc52Prefix   = []byte("\x1b]52;")
)

func (o *oscRingStripper) Filter(chunk []byte) []byte {
	var out []byte
	for _, b := range chunk {
		if o.suppressing {
			if o.escInOSC {
				o.escInOSC = false
				if b == '\\' {
					o.suppressing = false
				} else if b == 0x1b {
					o.escInOSC = true
				}
				continue
			}
			if b == 0x07 { // BEL terminates OSC
				o.suppressing = false
			} else if b == 0x1b {
				o.escInOSC = true
			}
			continue
		}

		if len(o.prefix) > 0 || b == 0x1b {
			o.prefix = append(o.prefix, b)
			if bytes.Equal(o.prefix, osc7777Prefix) || bytes.Equal(o.prefix, osc52Prefix) {
				o.prefix = o.prefix[:0]
				o.suppressing = true
				o.escInOSC = false
				continue
			}
			if bytes.HasPrefix(osc7777Prefix, o.prefix) || bytes.HasPrefix(osc52Prefix, o.prefix) {
				// Still matching prefix across chunk boundary
				continue
			}
			// Not a matching OSC 7777/52 prefix: flush accumulated prefix bytes
			out = append(out, o.prefix...)
			o.prefix = o.prefix[:0]
			continue
		}

		out = append(out, b)
	}
	return out
}

// SessionSpawner is a function that creates a new Slave for a given user, session ID, and mode.
type SessionSpawner func(user *auth.UserIdentity, sessionID SessionID, mode string) (Slave, error)

// SessionManager manages persistent user sessions, ring buffers, and reap timers.
type SessionManager struct {
	mu             sync.RWMutex
	sessions       map[SessionID]*Session
	spawner        SessionSpawner
	ringBufferSize int
}

// NewSessionManager creates a new SessionManager.
func NewSessionManager(spawner SessionSpawner) *SessionManager {
	return &SessionManager{
		sessions:       make(map[SessionID]*Session),
		spawner:        spawner,
		ringBufferSize: DefaultRingBufferSize,
	}
}

// generateUUIDv4 returns a random RFC 4122 UUID v4 string.
func generateUUIDv4() SessionID {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return SessionID(fmt.Sprintf("%s-%s-%s-%s-%s",
		hex.EncodeToString(b[0:4]),
		hex.EncodeToString(b[4:6]),
		hex.EncodeToString(b[6:8]),
		hex.EncodeToString(b[8:10]),
		hex.EncodeToString(b[10:16]),
	))
}

// CreateSession spawns a new PTY/Container session for the authenticated user.
func (sm *SessionManager) CreateSession(user *auth.UserIdentity, name string, mode string, reapDuration time.Duration) (*Session, error) {
	if user == nil {
		return nil, errors.New("session_manager: user identity is required")
	}

	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		if user.IsHostAccess {
			mode = "host"
		} else {
			mode = "sandbox"
		}
	}
	// Normalize any privileged requests: privileged container mode is removed
	if mode == "privileged" || mode == "privileged_container" {
		if user.IsHostAccess {
			mode = "host"
		} else {
			mode = "sandbox"
		}
	}
	if !user.IsHostAccess && mode == "host" {
		mode = "sandbox"
	}

	existingSessions := sm.ListSessions(user.Username)
	if len(existingSessions) >= MaxSessionsPerUser {
		return nil, fmt.Errorf("session limit reached (%d active sessions maximum per user)", MaxSessionsPerUser)
	}

	id := generateUUIDv4()
	slave, err := sm.spawner(user, id, mode)
	if err != nil {
		return nil, errors.Wrap(err, "failed to spawn session backend")
	}

	if name == "" {
		count := len(existingSessions) + 1
		name = fmt.Sprintf("Session %d", count)
	}

	sess := &Session{
		ID:              id,
		Name:            name,
		Username:        user.Username,
		Mode:            mode,
		UID:             user.UID,
		GID:             user.GID,
		HomeDir:         user.HomeDir,
		IsHostAccess:    user.IsHostAccess,
		ContainerID:     fmt.Sprintf("gotty-%s-%s", user.Username, id),
		PTYMaster:       slave,
		RingBuffer:      NewCircularBuffer(sm.ringBufferSize),
		AttachedClients: make(map[*sessionClientSlave]struct{}),
		ReapDuration:    reapDuration,
		CreatedAt:       time.Now(),
		LastDetachedAt:  time.Now(),
		onReap: func(sid SessionID) {
			sm.mu.Lock()
			delete(sm.sessions, sid)
			sm.mu.Unlock()
		},
	}

	sm.mu.Lock()
	sm.sessions[id] = sess
	sm.mu.Unlock()

	// Start background reader from PTYMaster
	go sess.readLoop()

	// Start initial reap timer until first client attaches
	sess.WSMutex.Lock()
	sess.armReapTimerLocked()
	sess.WSMutex.Unlock()

	return sess, nil
}

// GetSession retrieves a session owned by username.
func (sm *SessionManager) GetSession(id SessionID, username string) (*Session, error) {
	sm.mu.RLock()
	sess, ok := sm.sessions[id]
	sm.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session %s not found", id)
	}
	if username != "" && sess.Username != username {
		return nil, fmt.Errorf("session %s does not belong to user %s", id, username)
	}
	return sess, nil
}

// ListSessions returns all active sessions for the given username.
func (sm *SessionManager) ListSessions(username string) []SessionInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	var list []SessionInfo
	for _, s := range sm.sessions {
		if username != "" && s.Username != username {
			continue
		}
		s.WSMutex.RLock()
		info := SessionInfo{
			ID:              s.ID,
			Name:            s.Name,
			Username:        s.Username,
			Mode:            s.Mode,
			IsHostAccess:    s.IsHostAccess,
			AttachedClients: len(s.AttachedClients),
			CreatedAt:       s.CreatedAt,
			LastDetachedAt:  s.LastDetachedAt,
		}
		s.WSMutex.RUnlock()
		list = append(list, info)
	}
	return list
}

// RenameSession updates the human-readable tab title of a session.
func (sm *SessionManager) RenameSession(id SessionID, username string, newName string) error {
	sess, err := sm.GetSession(id, username)
	if err != nil {
		return err
	}
	sess.WSMutex.Lock()
	sess.Name = newName
	sess.WSMutex.Unlock()
	return nil
}

// UpdateUserReapDuration updates the reap policy for all active sessions belonging to username.
func (sm *SessionManager) UpdateUserReapDuration(username string, d time.Duration) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	for _, s := range sm.sessions {
		if s.Username == username {
			s.WSMutex.Lock()
			s.ReapDuration = d
			if len(s.AttachedClients) == 0 && !s.closed {
				s.armReapTimerLocked()
			}
			s.WSMutex.Unlock()
		}
	}
}

// DeleteSession terminates a session and removes it from the registry.
func (sm *SessionManager) DeleteSession(id SessionID, username string) error {
	sess, err := sm.GetSession(id, username)
	if err != nil {
		return err
	}
	sess.Terminate()
	return nil
}

// CloseAll terminates all active sessions managed by the SessionManager during server shutdown.
func (sm *SessionManager) CloseAll() {
	sm.mu.RLock()
	all := make([]*Session, 0, len(sm.sessions))
	for _, s := range sm.sessions {
		all = append(all, s)
	}
	sm.mu.RUnlock()

	for _, s := range all {
		s.Terminate()
	}
}

// ResolveSandboxContainerPID finds an active sandboxed container session for username
// and returns its container PID on the host (if any) and whether the user has an active container session.
func (sm *SessionManager) ResolveSandboxContainerPID(username string) (int, bool) {
	sm.mu.RLock()
	var candidates []*Session
	for _, s := range sm.sessions {
		if username == "" || s.Username == username {
			candidates = append(candidates, s)
		}
	}
	sm.mu.RUnlock()

	hasContainer := false
	for _, s := range candidates {
		if resolver, ok := s.PTYMaster.(ContainerPIDResolver); ok && resolver.IsSandboxedContainer() {
			hasContainer = true
			if pid, err := resolver.ResolveContainerPID(); err == nil && pid > 0 {
				return pid, true
			}
		}
	}
	return 0, hasContainer
}

// Attach connects a new virtual Slave client to the session, replaying the circular buffer.
func (sm *SessionManager) Attach(id SessionID, username string) (Slave, error) {
	sess, err := sm.GetSession(id, username)
	if err != nil {
		return nil, err
	}
	return sess.Attach(), nil
}

// Attach registers a new client to the session, replays RingBuffer, and cancels any reap timer.
func (s *Session) Attach() Slave {
	client := &sessionClientSlave{
		session: s,
	}
	client.cond = sync.NewCond(&client.mu)

	s.WSMutex.Lock()
	defer s.WSMutex.Unlock()

	if s.ReapTimer != nil {
		s.ReapTimer.Stop()
		s.ReapTimer = nil
	}

	// Seed client buffer with RingBuffer replay snapshot while holding WSMutex
	replay := s.RingBuffer.Bytes()
	if len(replay) > 0 {
		client.buf.Write(replay)
	}

	if s.closed {
		client.closed = true
	} else {
		s.AttachedClients[client] = struct{}{}
	}

	return client
}

func (s *Session) detach(client *sessionClientSlave) {
	s.WSMutex.Lock()
	defer s.WSMutex.Unlock()

	if _, exists := s.AttachedClients[client]; !exists {
		return
	}
	delete(s.AttachedClients, client)
	s.LastDetachedAt = time.Now()

	if len(s.AttachedClients) == 0 && !s.closed {
		s.armReapTimerLocked()
	}
}

func (s *Session) armReapTimerLocked() {
	if s.ReapTimer != nil {
		s.ReapTimer.Stop()
		s.ReapTimer = nil
	}
	if s.ReapDuration <= 0 {
		return
	}
	s.ReapTimer = time.AfterFunc(s.ReapDuration, func() {
		s.Terminate()
	})
}

// Terminate closes the underlying PTY/Container, disconnects all attached clients, and removes the session.
func (s *Session) Terminate() {
	s.WSMutex.Lock()
	if s.closed {
		s.WSMutex.Unlock()
		return
	}
	s.closed = true
	if s.ReapTimer != nil {
		s.ReapTimer.Stop()
		s.ReapTimer = nil
	}
	clients := make([]*sessionClientSlave, 0, len(s.AttachedClients))
	for c := range s.AttachedClients {
		clients = append(clients, c)
	}
	s.AttachedClients = make(map[*sessionClientSlave]struct{})
	s.WSMutex.Unlock()

	for _, c := range clients {
		c.closeInternal()
	}

	if s.PTYMaster != nil {
		_ = s.PTYMaster.Close()
	}
	s.RingBuffer.Reset()

	if s.onReap != nil {
		s.onReap(s.ID)
	}
}

func (s *Session) readLoop() {
	buf := make([]byte, 4096)
	for {
		n, err := s.PTYMaster.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			s.WSMutex.Lock()
			if filtered := s.oscStripper.Filter(chunk); len(filtered) > 0 {
				_, _ = s.RingBuffer.Write(filtered)
			}
			for c := range s.AttachedClients {
				c.push(chunk)
			}
			s.WSMutex.Unlock()
		}
		if err != nil {
			s.Terminate()
			return
		}
	}
}

// ResolveCWD resolves the active working directory of the session's shell process.
func (s *Session) ResolveCWD() (string, error) {
	if resolver, ok := s.PTYMaster.(CWDResolver); ok {
		if cwd, err := resolver.ResolveCWD(); err == nil && cwd != "" {
			return cwd, nil
		}
	}
	return "", errors.New("session slave does not support CWD resolution")
}

// sessionClientSlave adapts an attached WebSocket client to the server.Slave interface.
// Closing sessionClientSlave detaches the client without terminating the underlying Session.
type sessionClientSlave struct {
	session *Session
	mu      sync.Mutex
	cond    *sync.Cond
	buf     bytes.Buffer
	closed  bool
}

func (c *sessionClientSlave) push(data []byte) {
	c.mu.Lock()
	if !c.closed {
		if c.buf.Len()+len(data) > MaxClientBufferSize {
			// Drop oldest unread bytes so a slow client never causes unbounded OOM growth
			excess := (c.buf.Len() + len(data)) - MaxClientBufferSize
			if excess >= c.buf.Len() {
				c.buf.Reset()
				if len(data) > MaxClientBufferSize {
					data = data[len(data)-MaxClientBufferSize:]
				}
			} else {
				_ = c.buf.Next(excess)
			}
		}
		_, _ = c.buf.Write(data)
		c.cond.Signal()
	}
	c.mu.Unlock()
}

func (c *sessionClientSlave) closeInternal() {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		c.cond.Broadcast()
	}
	c.mu.Unlock()
}

func (c *sessionClientSlave) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for c.buf.Len() == 0 && !c.closed {
		c.cond.Wait()
	}
	if c.buf.Len() > 0 {
		return c.buf.Read(p)
	}
	return 0, io.EOF
}

func (c *sessionClientSlave) Write(p []byte) (int, error) {
	c.session.WSMutex.RLock()
	closed := c.session.closed
	s := c.session.PTYMaster
	c.session.WSMutex.RUnlock()
	if closed || s == nil {
		return 0, io.ErrClosedPipe
	}
	return s.Write(p)
}

// Close detaches this WebSocket client from the session while keeping the session alive.
func (c *sessionClientSlave) Close() error {
	c.closeInternal()
	c.session.detach(c)
	return nil
}

func (c *sessionClientSlave) WindowTitleVariables() map[string]interface{} {
	if c.session.PTYMaster != nil {
		vars := c.session.PTYMaster.WindowTitleVariables()
		if vars != nil {
			return vars
		}
	}
	return map[string]interface{}{
		"session_id": c.session.ID,
		"username":   c.session.Username,
	}
}

func (c *sessionClientSlave) ResizeTerminal(columns int, rows int) error {
	if c.session.PTYMaster != nil {
		return c.session.PTYMaster.ResizeTerminal(columns, rows)
	}
	return nil
}
