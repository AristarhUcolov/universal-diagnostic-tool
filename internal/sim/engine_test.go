package sim

import (
	"testing"
	"time"
)

// The launch used to try the acceleration timer must look like a real
// car: from standstill, about 6 s to 60 km/h and 12 s to 100 km/h.
func TestLaunch(t *testing.T) {
	e := NewEngine()
	e.SetRunning(true)
	e.Launch()
	if v := e.State().SpeedKmh; v > 1 {
		t.Fatalf("speed right after the launch %.1f km/h", v)
	}
	at := func(sec float64) float64 {
		e.mu.Lock()
		e.launchAt = time.Now().Add(-time.Duration(sec * float64(time.Second)))
		e.mu.Unlock()
		return e.State().SpeedKmh
	}
	if v := at(5); v >= 60 {
		t.Errorf("60 km/h before 5 s (%.1f)", v)
	}
	if v := at(6.5); v < 60 {
		t.Errorf("below 60 km/h after 6.5 s (%.1f)", v)
	}
	if v := at(11); v >= 100 {
		t.Errorf("100 km/h before 11 s (%.1f)", v)
	}
	if v := at(13); v < 100 {
		t.Errorf("below 100 km/h after 13 s (%.1f)", v)
	}
	if v := at(60); v != 130 {
		t.Errorf("cruise %.1f km/h, want 130", v)
	}
	e.SetDriving(false)
	if v := e.State().SpeedKmh; v != 0 {
		t.Errorf("parked at %.1f km/h", v)
	}
}
