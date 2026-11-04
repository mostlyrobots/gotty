package homedir

import (
	"os"
	"testing"
)

func TestExpand(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")

	if got := Expand("~/.gotty"); got != "/home/testuser/.gotty" {
		t.Errorf("got %q, want /home/testuser/.gotty", got)
	}
	if got := Expand("/etc/passwd"); got != "/etc/passwd" {
		t.Errorf("got %q, want /etc/passwd", got)
	}
	if got := Expand(""); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
	if got := Expand("/"); got != "/" {
		t.Errorf("got %q, want /", got)
	}
	_ = os.Getenv("HOME")
}
