// Package kw1281 implements Volkswagen's KW1281 block protocol used by
// VW, Audi, Seat and Skoda control units on the K-Line until about 2005
// (and by some instrument clusters much later). After a 5-baud
// initialisation the control unit and the tester take turns sending
// blocks [length][counter][title][data...][0x03]; every byte except the
// final 0x03 is acknowledged by the receiver with its complement.
package kw1281

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/dtc"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/kline"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/timing"
)

// Block titles.
const (
	TitleReadROM      = 0x03
	TitleActuatorTest = 0x04
	TitleClearDTC     = 0x05
	TitleEnd          = 0x06
	TitleReadDTC      = 0x07
	TitleACK          = 0x09
	TitleNAK          = 0x0A
	TitleBasicSetting = 0x28
	TitleGroupRead    = 0x29
	TitleRawData      = 0x02
	TitleGroupResp    = 0xE7
	TitleBasicResp    = 0xF4
	TitleActuatorResp = 0xF5
	TitleASCII        = 0xF6
	TitleDTCResp      = 0xFC
)

// Block is one KW1281 block.
type Block struct {
	Counter byte
	Title   byte
	Data    []byte
}

// Errors.
var (
	ErrProtocol = errors.New("kw1281: protocol error")
	ErrNAK      = errors.New("kw1281: request not supported by the control unit")
)

// Conn is an open KW1281 session with one control unit.
type Conn struct {
	line    kline.Line
	addr    byte
	baud    int
	counter byte
	ID      []string // identification sent by the control unit after init
	log     journal.Logger
	// ByteTimeout is the wait for each byte or complement.
	ByteTimeout time.Duration
}

// Connect wakes the control unit at addr (e.g. 0x01 engine, 0x17
// instruments) and reads its identification. baud 0 tries 10400 and
// 9600, the two speeds nearly all units use.
func Connect(ctx context.Context, line kline.Line, addr byte, baud int, log journal.Logger) (*Conn, error) {
	bauds := []int{baud}
	if baud == 0 {
		bauds = []int{10400, 9600}
	}
	var lastErr error
	for i, b := range bauds {
		if i > 0 {
			// Give the unit time to give up on the previous attempt.
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2500 * time.Millisecond):
			}
		}
		c, err := connect(ctx, line, addr, b, log)
		if err == nil {
			return c, nil
		}
		lastErr = err
		journal.Warnf(log, "kw1281", "address 0x%02X at %d baud: %v", addr, b, err)
	}
	return nil, lastErr
}

func connect(ctx context.Context, line kline.Line, addr byte, baud int, log journal.Logger) (*Conn, error) {
	if err := line.SetBaud(baud); err != nil {
		return nil, err
	}
	r, err := kline.SlowInit(line, kline.OddParity(addr), false, log)
	if err != nil {
		return nil, err
	}
	if r.KeyWord() != 1281 {
		journal.Warnf(log, "kw1281", "key word %d is not 1281 (key bytes %02X %02X); the unit may speak KWP2000 instead", r.KeyWord(), r.KB1, r.KB2)
	}
	c := &Conn{line: line, addr: addr, baud: baud, log: journal.Or(log), ByteTimeout: 150 * time.Millisecond}
	// The unit now sends its identification as ASCII blocks, each
	// acknowledged by us, and finishes with an ACK block.
	for i := 0; i < 16; i++ {
		b, err := c.recvBlock(1500 * time.Millisecond)
		if err != nil {
			return nil, err
		}
		switch b.Title {
		case TitleASCII:
			if s := strings.TrimSpace(hexutil.ASCII(b.Data)); s != "" {
				c.ID = append(c.ID, s)
			}
			if err := c.sendBlock(TitleACK, nil); err != nil {
				return nil, err
			}
		case TitleACK:
			journal.Infof(c.log, "kw1281", "connected to 0x%02X at %d baud: %s", addr, baud, strings.Join(c.ID, " | "))
			return c, nil
		default:
			if err := c.sendBlock(TitleACK, nil); err != nil {
				return nil, err
			}
		}
	}
	return nil, fmt.Errorf("%w: identification did not end", ErrProtocol)
}

