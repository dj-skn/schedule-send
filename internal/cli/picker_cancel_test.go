package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dj-skn/schedule-send/internal/job"
)

const pickerFirstSend = "20261003-160100-wAp1"

func pickerSchedulerLog(t *testing.T, h *harness) string {
	t.Helper()
	log := filepath.Join(h.dir, "scheduler.log")
	t.Setenv("SYSTEMCTL_STANDIN_LOG", log)
	t.Setenv("LAUNCHCTL_STANDIN_LOG", log)
	return log
}

func pickerSchedulerWrites(t *testing.T, log string) []string {
	t.Helper()
	b, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	var writes []string
	for line := range strings.SplitSeq(string(b), "\n") {
		args := strings.Fields(strings.TrimPrefix(line, "--user "))
		if len(args) == 0 {
			continue
		}
		switch args[0] {
		case "stop", "disable", "enable", "daemon-reload", "reset-failed", "bootstrap", "bootout":
			writes = append(writes, line)
		}
	}
	return writes
}

func TestPickerCancelsOnlyTheConfirmedSend(t *testing.T) {
	const chosen = "20261003-163000-wAp1"
	const otherPane = "20261003-160100-wBp5"
	// Go back with Enter once, then re-enter and confirm the second send.
	input := "c 1\nc 4\n3\n1\nx\n\nc 4\n2\ny\n"
	h := newHarness(t, &input)
	h.a.ttyWidth = func() int { return 52 }
	twoLimited(t, h)
	laterSend(t, h)
	h.schedule(t, job.State{ID: otherPane, Kind: "pane", PaneID: "wB:p5", Agent: "-",
		Cwd: "/home/dev/projects/shop-backend", At: "2026-10-03 16:01:00", RunAt: 1791036060})
	h.armed(t, otherPane)
	log := pickerSchedulerLog(t, h)

	if code := h.a.main(nil); code != 0 {
		t.Fatalf("exit %d\nterminal:\n%s\nstderr:\n%s", code, h.tty, h.stderr)
	}
	if want := "cancelled " + chosen + "\n"; h.stdout.String() != want {
		t.Errorf("receipt = %q, want %q", h.stdout, want)
	}
	for _, want := range []string{
		"Use c N with a queued pane number.",
		"Enter a number from 1 to 2.",
		"Type y or n, or press Enter.",
		"Cancel this send?\n│  wA:p1 at 2026-10-03 16:30:00\n│  ID " + chosen,
	} {
		if !strings.Contains(h.tty.String(), want) {
			t.Errorf("terminal lacks %q:\n%s", want, h.tty)
		}
	}
	for line := range strings.SplitSeq(h.tty.String(), "\n") {
		if width := utf8.RuneCountInString(line); width > 52 {
			t.Errorf("terminal line is %d columns: %q", width, line)
		}
	}
	for _, path := range []string{h.a.paths.StatePath(chosen), h.a.paths.MessagePath(chosen)} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("selected send still has %s: %v", path, err)
		}
	}
	for _, id := range []string{pickerFirstSend, otherPane} {
		for _, path := range []string{h.a.paths.StatePath(id), h.a.paths.MessagePath(id)} {
			if _, err := os.Stat(path); err != nil {
				t.Errorf("unselected send %s lost %s: %v", id, path, err)
			}
		}
	}
	if files := h.a.paths.StateFiles(); len(files) != 2 {
		t.Errorf("expected only the two unselected sends, got %v", files)
	}
	writes := pickerSchedulerWrites(t, log)
	stopped := false
	for _, line := range writes {
		if strings.Contains(line, "stop ") || strings.HasPrefix(line, "bootout ") {
			stopped = stopped || strings.Contains(line, chosen)
		}
		if strings.Contains(line, pickerFirstSend) || strings.Contains(line, otherPane) ||
			strings.HasPrefix(line, "--user enable ") || strings.HasPrefix(line, "bootstrap ") {
			t.Errorf("unexpected scheduler write: %s", line)
		}
	}
	if !stopped {
		t.Errorf("scheduler never stopped %s: %v", chosen, writes)
	}
}

func TestPickerCancelRequiresAnExplicitConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		args        []string
	}{
		{name: "decline", input: "c 4\nn\n"},
		{name: "end before Enter", input: "c 4\ny"},
		{name: "dry run", input: "c 4\ny\n", args: []string{"--dry-run"}},
		{name: "JSON", input: "c 4\ny\n", args: []string{"--json"}},
		{name: "non-terminal", args: []string{"--at", "23:00", "--message", "test"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var input *string
			if tc.input != "" {
				input = &tc.input
			}
			h := newHarness(t, input)
			twoLimited(t, h)
			log := pickerSchedulerLog(t, h)

			if code := h.a.main(tc.args); code == 0 {
				t.Fatalf("unexpected success\nterminal:\n%s\nstdout:\n%s", h.tty, h.stdout)
			}
			if _, err := os.Stat(h.a.paths.StatePath(pickerFirstSend)); err != nil {
				t.Errorf("unconfirmed cancellation lost state: %v", err)
			}
			if writes := pickerSchedulerWrites(t, log); len(writes) != 0 {
				t.Errorf("unconfirmed cancellation changed the scheduler: %v", writes)
			}
			shown := strings.Contains(h.tty.String(), pickerFirstSend)
			if shown != (len(tc.args) == 0) {
				t.Errorf("cancellation confirmation shown = %v for %v:\n%s", shown, tc.args, h.tty)
			}
		})
	}
}

// beforePickerConfirmTTY replaces the send just as the picker starts reading
// the affirmative answer, after displaying its confirmation.
type beforePickerConfirmTTY struct {
	fakeTTY
	beforeConfirm func()
}

func (f *beforePickerConfirmTTY) Read(p []byte) (int, error) {
	if f.beforeConfirm != nil && f.in.Len() == len("y\n") {
		f.beforeConfirm()
		f.beforeConfirm = nil
	}
	return f.fakeTTY.Read(p)
}

func TestPickerCancelRefusesAReplacementWithTheSameID(t *testing.T) {
	input := "c 4\ny\n"
	h := newHarness(t, &input)
	twoLimited(t, h)
	log := pickerSchedulerLog(t, h)
	statePath := h.a.paths.StatePath(pickerFirstSend)
	msgPath := h.a.paths.MessagePath(pickerFirstSend)
	const replacement = "a different message at the same time"
	h.a.openTTY = func() (io.ReadWriteCloser, error) {
		return &beforePickerConfirmTTY{fakeTTY{strings.NewReader(input), h.tty}, func() {
			state, err := job.Load(statePath)
			if err != nil {
				t.Fatal(err)
			}
			lock, err := h.a.paths.Lock(state.ID)
			if err != nil {
				t.Fatalf("confirmation holds the job lock: %v", err)
			}
			defer lock.Close()
			if err := os.Remove(statePath); err != nil {
				t.Fatal(err)
			}
			msg := replacement
			if err := h.a.paths.Publish(state, &msg); err != nil {
				t.Fatal(err)
			}
		}}, nil
	}

	if code := h.a.main(nil); code == 0 {
		t.Fatalf("cancellation claimed success:\n%s", h.stdout)
	}
	if !strings.Contains(h.tty.String(), pickerFirstSend) {
		t.Errorf("confirmation lacks selected job ID:\n%s", h.tty)
	}
	if msg, err := os.ReadFile(msgPath); err != nil || string(msg) != replacement {
		t.Errorf("replacement message = %q, %v", msg, err)
	}
	if _, err := os.Stat(statePath); err != nil {
		t.Errorf("replacement state missing: %v", err)
	}
	if writes := pickerSchedulerWrites(t, log); len(writes) != 0 {
		t.Errorf("stale confirmation changed the scheduler: %v", writes)
	}
}
