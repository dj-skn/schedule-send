// Package worker is what a job's timer runs: it re-checks the target, sends
// the payload, and confirms an agent actually took the prompt.
package worker

import (
	"fmt"
	"os"
)

// Fire runs the job with the given id and returns the exit status.
func Fire(id string) int {
	fmt.Fprintln(os.Stderr, "__fire is not implemented yet")
	return 1
}
