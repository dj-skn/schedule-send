// Package installpath identifies the stable executable recorded by the installer.
package installpath

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"unicode"
	"unicode/utf8"
)

// Encoded is set by tools/build with the final installation path. Plain builds
// leave it empty and can inspect jobs or preview schedules, but cannot arm them.
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
	path := string(data)
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !utf8.ValidString(path) {
		return "", errors.New("installation path must be a clean absolute UTF-8 path")
	}
	for _, r := range path {
		if unicode.IsControl(r) {
			return "", errors.New("installation path contains a control character")
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("installed executable is unavailable: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return "", errors.New("installed path is not an executable file")
	}
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
