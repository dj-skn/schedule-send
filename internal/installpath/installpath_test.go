package installpath

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveInstallationIdentity(t *testing.T) {
	dir := t.TempDir()
	installed := filepath.Join(dir, "bin space $ % \" ' \\")
	if err := os.WriteFile(installed, []byte("binary"), 0700); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(dir, "linked")
	if err := os.Symlink(installed, linked); err != nil {
		t.Fatal(err)
	}
	copied := filepath.Join(dir, "copy")
	if err := os.WriteFile(copied, []byte("binary"), 0700); err != nil {
		t.Fatal(err)
	}
	encode := func(path string) string { return base64.StdEncoding.EncodeToString([]byte(path)) }
	for _, tc := range []struct{ name, metadata, current, failure string }{
		{"installed", encode(installed), installed, ""},
		{"symlink invocation", encode(installed), linked, ""},
		{"copied", encode(installed), copied, "outside"},
		{"plain build", "", installed, "no installation path"},
		{"bad encoding", "%%%", installed, "invalid installation metadata"},
		{"relative metadata", encode("bin/schedule-send"), installed, "absolute"},
		{"unclean metadata", encode(dir + "/../bin"), installed, "clean"},
		{"missing", encode(filepath.Join(dir, "missing")), installed, "unavailable"},
		{"directory", encode(dir), installed, "executable file"},
		{"control", encode(dir + "/bad\npath"), installed, "control"},
		{"invalid UTF-8", encode(dir + "/\xff"), installed, "UTF-8"},
		{"missing running executable", encode(installed), dir + "/gone", "running executable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolve(tc.metadata, tc.current)
			if tc.failure != "" {
				if err == nil || !strings.Contains(err.Error(), tc.failure) {
					t.Fatalf("got %q, %v; want %s", got, err, tc.failure)
				}
			} else if err != nil || got != installed {
				t.Fatalf("got %q, %v; want %q", got, err, installed)
			}
		})
	}
	if err := os.Chmod(installed, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolve(encode(installed), installed); err == nil {
		t.Fatal("nonexecutable installation accepted")
	}
	if err := os.Rename(installed, copied); err != nil {
		t.Fatal(err)
	}
	if _, err := resolve(encode(installed), copied); err == nil {
		t.Fatal("moved installation accepted")
	}
}

func TestBuildHelperEncodesFinalPath(t *testing.T) {
	dir := t.TempDir()
	fakeGo := filepath.Join(dir, "go")
	if err := os.WriteFile(fakeGo, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	helper, err := filepath.Abs("../../tools/build")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "temporary output")
	final := filepath.Join(dir, "final space % $ \" ' \\")
	out, err := exec.Command(helper, output, final).CombinedOutput()
	if err != nil {
		t.Fatalf("build helper: %v: %s", err, out)
	}
	args := strings.Split(strings.TrimSpace(string(out)), "\n")
	want := []string{"build", "-trimpath", "-ldflags", "-X schedule-send/internal/installpath.Encoded=" + base64.StdEncoding.EncodeToString([]byte(final)), "-o", output, "./cmd/schedule-send"}
	if strings.Join(args, "\n") != strings.Join(want, "\n") {
		t.Fatalf("arguments %q, want %q", args, want)
	}
	for _, invalid := range []string{"relative", dir + "/bad\npath", dir + "/bad\tpath", dir + "/bad\x7fpath"} {
		out, err := exec.Command(helper, output, invalid).CombinedOutput()
		if err == nil {
			t.Fatalf("accepted %q: %s", invalid, out)
		}
	}
}
