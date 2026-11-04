package userconfig

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Config represents the per-user configuration stored at ~/.gotty/config.yaml.
type Config struct {
	Version     string            `json:"version"`
	Terminal    TerminalConfig    `json:"terminal"`
	Session     SessionConfig     `json:"session"`
	Keybindings KeybindingsConfig `json:"keybindings"`
}

type TerminalConfig struct {
	Theme           string `json:"theme"`
	FontSize        int    `json:"font_size"`
	FontFamily      string `json:"font_family"`
	CursorBlink     bool   `json:"cursor_blink"`
	ScrollbackLines int    `json:"scrollback_lines"`
}

type SessionConfig struct {
	ReapTimeout string `json:"reap_timeout"` // "never" | "1h" | "2h" | "8h" | "24h"
	DefaultMode string `json:"default_mode"` // "sandbox" | "host"
}

type KeybindingsConfig struct {
	CopyOnSelect      bool   `json:"copy_on_select"`
	ClipboardShortcut string `json:"clipboard_shortcut"` // "ctrl_shift" | "browser_default"
	CloseTabShortcut  string `json:"close_tab_shortcut"`
	NewTabShortcut    string `json:"new_tab_shortcut"`
}

// Default returns a Config initialized with standard defaults.
func Default() *Config {
	return &Config{
		Version: "1.0",
		Terminal: TerminalConfig{
			Theme:           "catppuccin-mocha",
			FontSize:        14,
			FontFamily:      "JetBrains Mono, Menlo, monospace",
			CursorBlink:     true,
			ScrollbackLines: 10000,
		},
		Session: SessionConfig{
			ReapTimeout: "never",
			DefaultMode: "sandbox",
		},
		Keybindings: KeybindingsConfig{
			CopyOnSelect:      true,
			ClipboardShortcut: "ctrl_shift",
			CloseTabShortcut:  "Alt+W",
			NewTabShortcut:    "Alt+T",
		},
	}
}

// ReapDuration parses Session.ReapTimeout into a time.Duration (0 means never reap).
func (c *Config) ReapDuration() time.Duration {
	switch strings.ToLower(strings.TrimSpace(c.Session.ReapTimeout)) {
	case "1h", "1hour":
		return 1 * time.Hour
	case "2h", "2hours":
		return 2 * time.Hour
	case "8h", "8hours":
		return 8 * time.Hour
	case "24h", "1d", "1day":
		return 24 * time.Hour
	case "never", "", "0":
		return 0
	default:
		if d, err := time.ParseDuration(c.Session.ReapTimeout); err == nil && d >= 0 {
			return d
		}
		return 0
	}
}

// ConfigPath returns the path to the user's config.yaml inside their home directory.
func ConfigPath(homeDir string) string {
	return filepath.Join(homeDir, ".gotty", "config.yaml")
}

