// Command mdserver serves a directory of markdown files as a browsable,
// rendered tree on localhost.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type config struct {
	rootDir      string
	listenAddr   string
	unsafeHTML   bool
	extensions   []string
	maxFileBytes int64
}

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "mdserver:", err)
		os.Exit(2)
	}
}

func run(args []string, stderr io.Writer) error {
	cfg, err := parseConfig(args, stderr)
	if err != nil {
		return err
	}
	slog.Info("config parsed; scanning and serving are not implemented yet",
		"root", cfg.rootDir, "addr", cfg.listenAddr, "unsafeHTML", cfg.unsafeHTML,
		"extensions", cfg.extensions, "maxFileBytes", cfg.maxFileBytes)
	return nil
}

func parseConfig(args []string, stderr io.Writer) (config, error) {
	flags := flag.NewFlagSet("mdserver", flag.ContinueOnError)
	flags.SetOutput(stderr)
	listenAddr := flags.String("addr", "127.0.0.1:8080", "listen address")
	unsafeHTML := flags.Bool("unsafe-html", false, "render raw HTML in markdown without sanitizing it")
	extensionList := flags.String("ext", ".md,.markdown", "comma-separated markdown file extensions (case-insensitive)")
	maxSize := flags.String("max-size", "5MB", "largest markdown file to render (e.g. 512KB, 5MB)")
	flags.Usage = func() {
		fmt.Fprintln(stderr, "usage: mdserver [flags] <root-dir>")
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		return config{}, err
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return config{}, errors.New("expected exactly one root directory argument")
	}

	rootDir, err := resolveRootDir(flags.Arg(0))
	if err != nil {
		return config{}, err
	}
	extensions, err := parseExtensions(*extensionList)
	if err != nil {
		return config{}, err
	}
	maxFileBytes, err := parseByteSize(*maxSize)
	if err != nil {
		return config{}, fmt.Errorf("-max-size: %w", err)
	}
	return config{
		rootDir:      rootDir,
		listenAddr:   *listenAddr,
		unsafeHTML:   *unsafeHTML,
		extensions:   extensions,
		maxFileBytes: maxFileBytes,
	}, nil
}

// resolveRootDir returns the root as an absolute path with symlinks resolved,
// which pathsafe.Resolve requires for its containment check.
func resolveRootDir(rawRoot string) (string, error) {
	absRoot, err := filepath.Abs(rawRoot)
	if err != nil {
		return "", fmt.Errorf("root %q: %w", rawRoot, err)
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return "", fmt.Errorf("root %q: %w", rawRoot, err)
	}
	info, err := os.Stat(realRoot)
	if err != nil {
		return "", fmt.Errorf("root %q: %w", rawRoot, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("root %q is not a directory", rawRoot)
	}
	return realRoot, nil
}

func parseExtensions(extensionList string) ([]string, error) {
	var extensions []string
	for _, rawExt := range strings.Split(extensionList, ",") {
		ext := strings.ToLower(strings.TrimSpace(rawExt))
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		extensions = append(extensions, ext)
	}
	if len(extensions) == 0 {
		return nil, errors.New("-ext: at least one extension is required")
	}
	return extensions, nil
}

var byteSizeUnits = []struct {
	suffix     string
	multiplier int64
}{
	// Longest suffixes first so "5MB" isn't matched by "B".
	{"GB", 1 << 30},
	{"MB", 1 << 20},
	{"KB", 1 << 10},
	{"B", 1},
}

// parseByteSize parses sizes like "5MB", "512KB" or "1048576". Units are
// binary (1MB = 1<<20 bytes).
func parseByteSize(rawSize string) (int64, error) {
	sizeText := strings.ToUpper(strings.TrimSpace(rawSize))
	multiplier := int64(1)
	for _, unit := range byteSizeUnits {
		if strings.HasSuffix(sizeText, unit.suffix) {
			sizeText = strings.TrimSpace(strings.TrimSuffix(sizeText, unit.suffix))
			multiplier = unit.multiplier
			break
		}
	}
	count, err := strconv.ParseInt(sizeText, 10, 64)
	if err != nil || count <= 0 {
		return 0, fmt.Errorf("invalid size %q", rawSize)
	}
	if count > (1<<62)/multiplier {
		return 0, fmt.Errorf("size %q is too large", rawSize)
	}
	return count * multiplier, nil
}
