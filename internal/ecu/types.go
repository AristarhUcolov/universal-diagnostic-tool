// Package ecu defines the protocol-independent view of a control unit
// that the user interface works with: identification, trouble codes,
// live parameters and output tests. Every protocol (OBD-II, KWP2000,
// UDS, KW1281, J1939, the legacy Russian protocol) provides a Handler,
// which is how the program "converts" between protocols: the same
// buttons work whatever the car speaks.
//
// The package also holds the data-driven definitions of manufacturer
// specific control units (JSON files) and the generic handlers that
// execute them.
package ecu

import (
	"context"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/dtc"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
)

// Param describes one live value.
type Param struct {
	ID       string    `json:"id"`
	Name     i18n.Text `json:"name"`
	Unit     string    `json:"unit,omitempty"`
	Group    string    `json:"group,omitempty"`
	Min      float64   `json:"min"`
	Max      float64   `json:"max"`
	Kind     string    `json:"kind"` // num, bool, text
	Digits   int       `json:"digits"`
	OBD      string    `json:"obd,omitempty"` // equivalent OBD-II PID for the ELM327 bridge
	Verified bool      `json:"verified"`
	Note     i18n.Text `json:"note,omitempty"`
}

// Value is one reading.
type Value struct {
	ID   string    `json:"id"`
	V    float64   `json:"v"`
	Text string    `json:"t,omitempty"` // text value (not localised)
	L    i18n.Text `json:"l,omitempty"` // localised text value
	OK   bool      `json:"ok"`
	Raw  string    `json:"raw,omitempty"`
}

// InfoItem is one line of identification.
type InfoItem struct {
	Key   string    `json:"key"`
	Name  i18n.Text `json:"name"`
	Value string    `json:"value"`
}

// Actuator is an output test or adjustment.
type Actuator struct {
	ID      string    `json:"id"`
	Name    i18n.Text `json:"name"`
	Kind    string    `json:"kind"` // toggle (on/off), pulse (one shot), value (set a number)
	Min     float64   `json:"min,omitempty"`
	Max     float64   `json:"max,omitempty"`
	Unit    string    `json:"unit,omitempty"`
	Warning i18n.Text `json:"warning,omitempty"`
	// Condition is what the test needs: "engine_off" or
	// "engine_running" (empty: either).
	Condition string `json:"condition,omitempty"`
	// TimeoutS is when a switched-on output is handed back to the
	// control unit automatically.
	TimeoutS int  `json:"timeoutS,omitempty"`
	Active   bool `json:"active,omitempty"`
}

// Description summarises a handler for the user interface.
type Description struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Protocol     string    `json:"protocol"`
	Address      string    `json:"address"`
	Family       string    `json:"family,omitempty"`     // DTC description family
	Definition   string    `json:"definition,omitempty"` // definition file id
	Status       string    `json:"status"`               // verified, partial, unverified, generic
	Notes        i18n.Text `json:"notes,omitempty"`
	Capabilities []string  `json:"capabilities"`
}

// Capability names.
const (
	CapDTC       = "dtc"
	CapClear     = "clear"
	CapLive      = "live"
	CapActuators = "actuators"
	CapRaw       = "raw"
	CapFreeze    = "freeze"
	CapReadiness = "readiness"
	CapBlocks    = "blocks"
	CapReset     = "reset"
)

// Resetter is implemented by handlers that can restart their control
// unit (ecuReset) and restore communication afterwards.
type Resetter interface {
	Reset(ctx context.Context) error
}

// ConditionError reports that an output test needs the engine stopped
// ("engine_off") or running ("engine_running") first. Unknown: the
// engine speed could not be read, so the condition could not be checked
// and the test was not started.
type ConditionError struct {
	Need    string
	Unknown bool
}

func (e *ConditionError) Error() string {
	switch {
	case e.Unknown:
		return "the engine speed could not be read, so the program cannot make sure the engine is in the state this test needs; the test was not started"
	case e.Need == "engine_off":
		return "stop the engine first: this test runs with the engine off"
	}
	return "start the engine first: this test needs a running engine"
}

// Code names the error for the interface: engine_off, engine_running or
// engine_unknown.
func (e *ConditionError) Code() string {
	if e.Unknown {
		return "engine_unknown"
	}
	return e.Need
}

// Handler is the uniform interface to one control unit.
type Handler interface {
	Describe() Description
	Identify(ctx context.Context) ([]InfoItem, error)
	ReadDTC(ctx context.Context) ([]dtc.DTC, error)
	ClearDTC(ctx context.Context) error
	Params() []Param
	Read(ctx context.Context, ids []string) ([]Value, error)
	Actuators() []Actuator
	Actuate(ctx context.Context, id, action string, value float64) (string, error)
	// Raw sends a request in the handler's native protocol and returns
	// the raw answer, for the terminal.
	Raw(ctx context.Context, req []byte) ([]byte, error)
	KeepAlive(ctx context.Context) error
	Close() error
}

// FreezeFrameReader is implemented by handlers that can read the values
// stored together with a trouble code.
type FreezeFrameReader interface {
	FreezeFrame(ctx context.Context) ([]Value, error)
}

// Monitor is one OBD readiness monitor.
type Monitor struct {
	ID       string    `json:"id"`
	Name     i18n.Text `json:"name"`
	Complete bool      `json:"complete"`
}

// ReadinessReader is implemented by OBD handlers.
type ReadinessReader interface {
	Readiness(ctx context.Context) (mil bool, count int, monitors []Monitor, err error)
}

// BlockReader gives raw access to data records (KWP2000 local
// identifiers, UDS identifiers) for the record explorer that helps to
// map unknown control units.
type BlockReader interface {
	Blocks() []string
	ReadBlock(ctx context.Context, id string) ([]byte, error)
}

// ParamIndex finds parameters by id.
func ParamIndex(ps []Param) map[string]Param {
	m := make(map[string]Param, len(ps))
	for _, p := range ps {
		m[p.ID] = p
	}
	return m
}
