package job

import (
	"testing"
	"time"
)

// stepClock is a wall clock that only moves when await sleeps.
type stepClock struct {
	now   time.Time
	slept []time.Duration
}

func (c *stepClock) Now() time.Time { return c.now }
func (c *stepClock) Sleep(d time.Duration) {
	c.slept = append(c.slept, d)
	c.now = c.now.Add(d)
}

func TestAwait(t *testing.T) {
	const runAt = 1791036060
	at := time.Unix(runAt, 0)
	cases := []struct {
		name               string
		start              time.Duration // relative to the firing time
		atLoad, persistent bool
		want               Verdict
		waited             time.Duration
	}{
		{"on time", 0, false, false, Go, 0},
		{"a calendar start five seconds late", 5 * time.Second, false, false, Go, 0},
		{"started at load, waits out the seconds", -75 * time.Second, true, false, Go, 75 * time.Second},
		{"an hour early waits it out", -time.Hour, false, false, Go, time.Hour},
		{"loaded far ahead stands down", -3 * time.Hour, false, true, StandDown, 0},
		{"woken hours late still delivers", 9 * time.Hour, false, false, Go, 0},
		{"persistent, loaded days late, delivers", 3 * 24 * time.Hour, false, true, Go, 0},
		{"at load, minutes late, not persistent", 6 * time.Minute, true, false, Drop, 0},
		{"at load, minutes late, persistent", 6 * time.Minute, true, true, Go, 0},
		{"at load, inside the grace", 4 * time.Minute, true, false, Go, 0},
		{"a year late never delivers", 365 * 24 * time.Hour, false, true, Drop, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clock := &stepClock{now: at.Add(c.start)}
			got, why := await(runAt, clock.Now, clock.Sleep, c.atLoad, c.persistent)
			if got != c.want {
				t.Fatalf("verdict %d (%q), want %d", got, why, c.want)
			}
			if (got == Go) != (why == "") {
				t.Errorf("reason %q for verdict %d", why, got)
			}
			var waited time.Duration
			for _, d := range clock.slept {
				if d > waitStep {
					t.Errorf("slept %v in one step, more than %v", d, waitStep)
				}
				waited += d
			}
			if waited != c.waited {
				t.Errorf("waited %v, want %v", waited, c.waited)
			}
		})
	}
}

func TestAwaitWithoutARecordedInstant(t *testing.T) {
	clock := &stepClock{now: time.Unix(1791036060, 0)}
	if got, _ := await(0, clock.Now, clock.Sleep, true, false); got != Go || len(clock.slept) != 0 {
		t.Fatalf("verdict %d after %d sleeps, want an immediate go", got, len(clock.slept))
	}
}
