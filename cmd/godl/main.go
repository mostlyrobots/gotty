package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const MaxSingleChunkBytes = 512 * 1024 // 512KB raw payload per OSC 7777 frame

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: godl <filepath> [filepath2 ...]\n")
		os.Exit(1)
	}

	for _, path := range os.Args[1:] {
		if err := EmitDownload(os.Stdout, path); err != nil {
			fmt.Fprintf(os.Stderr, "godl: %v\n", err)
			os.Exit(1)
		}
	}
}

// EmitDownload reads filePath and writes OSC 7777 download sequence(s) to out.
func EmitDownload(out io.Writer, filePath string) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("cannot stat %s: %w", filePath, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", filePath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", filePath, err)
	}

	filename := filepath.Base(filePath)
	b64Filename := base64.StdEncoding.EncodeToString([]byte(filename))
	totalSize := len(data)

	if totalSize <= MaxSingleChunkBytes {
		b64Data := base64.StdEncoding.EncodeToString(data)
		_, err = fmt.Fprintf(out, "\x1b]7777;download;filename=%s;size=%d;data=%s\x07", b64Filename, totalSize, b64Data)
		return err
	}

	// Chunked transfer for files > 512KB
	transferID := randomID()
	totalChunks := (totalSize + MaxSingleChunkBytes - 1) / MaxSingleChunkBytes

	for seq := 0; seq < totalChunks; seq++ {
		start := seq * MaxSingleChunkBytes
		end := start + MaxSingleChunkBytes
		if end > totalSize {
			end = totalSize
		}
		b64Chunk := base64.StdEncoding.EncodeToString(data[start:end])
		_, err = fmt.Fprintf(
			out,
			"\x1b]7777;download;id=%s;chunk=%d;total=%d;filename=%s;size=%d;data=%s\x07",
			transferID,
			seq,
			totalChunks,
			b64Filename,
			totalSize,
			b64Chunk,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func randomID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
