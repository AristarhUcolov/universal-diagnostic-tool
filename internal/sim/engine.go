// Package sim simulates a vehicle: a virtual K-Line and a virtual CAN
// bus with control units that answer like real ones (Bosch M1.5.4 /
// Январь over KWP2000, Микас-7.1 over the legacy protocol, a generic
// OBD-II/UDS engine on CAN, a VAG KW1281 unit, a J1939 truck engine).
// Everything above the bus - framing, initialisation, transport layers
// and the user interface - runs exactly as with a real car, so the
// simulator serves both as a test bench and as a demo mode.
package sim

import (
	"math"
	"math/rand"
	"sync"
	"time"
)

// Fault is a simulated trouble code.
type Fault struct {
	Code    string // SAE code (P0134)
	Mikas   byte   // numeric code reported by legacy GAZ controllers
	VAG     uint16 // KW1281 five-digit code
	Active  bool   // present now
	Pending bool   // not yet confirmed
}

// Engine models a petrol engine with an oxygen sensor so that the
// simulated control units return plausible, changing values.
type Engine struct {
	mu       sync.Mutex
	ignOn    time.Time
	running  bool
	runSince time.Time
	revUntil time.Time
	revLevel float64
	driving  bool
	coolant0 float64
	faults   []Fault
	frozen   *State // freeze frame captured with the first fault
	VIN      string
	rng      *rand.Rand
	cleared  time.Time
}

// State is a snapshot of the simulated engine.
type State struct {
	Running         bool
	RunTime         float64 // s since start
	RPM             float64
	DesiredIdleRPM  float64
	CoolantC        float64
	IntakeC         float64
	AmbientC        float64
	SpeedKmh        float64
	ThrottlePct     float64
	PedalPct        float64
	MAFkgh          float64
	MAPkPa          float64
	BaroKPa         float64
	O2V             float64
	O2V2            float64
	STFT            float64 // %
	LTFT            float64 // %
	BatteryV        float64
	AdvanceDeg      float64
	InjMs           float64
	FuelLh          float64
	LoadPct         float64
	IACSteps        float64
	DesiredIACSteps float64
	CyclicAirMg     float64
	AFR             float64
	ClosedLoop      bool
	Idle            bool
	Knock           bool
	FuelLevelPct    float64
	OilC            float64
	DistanceKm      float64 // since faults were cleared
	TimeSinceClear  float64 // minutes
	WarmUps         float64
	CatTempC        float64
	FuelPressureKPa float64
	TripFuel100     float64 // l/100 km
}

// NewEngine returns a running, cold engine with two stored faults.
func NewEngine() *Engine {
	now := time.Now()
	return &Engine{
		ignOn: now, running: true, runSince: now, coolant0: 18,
		VIN: "XTH31100041234567",
		rng: rand.New(rand.NewSource(now.UnixNano())),
		faults: []Fault{
			{Code: "P0134", Mikas: 35, VAG: 16518, Active: false},
			{Code: "P0443", Mikas: 175, VAG: 16827, Active: true},
		},
		cleared: now.Add(-36 * time.Hour),
	}
}

// SetRunning starts or stops the engine (ignition stays on).
func (e *Engine) SetRunning(on bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if on && !e.running {
		e.runSince = time.Now()
	}
	e.running = on
}

// Running reports whether the engine runs.
func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

// Rev opens the throttle for d (a quick blip of the pedal).
func (e *Engine) Rev(d time.Duration, level float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if level <= 0 {
		level = 0.3
	}
	e.revUntil = time.Now().Add(d)
	e.revLevel = level
}

// SetDriving switches a gentle drive cycle on or off.
func (e *Engine) SetDriving(on bool) {
	e.mu.Lock()
	e.driving = on
	e.mu.Unlock()
}

// Faults returns a copy of the fault list.
func (e *Engine) Faults() []Fault {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]Fault(nil), e.faults...)
}

// AddFault injects a fault.
func (e *Engine) AddFault(f Fault) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.faults {
		if e.faults[i].Code == f.Code {
			e.faults[i] = f
			return
		}
	}
	if len(e.faults) == 0 {
		s := e.stateLocked()
		e.frozen = &s
	}
	e.faults = append(e.faults, f)
}

// ClearFaults erases stored faults; active ones come back at once, like
// in a real control unit.
func (e *Engine) ClearFaults() {
	e.mu.Lock()
	defer e.mu.Unlock()
	var keep []Fault
	for _, f := range e.faults {
		if f.Active {
			f.Pending = true
			keep = append(keep, f)
		}
	}
	e.faults = keep
	e.frozen = nil
	e.cleared = time.Now()
}

