// Package diag defines the common request/response channel that every
// transport implements (K-Line KWP2000, ISO 15765 CAN, ELM327, J2534,
// DoIP, VW TP 2.0...) together with the application-layer helpers that
// are shared by KWP2000 (ISO 14230-3), UDS (ISO 14229) and OBD-II
// (SAE J1979): negative-response handling, "response pending" waits,
// collecting answers from several ECUs.
//
// Because all of those protocols reply with SID+0x40 or 7F SID NRC, a
// single helper drives them all. That is what lets the rest of the
// program treat a 1998 K-Line ECU and a 2020 CAN ECU the same way.
package diag

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Message is one application-layer message.
type Message struct {
	Src  uint32    // sender: ECU address, CAN id or logical address
	Dst  uint32    // receiver
	Data []byte    // starts with the service or response identifier
	Time time.Time // when it was received
}

// Channel is a diagnostic link to one ECU (physical addressing) or to all
// ECUs (functional addressing).
type Channel interface {
	// Send transmits a request. data starts with the service id.
	Send(ctx context.Context, data []byte) error
	// Recv waits up to timeout for the next message from the vehicle.
	Recv(ctx context.Context, timeout time.Duration) (Message, error)
	// Info describes the link.
	Info() ChannelInfo
	// Close ends the session (stop communication where applicable).
	Close() error
}

// ChannelInfo describes an open channel for the user interface and the
// journal.
type ChannelInfo struct {
	Protocol   string            `json:"protocol"`       // human readable name
	ProtoID    string            `json:"protoId"`        // machine id, see package vehicle
	Adapter    string            `json:"adapter"`        // adapter description
	Target     uint32            `json:"target"`         // physical target address / CAN id
	Tester     uint32            `json:"tester"`         // tester address / response CAN id
	Functional bool              `json:"functional"`     // broadcast to all ECUs
	Baud       int               `json:"baud,omitempty"` // K-Line baud or CAN bit rate
	KeyBytes   []byte            `json:"keyBytes,omitempty"`
	CAN        bool              `json:"can"` // message format follows ISO 15765
	Extra      map[string]string `json:"extra,omitempty"`
}

// Errors shared by all transports.
var (
	ErrTimeout   = errors.New("no response from the vehicle (timeout)")
	ErrClosed    = errors.New("diagnostic channel is closed")
	ErrNotActive = errors.New("not connected to a vehicle")
)

// IsTimeout reports whether err is (or wraps) ErrTimeout.
func IsTimeout(err error) bool { return errors.Is(err, ErrTimeout) }

// NegativeResponse is returned when the ECU answers 7F SID NRC.
type NegativeResponse struct {
	Service byte
	Code    byte
}

func (e *NegativeResponse) Error() string {
	return fmt.Sprintf("ECU refused %s (0x%02X): %s (NRC 0x%02X)",
		ServiceName(e.Service), e.Service, NRCName(e.Code), e.Code)
}

// IsNRC reports whether err is a negative response with the given code.
func IsNRC(err error, code byte) bool {
	var nr *NegativeResponse
	return errors.As(err, &nr) && nr.Code == code
}

// Common negative response codes.
const (
	NRCGeneralReject          = 0x10
	NRCServiceNotSupported    = 0x11
	NRCSubFunctionNotSupp     = 0x12
	NRCIncorrectLength        = 0x13
	NRCBusyRepeatRequest      = 0x21
	NRCConditionsNotCorrect   = 0x22
	NRCRequestSequenceError   = 0x24
	NRCRequestOutOfRange      = 0x31
	NRCSecurityAccessDenied   = 0x33
	NRCInvalidKey             = 0x35
	NRCExceededAttempts       = 0x36
	NRCTimeDelayNotExpired    = 0x37
	NRCResponsePending        = 0x78
	NRCServiceNotInSession    = 0x7F
	NRCSubFunctionNotInSesion = 0x7E
)