// Address returns the control unit address.
func (c *Conn) Address() byte { return c.addr }

// Baud returns the speed in use.
func (c *Conn) Baud() int { return c.baud }

func (c *Conn) readAck(timeout time.Duration) (byte, error) {
	b, err := c.line.RecvByte(timeout)
	if err != nil {
		return 0, err
	}
	timing.Sleep(time.Millisecond)
	if err := c.line.Send([]byte{^b}, 0); err != nil {
		return 0, err
	}
	return b, nil
}

func (c *Conn) recvBlock(first time.Duration) (Block, error) {
	n, err := c.readAck(first)
	if err != nil {
		return Block{}, fmt.Errorf("kw1281: no block from the control unit: %w", err)
	}
	if n < 3 {
		return Block{}, fmt.Errorf("%w: block length %d", ErrProtocol, n)
	}
	raw := []byte{n}
	body := make([]byte, 0, n)
	for i := 0; i < int(n)-1; i++ {
		b, err := c.readAck(c.ByteTimeout)
		if err != nil {
			return Block{}, fmt.Errorf("kw1281: block interrupted (% X): %w", raw, err)
		}
		raw = append(raw, b)
		body = append(body, b)
	}
	end, err := c.line.RecvByte(c.ByteTimeout)
	if err != nil {
		return Block{}, fmt.Errorf("kw1281: block end missing (% X): %w", raw, err)
	}
	raw = append(raw, end)
	if end != 0x03 {
		return Block{}, fmt.Errorf("%w: block end 0x%02X instead of 0x03", ErrProtocol, end)
	}
	blk := Block{Counter: body[0], Title: body[1], Data: body[2:]}
	c.counter = blk.Counter
	journal.Rx(c.log, "kw1281", fmt.Sprintf("0x%02X", c.addr), raw, titleName(blk.Title))
	return blk, nil
}

func (c *Conn) sendBlock(title byte, data []byte) error {
	// Leave the unit a moment after its block end.
	timing.Sleep(5 * time.Millisecond)
	c.counter++
	raw := make([]byte, 0, len(data)+4)
	raw = append(raw, byte(len(data)+3), c.counter, title)
	raw = append(raw, data...)
	raw = append(raw, 0x03)
	journal.Tx(c.log, "kw1281", fmt.Sprintf("0x%02X", c.addr), raw, titleName(title))
	for i, b := range raw {
		if err := c.line.Send([]byte{b}, 0); err != nil {
			return err
		}
		if i == len(raw)-1 {
			break
		}
		ack, err := c.line.RecvByte(c.ByteTimeout)
		if err != nil {
			return fmt.Errorf("kw1281: no acknowledge for byte %d of block: %w", i, err)
		}
		if ack != ^b {
			return fmt.Errorf("%w: acknowledge 0x%02X for byte 0x%02X", ErrProtocol, ack, b)
		}
		timing.Sleep(time.Millisecond)
	}
	return nil
}

// Exchange sends a block and returns the unit's answer block.
func (c *Conn) Exchange(ctx context.Context, title byte, data []byte) (Block, error) {
	if err := ctx.Err(); err != nil {
		return Block{}, err
	}
	if err := c.sendBlock(title, data); err != nil {
		return Block{}, err
	}
	return c.recvBlock(1500 * time.Millisecond)
}

// KeepAlive exchanges ACK blocks; the unit drops the session after about
// a second of silence.
func (c *Conn) KeepAlive(ctx context.Context) error {
	b, err := c.Exchange(ctx, TitleACK, nil)
	if err != nil {
		return err
	}
	if b.Title != TitleACK {
		return fmt.Errorf("%w: expected ACK, got 0x%02X", ErrProtocol, b.Title)
	}
	return nil
}

// Fault is one KW1281 fault record.
type Fault struct {
	Code   uint16
	Status byte
}

// ReadFaults reads the fault memory.
func (c *Conn) ReadFaults(ctx context.Context) ([]Fault, error) {
	b, err := c.Exchange(ctx, TitleReadDTC, nil)
	if err != nil {
		return nil, err
	}
	return c.collectFaults(ctx, b)
}

