package cli

import (
	"bytes"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/dj-skn/schedule-send/internal/show"
	"github.com/dj-skn/schedule-send/internal/term"
)

func TestCtrlCWhileAskingSaysNothingWasScheduled(t *testing.T) {
	var drawn bytes.Buffer
	q := &asker{tty: fakeTTY{strings.NewReader(""), &drawn}, rail: show.NewRail(term.Colors{})}
	exited := make(chan int, 1)
	a := &app{exit: func(code int) { exited <- code }}
	stop := a.cancelOnInterrupt(q)
	defer stop()

	// The handler is in place, so the signal reaches it rather than the test.
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-exited:
		if code != 130 {
			t.Errorf("exit %d, want 130", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Ctrl-C did not end the run")
	}
	if want := "\n└  Cancelled. Nothing was scheduled.\n"; drawn.String() != want {
		t.Errorf("terminal %q, want %q", drawn.String(), want)
	}
}

func TestCtrlCIsHandedBackBeforeArming(t *testing.T) {
	input := "\n\n"
	h := newHarness(t, &input)
	if code := h.a.main([]string{"--pane", "wB:p1", "--dry-run"}); code != 0 {
		t.Fatalf("exit %d: %s", code, h.stderr)
	}
	if h.a.ask == nil || h.a.ask.stop != nil {
		t.Errorf("Ctrl-C is still caught after the questions")
	}
}
