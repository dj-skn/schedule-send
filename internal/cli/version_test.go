package cli

import (
	"bytes"
	"testing"
)

func TestVersion(t *testing.T) {
	prev := Version
	Version = "1.2.3"
	t.Cleanup(func() { Version = prev })
	for args, want := range map[string]string{
		"--version":        "schedule-send 1.2.3\n",
		"--version --json": "{\"version\":\"1.2.3\"}\n",
	} {
		var stdout, stderr bytes.Buffer
		a := &app{stdout: &stdout, stderr: &stderr}
		var argv []string
		for _, arg := range bytes.Fields([]byte(args)) {
			argv = append(argv, string(arg))
		}
		if code := a.main(argv); code != 0 || stdout.String() != want || stderr.Len() != 0 {
			t.Errorf("%s: exit %d, stdout %q, stderr %q", args, code, stdout.String(), stderr.String())
		}
	}
	var stdout, stderr bytes.Buffer
	a := &app{stdout: &stdout, stderr: &stderr}
	if code := a.main([]string{"--version", "--at", "10:00"}); code == 0 || stdout.Len() != 0 {
		t.Errorf("a version request with a scheduling option: exit %d, stdout %q", code, stdout.String())
	}
}
