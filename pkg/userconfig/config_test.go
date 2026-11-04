package userconfig

import (
	"testing"
	"time"
)

func TestSaveAndLoad(t *testing.T) {
	home := t.TempDir()

	// Loading when no file exists returns defaults
	cfg, err := Load(home)
	if err != nil {
		t.Fatalf("Load default failed: %v", err)
	}
	if cfg.Terminal.Theme != "catppuccin-mocha" {
		t.Errorf("got theme %q, want catppuccin-mocha", cfg.Terminal.Theme)
	}
	if cfg.ReapDuration() != 0 {
		t.Errorf("got reap duration %v, want 0", cfg.ReapDuration())
	}

	if cfg.Session.DefaultMode != "sandbox" {
		t.Errorf("got default_mode %q, want sandbox", cfg.Session.DefaultMode)
	}

	// Modify and save
	cfg.Terminal.Theme = "tokyo-night"
	cfg.Terminal.FontSize = 16
	cfg.Terminal.CursorBlink = false
	cfg.Session.ReapTimeout = "2h"
	cfg.Session.DefaultMode = "host"
	cfg.Keybindings.CopyOnSelect = false
	cfg.Keybindings.ClipboardShortcut = "browser_default"
	cfg.Keybindings.NewTabShortcut = "Ctrl+Shift+T"

	if err := Save(home, cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load back
	loaded, err := Load(home)
	if err != nil {
		t.Fatalf("Load after save failed: %v", err)
	}
	if loaded.Terminal.Theme != "tokyo-night" {
		t.Errorf("got theme %q, want tokyo-night", loaded.Terminal.Theme)
	}
	if loaded.Terminal.FontSize != 16 {
		t.Errorf("got font_size %d, want 16", loaded.Terminal.FontSize)
	}
	if loaded.Terminal.CursorBlink != false {
		t.Errorf("got cursor_blink %v, want false", loaded.Terminal.CursorBlink)
	}
	if loaded.Session.ReapTimeout != "2h" {
		t.Errorf("got reap_timeout %q, want 2h", loaded.Session.ReapTimeout)
	}
	if loaded.Session.DefaultMode != "host" {
		t.Errorf("got default_mode %q, want host", loaded.Session.DefaultMode)
	}
	if loaded.ReapDuration() != 2*time.Hour {
		t.Errorf("got ReapDuration %v, want 2h", loaded.ReapDuration())
	}
	if loaded.Keybindings.CopyOnSelect != false {
		t.Errorf("got copy_on_select %v, want false", loaded.Keybindings.CopyOnSelect)
	}
	if loaded.Keybindings.ClipboardShortcut != "browser_default" {
		t.Errorf("got clipboard_shortcut %q, want browser_default", loaded.Keybindings.ClipboardShortcut)
	}
	if loaded.Keybindings.NewTabShortcut != "Ctrl+Shift+T" {
		t.Errorf("got new_tab_shortcut %q, want Ctrl+Shift+T", loaded.Keybindings.NewTabShortcut)
	}
}
