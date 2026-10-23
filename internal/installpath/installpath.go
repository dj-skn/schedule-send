// Package installpath identifies the stable executable recorded by the installer.
package installpath

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Encoded is set by tools/build with the final installation path. An
// installer that knows the path records it whole. A release binary, built
// before anyone installs it, records one beginning "~/", which is the same
// place under every user's home. Plain builds leave it empty and can inspect
// jobs or preview schedules, but cannot arm them.
var Encoded string

// Resolve checks that this process runs at the recorded installation location.
func Resolve() (string, error) {
	current, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find running executable: %w", err)
	}
	return resolve(Encoded, current)
}

func resolve(encoded, current string) (string, error) {
	if encoded == "" {
		return "", errors.New("this build has no installation path")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", errors.New("invalid installation metadata")
	}
	path, err := expand(string(data))
	if err != nil {
		return "", err
	}
	if err := Check(path); err != nil {
		return "", err
	}
	// tools/install publishes a regular file. A symlink there could point the
	// recorded path, and every timer armed through it, at a worktree build.
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("installed executable is unavailable: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("installed path is a symlink, not the installed file")
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return "", errors.New("installed path is not an executable file")
	}
	// Parent directories may still be symlinks; compare the real files.
	installed, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve installation path: %w", err)
	}
	running, err := filepath.EvalSymlinks(current)
	if err != nil {
		return "", fmt.Errorf("resolve running executable: %w", err)
	}
	if running != installed {
		return "", errors.New("this executable is outside its recorded installation location")
	}
	return path, nil
}

// expand turns a recorded path that begins "~/" into this user's. The home
// directory is part of the result, so Check applies to it as well.
func expand(path string) (string, error) {
	rest, ok := strings.CutPrefix(path, "~/")
	if !ok {
		return path, nil
	}
	if err := CheckRecorded(path); err != nil {
		return "", err
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return "", errors.New("installation path is under a home directory that is not known")
	}
	return filepath.Join(home, rest), nil
}

// CheckRecorded is Check for a path as tools/build is given it, which may
// begin "~/". What follows must be a clean path that stays under the home.
func CheckRecorded(path string) error {
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if rest == "" {
			return errors.New("installation path names the home directory itself")
		}
		return Check("/" + rest)
	}
	return Check(path)
}

// Check is the one set of rules for a path that tools/build records and a
// service's ExecStart runs: clean, absolute, UTF-8, without control characters,
// and without '=', which /usr/bin/env would take for a variable assignment
// even after "--".
func Check(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !utf8.ValidString(path) {
		return errors.New("installation path must be a clean absolute UTF-8 path")
	}
	for _, r := range path {
		if unicode.IsControl(r) {
			return errors.New("installation path contains a control character")
		}
	}
	if strings.ContainsRune(path, '=') {
		return errors.New("installation path contains '=', which /usr/bin/env would read as a variable assignment")
	}
	return nil
}
