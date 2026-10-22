// Package uds implements the client side of Unified Diagnostic Services
// (ISO 14229-1), the diagnostic application layer of modern vehicles on
// CAN (ISO 15765), DoIP (ISO 13400) and FlexRay/Ethernet gateways.
package uds

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/dtc"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
)

// Session types.
const (
	SessionDefault     = 0x01
	SessionProgramming = 0x02
	SessionExtended    = 0x03
)

// Client issues UDS requests on a channel.
type Client struct {
	Ch     diag.Channel
	Timing diag.Timing
	// P2Star is the ECU's announced extended timeout (from the session
	// response); informational.
	P2Star time.Duration
}

// NewClient creates a client.
func NewClient(ch diag.Channel) *Client {
	return &Client{Ch: ch, Timing: diag.DefaultTiming()}
}

func (c *Client) request(ctx context.Context, req ...byte) ([]byte, error) {
	return diag.Request(ctx, c.Ch, req, c.Timing)
}

// Session switches the diagnostic session and returns the timing
// parameters the ECU reports.
func (c *Client) Session(ctx context.Context, session byte) (p2, p2star time.Duration, err error) {
	resp, err := c.request(ctx, 0x10, session)
	if err != nil {
		return 0, 0, err
	}
	if len(resp) >= 6 {
		p2 = time.Duration(binary.BigEndian.Uint16(resp[2:])) * time.Millisecond
		p2star = time.Duration(binary.BigEndian.Uint16(resp[4:])) * 10 * time.Millisecond
		c.P2Star = p2star
	}
	return p2, p2star, nil
}

// Reset restarts the ECU (1 hard, 2 key off/on, 3 soft).
func (c *Client) Reset(ctx context.Context, kind byte) error {
	_, err := c.request(ctx, 0x11, kind)
	return err
}

// TesterPresent keeps a non-default session alive. With suppress the
// ECU sends no answer.
func (c *Client) TesterPresent(ctx context.Context, suppress bool) error {
	if suppress {
		return c.Ch.Send(ctx, []byte{0x3E, 0x80})
	}
	_, err := c.request(ctx, 0x3E, 0x00)
	return err
}

// ReadDID reads one data identifier.
func (c *Client) ReadDID(ctx context.Context, did uint16) ([]byte, error) {
	resp, err := c.request(ctx, 0x22, byte(did>>8), byte(did))
	if err != nil {
		return nil, err
	}
	if len(resp) < 3 || binary.BigEndian.Uint16(resp[1:]) != did {
		return nil, fmt.Errorf("uds: answer for wrong identifier: % X", resp)
	}
	return resp[3:], nil
}

// WriteDID writes a data identifier (usually needs an extended session
// and security access).
func (c *Client) WriteDID(ctx context.Context, did uint16, data []byte) error {
	req := append([]byte{0x2E, byte(did >> 8), byte(did)}, data...)
	_, err := diag.Request(ctx, c.Ch, req, c.Timing)
	return err
}

// ReadDTCs reads trouble codes whose status matches mask (0x08 confirmed,
// 0x04 pending, 0xFF all) with sub-function 0x02.
func (c *Client) ReadDTCs(ctx context.Context, mask byte) ([]dtc.DTC, byte, error) {
	resp, err := c.request(ctx, 0x19, 0x02, mask)
	if err != nil {
		return nil, 0, err
	}
	if len(resp) < 3 {
		return nil, 0, fmt.Errorf("uds: short DTC answer % X", resp)
	}
	avail := resp[2]
	var out []dtc.DTC
	for i := 3; i+3 < len(resp); i += 4 {
		d := DecodeDTC(resp[i:i+3], resp[i+3])
		out = append(out, d)
	}
	return out, avail, nil
}

// DecodeDTC converts a three-byte ISO 14229 DTC (SAE code + failure
// type byte) and its status.
func DecodeDTC(code []byte, status byte) dtc.DTC {
	d := dtc.DTC{Code: dtc.SAE(code[0], code[1]), Raw: fmt.Sprintf("%02X%02X%02X %02X", code[0], code[1], code[2], status), Protocol: "UDS"}
	if code[2] != 0 {
		d.Extra = map[string]string{"ftb": fmt.Sprintf("%02X", code[2])}
		d.Detail = dtc.FailureType(code[2])
	}
	dtc.ApplyUDSStatus(&d, status)
	return d
}

// ClearDTCs clears a DTC group (0xFFFFFF = all).
func (c *Client) ClearDTCs(ctx context.Context, group uint32) error {
	_, err := c.request(ctx, 0x14, byte(group>>16), byte(group>>8), byte(group))
	return err
}

// KeyFunc computes the security key for a seed. Manufacturers keep these
// algorithms secret; the program ships none for real vehicles and
// expects the user to provide one (see package seedkey).
type KeyFunc func(level byte, seed []byte) ([]byte, error)