// Load reads ~/.gotty/config.yaml from homeDir using os.OpenRoot to prevent symlink escapes.
// If the file does not exist, Default() is returned without error.
func Load(homeDir string) (*Config, error) {
	cfg := Default()
	if homeDir == "" {
		return cfg, nil
	}
	fi, err := os.Lstat(homeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("userconfig: failed to lstat homeDir %s: %w", homeDir, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return nil, fmt.Errorf("userconfig: homeDir %s must be a real directory, not a symlink", homeDir)
	}

	root, err := os.OpenRoot(homeDir)
	if err != nil {
		return nil, fmt.Errorf("userconfig: failed to open root %s: %w", homeDir, err)
	}
	defer root.Close()

	f, err := root.OpenFile(filepath.Join(".gotty", "config.yaml"), os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, fmt.Errorf("userconfig: failed to open config.yaml in %s: %w", homeDir, err)
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("userconfig: failed to read config.yaml in %s: %w", homeDir, err)
	}
	if err := ParseYAML(string(data), cfg); err != nil {
		return nil, fmt.Errorf("userconfig: failed to parse config.yaml in %s: %w", homeDir, err)
	}
	return cfg, nil
}

// Save writes the Config as YAML to ~/.gotty/config.yaml inside homeDir.
func Save(homeDir string, cfg *Config) error {
	return SaveAsUser(homeDir, cfg, 0, 0)
}

// SaveAsUser writes the Config as YAML to ~/.gotty/config.yaml inside homeDir
// using os.OpenRoot and O_NOFOLLOW to prevent symlink privilege escalation,
// and sets ownership to uid:gid before writing when running as root.
func SaveAsUser(homeDir string, cfg *Config, uid, gid uint32) error {
	if homeDir == "" {
		return fmt.Errorf("userconfig: empty home directory")
	}
	if cfg.Version == "" {
		cfg.Version = "1.0"
	}
	fi, err := os.Lstat(homeDir)
	if err != nil {
		return fmt.Errorf("userconfig: failed to lstat homeDir %s: %w", homeDir, err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.IsDir() {
		return fmt.Errorf("userconfig: homeDir %s must be a real directory, not a symlink", homeDir)
	}

	root, err := os.OpenRoot(homeDir)
	if err != nil {
		return fmt.Errorf("userconfig: failed to open root %s: %w", homeDir, err)
	}
	defer root.Close()

	if err := root.Mkdir(".gotty", 0755); err != nil && !os.IsExist(err) {
		return fmt.Errorf("userconfig: failed to create .gotty in %s: %w", homeDir, err)
	}

	dirFile, err := root.OpenFile(".gotty", syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_DIRECTORY, 0)
	if err != nil {
		return fmt.Errorf("userconfig: .gotty in %s is not a safe directory: %w", homeDir, err)
	}
	if os.Geteuid() == 0 && uid > 0 {
		_ = dirFile.Chown(int(uid), int(gid))
	}
	_ = dirFile.Close()

	relPath := filepath.Join(".gotty", "config.yaml")
	out, err := root.OpenFile(relPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC|syscall.O_NOFOLLOW, 0644)
	if err != nil {
		return fmt.Errorf("userconfig: failed to open %s in %s safely: %w", relPath, homeDir, err)
	}
	defer out.Close()

	if os.Geteuid() == 0 && uid > 0 {
		if err := out.Chown(int(uid), int(gid)); err != nil {
			return fmt.Errorf("userconfig: failed to chown %s: %w", relPath, err)
		}
	}

	content := MarshalYAML(cfg)
	if _, err := out.WriteString(content); err != nil {
		return fmt.Errorf("userconfig: failed to write %s in %s: %w", relPath, homeDir, err)
	}
	return nil
}

// MarshalYAML serializes Config into the canonical YAML format.
func MarshalYAML(cfg *Config) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("version: %q\n", cfg.Version))
	b.WriteString("terminal:\n")
	b.WriteString(fmt.Sprintf("  theme: %q\n", cfg.Terminal.Theme))
	b.WriteString(fmt.Sprintf("  font_size: %d\n", cfg.Terminal.FontSize))
	b.WriteString(fmt.Sprintf("  font_family: %q\n", cfg.Terminal.FontFamily))
	b.WriteString(fmt.Sprintf("  cursor_blink: %t\n", cfg.Terminal.CursorBlink))
	b.WriteString(fmt.Sprintf("  scrollback_lines: %d\n", cfg.Terminal.ScrollbackLines))
	b.WriteString("session:\n")
	b.WriteString(fmt.Sprintf("  reap_timeout: %q\n", cfg.Session.ReapTimeout))
	b.WriteString(fmt.Sprintf("  default_mode: %q\n", cfg.Session.DefaultMode))
	b.WriteString("keybindings:\n")
	b.WriteString(fmt.Sprintf("  copy_on_select: %t\n", cfg.Keybindings.CopyOnSelect))
	b.WriteString(fmt.Sprintf("  clipboard_shortcut: %q\n", cfg.Keybindings.ClipboardShortcut))
	b.WriteString(fmt.Sprintf("  close_tab_shortcut: %q\n", cfg.Keybindings.CloseTabShortcut))
	b.WriteString(fmt.Sprintf("  new_tab_shortcut: %q\n", cfg.Keybindings.NewTabShortcut))
	return b.String()
}

// ParseYAML parses the structured config YAML into cfg.
func ParseYAML(input string, cfg *Config) error {
	section := ""
	scanner := bufio.NewScanner(strings.NewReader(input))
	for scanner.Scan() {
		raw := scanner.Text()
		// Strip inline comments outside quotes
		line := stripYAMLComment(raw)
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		isIndented := strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := unquoteYAML(strings.TrimSpace(parts[1]))

		if !isIndented {
			if val == "" {
				section = key
				continue
			}
			section = ""
			if key == "version" {
				cfg.Version = val
			}
			continue
		}

		switch section {
		case "terminal":
			switch key {
			case "theme":
				cfg.Terminal.Theme = val
			case "font_size":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Terminal.FontSize = n
				}
			case "font_family":
				cfg.Terminal.FontFamily = val
			case "cursor_blink":
				if b, err := strconv.ParseBool(val); err == nil {
					cfg.Terminal.CursorBlink = b
				}
			case "scrollback_lines":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Terminal.ScrollbackLines = n
				}
			}
		case "session":
			switch key {
			case "reap_timeout":
				cfg.Session.ReapTimeout = val
			case "default_mode":
				cfg.Session.DefaultMode = val
			}
		case "keybindings":
			switch key {
			case "copy_on_select":
				if b, err := strconv.ParseBool(val); err == nil {
					cfg.Keybindings.CopyOnSelect = b
				}
			case "clipboard_shortcut":
				cfg.Keybindings.ClipboardShortcut = val
			case "close_tab_shortcut":
				cfg.Keybindings.CloseTabShortcut = val
			case "new_tab_shortcut":
				cfg.Keybindings.NewTabShortcut = val
			}
		}
	}
	return scanner.Err()
}

func stripYAMLComment(s string) string {
	inSingle := false
	inDouble := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\'' && !inDouble {
			inSingle = !inSingle
		} else if ch == '"' && !inSingle {
			inDouble = !inDouble
		} else if ch == '#' && !inSingle && !inDouble {
			return s[:i]
		}
	}
	return s
}

func unquoteYAML(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
