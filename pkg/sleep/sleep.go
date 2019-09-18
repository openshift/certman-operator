// Package sleep is a simple utility to implement an exponential backoff sleep.
// It is intended to be used before external API calls, to pause before making
// API requests depending on the number of failures (failCount) encountered so far.
package sleep

import (
	"math"
	"time"
)

// ExponentialBackOff will sleep for a minimum of 2 seconds, maxiumum of 2 hours,
// depending on the number of seconds given as failCount (which represents the number
// of API failures a CertificateRequest has encountered).
// Sleep time increases by a power of 2 for each API failure.
// For example, a failCount of 1 sleeps for 2 seconds. A failCount of 2 sleeps for 4 seconds.
func ExponentialBackOff(failCount int) {
	// Sleeptime is a minimum of 2 seconds (1<<1), maximum of 2 hours (7200).
	sleeptime := math.Min(7200, float64(uint(1)<<uint(failCount)))
	println("Exponential backoff: sleeping", sleeptime, "seconds.")
	time.Sleep(time.Duration(sleeptime) * time.Second)
}