// ClearFaults erases the fault memory and returns what remains (faults
// that are still present come back immediately).
func (c *Conn) ClearFaults(ctx context.Context) ([]Fault, error) {
	b, err := c.Exchange(ctx, TitleClearDTC, nil)
	if err != nil {
		return nil, err
	}
	return c.collectFaults(ctx, b)
}

func (c *Conn) collectFaults(ctx context.Context, b Block) ([]Fault, error) {
	var out []Fault
	for i := 0; i < 32; i++ {
		switch b.Title {
		case TitleDTCResp:
			for j := 0; j+2 < len(b.Data); j += 3 {
				code := uint16(b.Data[j])<<8 | uint16(b.Data[j+1])
				st := b.Data[j+2]
				if code == 0xFFFF && st == 0x88 {
					continue // "no faults"
				}
				out = append(out, Fault{Code: code, Status: st})
			}
		case TitleACK:
			return out, nil
		case TitleNAK:
			return out, ErrNAK
		default:
			return out, fmt.Errorf("%w: unexpected block 0x%02X in fault list", ErrProtocol, b.Title)
		}
		var err error
		if b, err = c.Exchange(ctx, TitleACK, nil); err != nil {
			return out, err
		}
	}
	return out, nil
}

// ToDTC converts faults to the common representation.
func ToDTC(faults []Fault, source string) []dtc.DTC {
	out := make([]dtc.DTC, 0, len(faults))
	for _, f := range faults {
		text, sae := dtc.VAGCode(f.Code)
		d := dtc.DTC{Code: text, Raw: fmt.Sprintf("%04X %02X", f.Code, f.Status), Status: f.Status,
			Confirmed: true, Active: f.Status&0x80 == 0, Source: source, Protocol: "KW1281",
			Detail: dtc.VAGFaultType(f.Status)}
		if sae != "" {
			d.Extra = map[string]string{"sae": sae}
		}
		if f.Status&0x80 != 0 {
			d.Flags = append(d.Flags, dtc.FlagIntermittent)
		} else {
			d.Flags = append(d.Flags, dtc.FlagActive)
		}
		out = append(out, d)
	}
	return out
}

// ReadGroup reads a measuring value group (1..255).
func (c *Conn) ReadGroup(ctx context.Context, group byte) ([]Cell, error) {
	b, err := c.Exchange(ctx, TitleGroupRead, []byte{group})
	if err != nil {
		return nil, err
	}
	switch b.Title {
	case TitleGroupResp, TitleBasicResp:
		var cells []Cell
		for i := 0; i+2 < len(b.Data); i += 3 {
			cells = append(cells, DecodeCell(b.Data[i], b.Data[i+1], b.Data[i+2]))
		}
		return cells, nil
	case TitleRawData:
		return []Cell{{Formula: 0, Text: hexutil.Format(b.Data), Raw: append([]byte(nil), b.Data...)}}, nil
	case TitleNAK:
		return nil, ErrNAK
	}
	return nil, fmt.Errorf("%w: unexpected answer 0x%02X to group read", ErrProtocol, b.Title)
}

// Close ends the session.
func (c *Conn) Close() error {
	_ = c.sendBlock(TitleEnd, nil)
	return nil
}

func titleName(t byte) string {
	switch t {
	case TitleACK:
		return "ACK"
	case TitleNAK:
		return "NAK"
	case TitleASCII:
		return "identification"
	case TitleReadDTC:
		return "read faults"
	case TitleClearDTC:
		return "clear faults"
	case TitleDTCResp:
		return "fault list"
	case TitleGroupRead:
		return "read group"
	case TitleGroupResp:
		return "group values"
	case TitleEnd:
		return "end session"
	case TitleBasicSetting:
		return "basic setting"
	case TitleBasicResp:
		return "basic setting values"
	case TitleActuatorTest:
		return "output test"
	case TitleActuatorResp:
		return "output test answer"
	case TitleRawData:
		return "raw data"
	}
	return fmt.Sprintf("title 0x%02X", t)
}