// SecurityAccess unlocks the given level (odd number: 0x01, 0x03...).
func (c *Client) SecurityAccess(ctx context.Context, level byte, key KeyFunc) error {
	resp, err := c.request(ctx, 0x27, level)
	if err != nil {
		return err
	}
	seed := resp[2:]
	allZero := true
	for _, b := range seed {
		if b != 0 {
			allZero = false
		}
	}
	if allZero {
		return nil // already unlocked
	}
	if key == nil {
		return fmt.Errorf("uds: security access level 0x%02X needs a key algorithm (seed % X)", level, seed)
	}
	k, err := key(level, seed)
	if err != nil {
		return err
	}
	_, err = c.request(ctx, append([]byte{0x27, level + 1}, k...)...)
	return err
}

// Routine starts (1), stops (2) or reads results (3) of a routine.
func (c *Client) Routine(ctx context.Context, kind byte, id uint16, data []byte) ([]byte, error) {
	req := append([]byte{0x31, kind, byte(id >> 8), byte(id)}, data...)
	resp, err := diag.Request(ctx, c.Ch, req, c.Timing)
	if err != nil {
		return nil, err
	}
	if len(resp) < 4 {
		return nil, nil
	}
	return resp[4:], nil
}

// IOControl controls an input/output: 0 return control, 1 reset to
// default, 2 freeze, 3 short-term adjustment with state.
func (c *Client) IOControl(ctx context.Context, did uint16, param byte, state []byte) ([]byte, error) {
	req := append([]byte{0x2F, byte(did >> 8), byte(did), param}, state...)
	resp, err := diag.Request(ctx, c.Ch, req, c.Timing)
	if err != nil {
		return nil, err
	}
	return resp[3:], nil
}

// ReadMemory reads memory with addressAndLengthFormatIdentifier alfid
// (e.g. 0x24: 4-byte address, 2-byte size).
func (c *Client) ReadMemory(ctx context.Context, addr uint32, size uint16) ([]byte, error) {
	req := []byte{0x23, 0x24, byte(addr >> 24), byte(addr >> 16), byte(addr >> 8), byte(addr), byte(size >> 8), byte(size)}
	resp, err := diag.Request(ctx, c.Ch, req, c.Timing)
	if err != nil {
		return nil, err
	}
	return resp[1:], nil
}

// ControlDTCSetting turns DTC recording on (1) or off (2).
func (c *Client) ControlDTCSetting(ctx context.Context, on bool) error {
	t := byte(0x02)
	if on {
		t = 0x01
	}
	_, err := c.request(ctx, 0x85, t)
	return err
}

// DIDNames gives readable names for the standard identifiers of ISO
// 14229-1 annex C.
var DIDNames = map[uint16]i18n.Text{
	0xF180: i18n.T("Boot software identification", "Идентификация загрузчика"),
	0xF181: i18n.T("Application software identification", "Идентификация программы"),
	0xF182: i18n.T("Application data identification", "Идентификация данных"),
	0xF183: i18n.T("Boot software fingerprint", "Отпечаток загрузчика"),
	0xF184: i18n.T("Application software fingerprint", "Отпечаток программы"),
	0xF185: i18n.T("Application data fingerprint", "Отпечаток данных"),
	0xF186: i18n.T("Active diagnostic session", "Активная диагностическая сессия"),
	0xF187: i18n.T("Manufacturer spare part number", "Номер запчасти производителя"),
	0xF188: i18n.T("Manufacturer ECU software number", "Номер ПО производителя"),
	0xF189: i18n.T("Manufacturer ECU software version", "Версия ПО производителя"),
	0xF18A: i18n.T("System supplier identifier", "Поставщик системы"),
	0xF18B: i18n.T("ECU manufacturing date", "Дата изготовления блока"),
	0xF18C: i18n.T("ECU serial number", "Серийный номер блока"),
	0xF18E: i18n.T("Manufacturer kit assembly part number", "Номер сборки производителя"),
	0xF190: i18n.T("VIN", "VIN"),
	0xF191: i18n.T("Manufacturer ECU hardware number", "Номер аппаратной части"),
	0xF192: i18n.T("Supplier ECU hardware number", "Номер аппаратной части поставщика"),
	0xF193: i18n.T("Supplier ECU hardware version", "Версия аппаратной части поставщика"),
	0xF194: i18n.T("Supplier ECU software number", "Номер ПО поставщика"),
	0xF195: i18n.T("Supplier ECU software version", "Версия ПО поставщика"),
	0xF196: i18n.T("Exhaust regulation / type approval number", "Номер экологического одобрения"),
	0xF197: i18n.T("System name or engine type", "Название системы / тип двигателя"),
	0xF198: i18n.T("Repair shop code / tester serial", "Код мастерской / серийный номер тестера"),
	0xF199: i18n.T("Programming date", "Дата программирования"),
	0xF19D: i18n.T("ECU installation date", "Дата установки блока"),
	0xF19E: i18n.T("ODX file", "Файл ODX"),
}

// IdentDIDs is the order in which identification is read.
var IdentDIDs = []uint16{0xF190, 0xF187, 0xF18A, 0xF191, 0xF192, 0xF194, 0xF195, 0xF197, 0xF18C, 0xF189, 0xF199, 0xF19E, 0xF186}
