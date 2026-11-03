package main

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestParseConfig(t *testing.T) {
	rootDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(rootDir, "file.md")
	if err := os.WriteFile(filePath, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := parseConfig([]string{"-ext", "MD, txt", "-max-size", "512kb", rootDir}, io.Discard)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.rootDir != rootDir || cfg.listenAddr != "127.0.0.1:8642" || cfg.unsafeHTML {
		t.Fatalf("unexpected config: %+v", cfg)
	}
	if !slices.Equal(cfg.extensions, []string{".md", ".txt"}) {
		t.Fatalf("extensions = %v", cfg.extensions)
	}
	if cfg.maxFileBytes != 512<<10 {
		t.Fatalf("maxFileBytes = %d", cfg.maxFileBytes)
	}

	errorCases := map[string][]string{
		"no root":       {},
		"two roots":     {rootDir, rootDir},
		"missing root":  {filepath.Join(rootDir, "nope")},
		"root is file":  {filePath},
		"empty ext":     {"-ext", " , ", rootDir},
		"bad max size":  {"-max-size", "lots", rootDir},
		"zero max size": {"-max-size", "0", rootDir},
	}
	for name, args := range errorCases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseConfig(args, io.Discard); err == nil {
				t.Fatalf("parseConfig(%q) succeeded, want error", args)
			}
		})
	}
}

func TestParseByteSize(t *testing.T) {
	tests := map[string]int64{
		"1048576": 1 << 20,
		"5MB":     5 << 20,
		"5 mb":    5 << 20,
		"2GB":     2 << 30,
		"10B":     10,
	}
	for input, want := range tests {
		got, err := parseByteSize(input)
		if err != nil || got != want {
			t.Errorf("parseByteSize(%q) = %d, %v; want %d", input, got, err, want)
		}
	}
	for _, input := range []string{"", "MB", "-1MB", "1.5MB", "1TB", "99999999999GB"} {
		if _, err := parseByteSize(input); err == nil {
			t.Errorf("parseByteSize(%q) succeeded, want error", input)
		}
	}
}
