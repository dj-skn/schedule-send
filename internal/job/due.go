package job

import "time"

// Verdict is what a started worker does about its job's time.
type Verdict int

const (
	Go        Verdict = iota // deliver now
	StandDown                // exit without touching the job: its trigger starts it again
	Drop                     // never deliver: report and remove the job
)

// Bounds for a scheduler that can start a job at the wrong time.
const (
	maxEarly = 2 * time.Hour      // a start at load, or a clock change, ahead of the time
	maxLate  = 7 * 24 * time.Hour // past this a job is stale, whatever started it
	waitStep = 30 * time.Second   // the wall clock is read again at least this often
)

// await holds a started worker until its job's firing time and says what to
// do then. A late start is delivered: the machine was asleep at the time, or
// the job is one that is loaded again at login because it was asked to
// survive that. A zero runAt is a job without a recorded instant, which is
// delivered as started.
//
// It waits in short steps against the wall clock, since a timer does not
// advance while the machine sleeps.
func await(runAt int64, now func() time.Time, sleep func(time.Duration)) (Verdict, string) {
	if runAt == 0 {
		return Go, ""
	}
	at := time.Unix(runAt, 0)
	for {
		early := at.Sub(now())
		if early > maxEarly {
			return StandDown, "was started ahead of its time and waits for its trigger"
		}
		if early <= 0 {
			if -early > maxLate {
				return Drop, "it is more than 7 days late"
			}
			return Go, ""
		}
		if early > waitStep {
			early = waitStep
		}
		sleep(early)
	}
}