// FreezeFrame returns the state captured when the first fault was set.
func (e *Engine) FreezeFrame() (State, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.frozen == nil {
		if len(e.faults) == 0 {
			return State{}, false
		}
		s := e.stateLocked()
		s.RPM, s.CoolantC, s.SpeedKmh, s.LoadPct = 2150, 86, 54, 41
		e.frozen = &s
	}
	return *e.frozen, true
}

// State returns the current values.
func (e *Engine) State() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stateLocked()
}

func (e *Engine) stateLocked() State {
	now := time.Now()
	var s State
	s.BaroKPa = 99
	s.AmbientC = 16
	s.FuelLevelPct = 63
	s.TimeSinceClear = now.Sub(e.cleared).Minutes()
	s.WarmUps = 7
	s.DistanceKm = 412 + now.Sub(e.cleared).Hours()*0.5
	noise := func(a float64) float64 { return (e.rng.Float64()*2 - 1) * a }
	if !e.running {
		s.BatteryV = 12.4 + noise(0.02)
		s.CoolantC = e.coolant0
		s.IntakeC = s.AmbientC
		s.O2V = 0.45
		s.MAPkPa = s.BaroKPa
		s.IACSteps, s.DesiredIACSteps = 82, 82
		s.OilC = e.coolant0
		s.CatTempC = 40
		return s
	}
	t := now.Sub(e.runSince).Seconds()
	s.Running = true
	s.RunTime = t
	// Coolant warms towards 90 °C with a 2-minute time constant.
	s.CoolantC = 90 - (90-e.coolant0)*math.Exp(-t/120)
	s.OilC = 90 - (90-e.coolant0)*math.Exp(-t/200)
	s.IntakeC = s.AmbientC + 12*(1-math.Exp(-t/300))
	cold := clamp((60-s.CoolantC)/60, 0, 1)
	s.DesiredIdleRPM = math.Round((850+350*cold)/10) * 10
	throttle := 0.0
	if now.Before(e.revUntil) {
		throttle = 100 * e.revLevel
	}
	if e.driving {
		phase := math.Mod(t, 90) / 90
		throttle = math.Max(throttle, 18+14*math.Sin(phase*2*math.Pi))
		s.SpeedKmh = math.Max(0, 55+35*math.Sin(phase*2*math.Pi-1))
	}
	s.ThrottlePct = throttle
	s.PedalPct = throttle
	s.Idle = throttle < 1 && s.SpeedKmh < 1
	rpm := s.DesiredIdleRPM + 12*math.Sin(t*2.7) + noise(6)
	if !s.Idle {
		rpm = 900 + throttle*75
		if e.driving {
			rpm = 1300 + s.SpeedKmh*28 + throttle*15
		}
	}
	s.RPM = math.Max(0, rpm)
	load := 0.22 + throttle/100*0.7
	s.LoadPct = load * 100
	s.MAFkgh = 0.0105*s.RPM*(0.85+load) + noise(0.1)
	s.MAPkPa = 30 + throttle*0.65
	s.CyclicAirMg = s.MAFkgh * 1e6 / (s.RPM * 60 * 2)
	s.ClosedLoop = s.CoolantC > 40 && t > 25
	if s.ClosedLoop {
		// Narrow-band sensor switching around 1 Hz.
		sw := math.Sin(t * 2 * math.Pi * 1.1)
		s.O2V = 0.45 + 0.38*math.Copysign(math.Pow(math.Abs(sw), 0.4), sw)
		s.STFT = 3.5 * sw
		s.LTFT = 2.3
	} else {
		s.O2V = 0.45 + noise(0.01)
	}
	s.O2V2 = 0.68 + 0.03*math.Sin(t*0.3)
	s.BatteryV = 14.1 + noise(0.05)
	s.AdvanceDeg = 12 + throttle*0.2 + noise(0.5)
	s.InjMs = 2.4 + load*3.5 + cold*0.8
	s.FuelLh = 0.75 + throttle*0.09 + s.SpeedKmh*0.05
	if s.SpeedKmh > 3 {
		s.TripFuel100 = s.FuelLh / s.SpeedKmh * 100
	}
	s.AFR = 14.7 * (1 + s.STFT/100)
	s.DesiredIACSteps = math.Round(30 + 40*cold)
	s.IACSteps = s.DesiredIACSteps + math.Round(noise(1))
	s.CatTempC = 250 + 400*(1-math.Exp(-t/90)) + throttle*3
	s.FuelPressureKPa = 300
	s.Knock = throttle > 60 && e.rng.Float64() < 0.1
	return s
}

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}
