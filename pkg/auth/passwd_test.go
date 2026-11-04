package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolve_PasswdAndGroups(t *testing.T) {
	tmpDir := t.TempDir()
	passwdFile := filepath.Join(tmpDir, "passwd")
	groupFile := filepath.Join(tmpDir, "group")

	passwdContent := `# Mock /etc/passwd
root:x:0:0:root:/root:/bin/bash
devuser:x:1000:1000:Dev User,devuser@example.com,,,:/home/devuser:/bin/bash
routeradmin:x:1001:1001:Router Admin <admin@router.lan>,,,:/home/routeradmin:/bin/sh
guest:x:1002:1002:Guest User,guest@example.com,,,:/home/guest:/bin/sh
`
	groupContent := `root:x:0:
wheel:x:10:devuser
sudo:x:27:
gotty-host:x:998:routeradmin
devuser:x:1000:
routeradmin:x:1001:
guest:x:1002:
`

	if err := os.WriteFile(passwdFile, []byte(passwdContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(groupFile, []byte(groupContent), 0644); err != nil {
		t.Fatal(err)
	}

	resolver := &Resolver{
		PasswdPath: passwdFile,
		GroupPath:  groupFile,
	}

	t.Run("match by GECOS email with wheel group -> host access", func(t *testing.T) {
		ident, err := resolver.Resolve("devuser@example.com")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ident.Username != "devuser" {
			t.Errorf("got username %q, want devuser", ident.Username)
		}
		if ident.UID != 1000 || ident.GID != 1000 {
			t.Errorf("got uid/gid %d/%d, want 1000/1000", ident.UID, ident.GID)
		}
		if !ident.IsHostAccess {
			t.Errorf("expected IsHostAccess=true for user in wheel group")
		}
	})

	t.Run("match by angle-bracket email in GECOS with gotty-host group", func(t *testing.T) {
		ident, err := resolver.Resolve("admin@router.lan")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ident.Username != "routeradmin" {
			t.Errorf("got username %q, want routeradmin", ident.Username)
		}
		if !ident.IsHostAccess {
			t.Errorf("expected IsHostAccess=true for user in gotty-host group")
		}
	})

	t.Run("match standard user by email -> sandboxed (IsHostAccess=false)", func(t *testing.T) {
		ident, err := resolver.Resolve("guest@example.com")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ident.Username != "guest" {
			t.Errorf("got username %q, want guest", ident.Username)
		}
		if ident.IsHostAccess {
			t.Errorf("expected IsHostAccess=false for guest user")
		}
	})

	t.Run("match by exact username", func(t *testing.T) {
		ident, err := resolver.Resolve("guest")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ident.Username != "guest" {
			t.Errorf("got username %q, want guest", ident.Username)
		}
	})

	t.Run("unmapped user returns error", func(t *testing.T) {
		_, err := resolver.Resolve("intruder@example.com")
		if err == nil {
			t.Fatalf("expected error for unmapped user, got nil")
		}
	})

	t.Run("empty header returns error", func(t *testing.T) {
		_, err := resolver.Resolve("   ")
		if err == nil {
			t.Fatalf("expected error for empty header, got nil")
		}
	})
}
