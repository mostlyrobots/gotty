package main

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmitDownload_SingleFrame(t *testing.T) {
	tmp := t.TempDir()
	fpath := filepath.Join(tmp, "hello.txt")
	content := []byte("hello world from godl")
	if err := os.WriteFile(fpath, content, 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := EmitDownload(&buf, fpath); err != nil {
		t.Fatalf("EmitDownload failed: %v", err)
	}

	out := buf.String()
	if !strings.HasPrefix(out, "\x1b]7777;download;") || !strings.HasSuffix(out, "\x07") {
		t.Fatalf("unexpected OSC framing: %q", out)
	}

	b64Name := base64.StdEncoding.EncodeToString([]byte("hello.txt"))
	b64Data := base64.StdEncoding.EncodeToString(content)

	if !strings.Contains(out, "filename="+b64Name) {
		t.Errorf("missing expected filename=%s in %q", b64Name, out)
	}
	if !strings.Contains(out, "data="+b64Data) {
		t.Errorf("missing expected data=%s in %q", b64Data, out)
	}
}

func TestEmitDownload_ChunkedFrames(t *testing.T) {
	tmp := t.TempDir()
	fpath := filepath.Join(tmp, "large.bin")
	content := bytes.Repeat([]byte("A"), MaxSingleChunkBytes+1024)
	if err := os.WriteFile(fpath, content, 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := EmitDownload(&buf, fpath); err != nil {
		t.Fatalf("EmitDownload failed: %v", err)
	}

	out := buf.String()
	frames := strings.Split(strings.TrimSuffix(out, "\x07"), "\x07")
	if len(frames) != 2 {
		t.Fatalf("expected 2 chunked OSC frames, got %d", len(frames))
	}
	if !strings.Contains(frames[0], "chunk=0;total=2;") {
		t.Errorf("first frame missing chunk=0;total=2: %q", frames[0])
	}
	if !strings.Contains(frames[1], "chunk=1;total=2;") {
		t.Errorf("second frame missing chunk=1;total=2: %q", frames[1])
	}
}
