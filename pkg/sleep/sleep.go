// Package sleep is a simple utility to implement an exponential backoff sleep.
// It is intended to be used before external API calls, to pause before making
// API requests depending on the number of failures (failCount) encountered so far.
package sleep

import (
	"fmt"
	"math"
	"time"
)

// ExponentialBackOff will sleep for a minimum of 1 seconds, maxiumum of 4096 seconds.
// Sleep time increases exponentially with each API failure (2**failCount).
func ExponentialBackOff(failCount int) {

	fmt.Println("DEBUG: Number of failures encountered: ", failCount)
	if failCount > 12 {
		fmt.Println("DEBUG: Resetting failCount to prevent integer overflow and long sleep times")
		failCount = 12
	}

	sleeptime := int(math.Exp2(float64(failCount)))
	fmt.Println("DEBUG: Exponential backoff: sleeping", sleeptime, "seconds.")
	time.Sleep(time.Duration(sleeptime) * time.Second)
}
