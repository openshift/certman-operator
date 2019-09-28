// Package sleep is a simple utility to implement an exponential backoff sleep.
// It is intended to be used before external API calls, to pause before making
// API requests depending on the number of failures (failCount) encountered so far.
package sleep

import (
	"fmt"
	"math"
	"time"
)

// ExponentialBackOff will sleep for a minimum of 1 seconds, maxiumum of 4096 seconds,
// depending on the number of seconds given as failCount (which represents the number
// of API failures a CertificateRequest has encountered).
// Sleep time increases by a power of 2 for each API failure.
// For example, a failCount of 1 sleeps for 2 seconds. A failCount of 2 sleeps for 4 seconds.
func ExponentialBackOff(failCount int) {

	fmt.Println("Number of failures encountered: ", failCount)
	if failCount > 12 {
		fmt.Println("Resetting failCount to prevent integer overflow and long sleep times")
		failCount = 12
	}

	sleeptime := int(math.Exp2(float64(failCount)))
	fmt.Println("Exponential backoff: sleeping", sleeptime, "seconds.")
	time.Sleep(time.Duration(sleeptime) * time.Second)
}
