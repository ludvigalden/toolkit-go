// Command selfupdate_e2e serves a 0.0.2 demo build with checksums.txt
// from a local HTTP server and checks the 0.0.1 build replaces itself
// and refuses a bad checksum. MODULE_ROOT names the module root.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var failures int

func check(ok bool, format string, args ...any) {
	if ok {
		fmt.Printf("ok   %s\n", fmt.Sprintf(format, args...))
		return
	}
	failures++
	fmt.Printf("FAIL %s\n", fmt.Sprintf(format, args...))
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "selfupdate_e2e: "+format+"\n", args...)
	os.Exit(1)
}

func main() {
	root := os.Getenv("MODULE_ROOT")
	if root == "" {
		root, _ = os.Getwd()
	}
	work, err := os.MkdirTemp("", "selfupdate-e2e-*")
	if err != nil {
		fatal("tempdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(work) }()

	v1 := build(root, work, "0.0.1")
	v2 := build(root, work, "0.0.2")

	asset := fmt.Sprintf("demo_%s_%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	archive := tarGz("demo", v2)
	sum := sha256.Sum256(archive)
	good := fmt.Sprintf("%s  %s\n", hex.EncodeToString(sum[:]), asset)
	bad := fmt.Sprintf("%s  %s\n", strings.Repeat("0", 64), asset)

	checksums := good
	mux := http.NewServeMux()
	mux.HandleFunc("/example/demo/manifest.yaml", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintf(w, `last_release_id: 2
last_asset_id: 2
releases:
  - id: 2
    name: demo 0.0.2
    tag_name: v0.0.2
    url: releases/v0.0.2
    published_at: 2026-11-08T00:00:00Z
    release_notes: "Second release."
    assets:
      - id: 1
        name: %s
        size: %d
        url: dl/%s
      - id: 2
        name: checksums.txt
        size: %d
        url: dl/checksums.txt
`, asset, len(archive), asset, len(checksums))
	})
	mux.HandleFunc("/example/demo/dl/"+asset, func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(archive) })
	mux.HandleFunc("/example/demo/dl/checksums.txt", func(w http.ResponseWriter, _ *http.Request) { _, _ = fmt.Fprint(w, checksums) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Tampered round first, on its own copy, so the good round starts clean.
	tampered := filepath.Join(work, "tampered", "demo")
	copyFile(v1, tampered)
	checksums = bad
	out, err := exec.Command(tampered, "self-update", srv.URL).CombinedOutput()
	check(err != nil, "tampered checksum is refused (output: %s)", strings.TrimSpace(string(out)))
	check(run(tampered, "version") == "0.0.1", "tampered round leaves the binary at 0.0.1")

	// Good round: the installed v0.0.1 binary replaces itself.
	checksums = good
	installed := filepath.Join(work, "bin", "demo")
	copyFile(v1, installed)
	check(run(installed, "version") == "0.0.1", "installed binary starts at 0.0.1")
	out, err = exec.Command(installed, "self-update", srv.URL).CombinedOutput()
	check(err == nil && strings.Contains(string(out), "latest=0.0.2 updated=true"),
		"self-update applied 0.0.2 (err %v, output: %s)", err, strings.TrimSpace(string(out)))
	check(run(installed, "version") == "0.0.2", "the binary on disk now prints 0.0.2")

	// A second run is a no-op.
	out, err = exec.Command(installed, "self-update", srv.URL).CombinedOutput()
	check(err == nil && strings.Contains(string(out), "updated=false"), "already-latest run does not update (output: %s)", strings.TrimSpace(string(out)))

	if failures > 0 {
		fmt.Printf("%d check(s) failed\n", failures)
		os.Exit(1)
	}
	fmt.Println("all checks passed")
}

func build(root, work, version string) string {
	out := filepath.Join(work, "build-"+version, "demo")
	cmd := exec.Command("go", "build", "-o", out,
		"-ldflags", "-X main.version="+version,
		"./tests/update/selfupdate_e2e/demo")
	cmd.Dir = root
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatal("build demo %s: %v", version, err)
	}
	return out
}

func run(bin string, args ...string) string {
	out, err := exec.Command(bin, args...).Output()
	if err != nil {
		return "error: " + err.Error()
	}
	return strings.TrimSpace(string(out))
}

func tarGz(name, path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		fatal("read %s: %v", path, err)
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
		fatal("tar header: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		fatal("tar write: %v", err)
	}
	if err := tw.Close(); err != nil {
		fatal("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		fatal("gzip close: %v", err)
	}
	return buf.Bytes()
}

func copyFile(src, dst string) {
	data, err := os.ReadFile(src)
	if err != nil {
		fatal("read %s: %v", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		fatal("mkdir: %v", err)
	}
	if err := os.WriteFile(dst, data, 0o755); err != nil {
		fatal("write %s: %v", dst, err)
	}
}