// Timing controls how long the application layer waits.
type Timing struct {
	P2          time.Duration // wait for the first response
	P2Ext       time.Duration // wait after "response pending" (NRC 0x78)
	BusyRetries int           // repeats after NRC 0x21
	BusyDelay   time.Duration // pause before repeating
}

// DefaultTiming suits every transport: the transports enforce their own
// strict bus timing, this is only the application-level patience, which
// includes adapter and USB latency.
func DefaultTiming() Timing {
	return Timing{P2: 1500 * time.Millisecond, P2Ext: 6 * time.Second, BusyRetries: 3, BusyDelay: 300 * time.Millisecond}
}

// matches reports whether resp answers req (positive or negative).
func matches(req, resp []byte) bool {
	if len(req) == 0 || len(resp) == 0 {
		return false
	}
	if resp[0] == 0x7F {
		return len(resp) >= 2 && resp[1] == req[0]
	}
	return resp[0] == req[0]|0x40
}

// Request sends req and waits for the matching answer, handling
// "response pending" and "busy, repeat request". It returns the positive
// response data (starting with SID+0x40) or a *NegativeResponse.
func Request(ctx context.Context, ch Channel, req []byte, t Timing) ([]byte, error) {
	m, err := RequestMsg(ctx, ch, req, t)
	if err != nil {
		return nil, err
	}
	return m.Data, nil
}

// RequestMsg is Request returning the whole message (with the sender).
func RequestMsg(ctx context.Context, ch Channel, req []byte, t Timing) (Message, error) {
	if t.P2 == 0 {
		t = DefaultTiming()
	}
	retries := t.BusyRetries
	for {
		if err := ch.Send(ctx, req); err != nil {
			return Message{}, err
		}
		m, err := awaitAnswer(ctx, ch, req, t)
		if err == nil {
			return m, nil
		}
		if IsNRC(err, NRCBusyRepeatRequest) && retries > 0 {
			retries--
			select {
			case <-ctx.Done():
				return Message{}, ctx.Err()
			case <-time.After(t.BusyDelay):
			}
			continue
		}
		return Message{}, err
	}
}

func awaitAnswer(ctx context.Context, ch Channel, req []byte, t Timing) (Message, error) {
	deadline := time.Now().Add(t.P2)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return Message{}, ErrTimeout
		}
		m, err := ch.Recv(ctx, remaining)
		if err != nil {
			return Message{}, err
		}
		if !matches(req, m.Data) {
			continue // unsolicited or stale message
		}
		if m.Data[0] == 0x7F {
			code := byte(0)
			if len(m.Data) >= 3 {
				code = m.Data[2]
			}
			if code == NRCResponsePending {
				deadline = time.Now().Add(t.P2Ext)
				continue
			}
			return m, &NegativeResponse{Service: req[0], Code: code}
		}
		return m, nil
	}
}

// RequestAll sends a functionally addressed request and collects every
// answer that arrives until no new message has come for quiet (or the
// first-response timeout expires). Negative answers are returned as
// messages too; the caller decides what to do with them.
func RequestAll(ctx context.Context, ch Channel, req []byte, first, quiet time.Duration) ([]Message, error) {
	if err := ch.Send(ctx, req); err != nil {
		return nil, err
	}
	var out []Message
	pending := map[uint32]bool{}
	deadline := time.Now().Add(first)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		m, err := ch.Recv(ctx, remaining)
		if err != nil {
			if IsTimeout(err) {
				break
			}
			if len(out) > 0 {
				return out, nil
			}
			return nil, err
		}
		if !matches(req, m.Data) {
			continue
		}
		if m.Data[0] == 0x7F && len(m.Data) >= 3 && m.Data[2] == NRCResponsePending {
			pending[m.Src] = true
			deadline = time.Now().Add(DefaultTiming().P2Ext)
			continue
		}
		delete(pending, m.Src)
		out = append(out, m)
		if len(pending) == 0 {
			deadline = time.Now().Add(quiet)
		}
	}
	if len(out) == 0 {
		return nil, ErrTimeout
	}
	return out, nil
}
