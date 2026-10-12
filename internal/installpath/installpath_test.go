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
		{"C1 control", encode(dir + "/bad\u0085path"), installed, "control"},
		{"equals sign", encode(dir + "/a=b"), installed, "'='"},
		{"symlinked installation", encode(linked), installed, "symlink"},
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
	realGo, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is not on PATH")
	}
	dir := t.TempDir()
	// The stand-in records the build but runs the real path check, so the
	// helper is tested against the same rules arming applies.
	fakeGo := filepath.Join(dir, "go")
	script := "#!/bin/sh\nif [ \"$1\" = run ]; then exec \"$REAL_GO\" \"$@\"; fi\nprintf '%s\\n' \"$@\"\n"
	if err := os.WriteFile(fakeGo, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("REAL_GO", realGo)
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	helper, err := filepath.Abs("../../tools/build")
	if err != nil {
		t.Fatal(err)
	}
	final := filepath.Join(dir, "final space % $ \" ' \\")
	for _, tc := range []struct{ name, output, cwd, want string }{
		{"absolute output", filepath.Join(dir, "temporary output"), "", filepath.Join(dir, "temporary output")},
		{"relative output", "relative output", dir, filepath.Join(dir, "relative output")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(helper, tc.output, final)
			cmd.Dir = tc.cwd
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("build helper: %v: %s", err, out)
			}
			args := strings.Split(strings.TrimSpace(string(out)), "\n")
			want := []string{"build", "-trimpath", "-ldflags", "-X schedule-send/internal/installpath.Encoded=" + base64.StdEncoding.EncodeToString([]byte(final)), "-o", tc.want, "./cmd/schedule-send"}
			if strings.Join(args, "\n") != strings.Join(want, "\n") {
				t.Fatalf("arguments %q, want %q", args, want)
			}
		})
	}
	for _, invalid := range []string{
		"relative", dir + "/bad\npath", dir + "/bad\tpath", dir + "/bad\x7fpath",
		dir + "/bad\u0085path", dir + "/bad\xffpath", dir + "//unclean", dir + "/a=b",
	} {
		out, err := exec.Command(helper, filepath.Join(dir, "output"), invalid).CombinedOutput()
		if err == nil || strings.Contains(string(out), "-trimpath") {
			t.Fatalf("accepted %q: %s", invalid, out)
		}
	}
}
