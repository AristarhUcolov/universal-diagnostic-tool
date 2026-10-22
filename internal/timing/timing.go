// Package timing provides the millisecond-accurate delays that K-Line
// initialisation needs (25 ms wake-up pulses, 200 ms bits at 5 baud).
// The Go scheduler's time.Sleep may overshoot by a millisecond or more,
// so Sleep finishes the last stretch with a short busy wait on the
// monotonic clock.
package timing

import (
	"sync"
	"time"
)

// spinThreshold is how much of the delay is spent busy-waiting.
const spinThreshold = 2 * time.Millisecond

// Sleep blocks for d with sub-millisecond accuracy. It burns CPU for at
// most spinThreshold, so it is meant for protocol timing, not for long
// waits.
func Sleep(d time.Duration) {
	if d <= 0 {
		return
	}
	deadline := time.Now().Add(d)
	if d > spinThreshold {
		time.Sleep(d - spinThreshold)
	}
	for time.Now().Before(deadline) {
	}
}

// Until blocks until t (with the same accuracy as Sleep).
func Until(t time.Time) { Sleep(time.Until(t)) }

var (
	hrMu    sync.Mutex
	hrCount int
)

// HighResolution asks the OS for a 1 ms scheduler tick while K-Line
// communication is active and returns a function that releases the
// request. Calls nest.
func HighResolution() (release func()) {
	hrMu.Lock()
	if hrCount == 0 {
		beginPeriod()
	}
	hrCount++
	hrMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			hrMu.Lock()
			hrCount--
			if hrCount == 0 {
				endPeriod()
			}
			hrMu.Unlock()
		})
	}
}
