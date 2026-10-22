package kline

import (
	"fmt"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/timing"
)

// SlowInitResult holds what the control unit sent during 5-baud init.
type SlowInitResult struct {
	Sync     byte // 0x55
	KB1, KB2 byte // key bytes
}

// KeyWord returns the ISO 14230 key word number built from the key bytes
// (2000-2031 for KWP2000, 1281 for VAG KW1281, 0x0808 style for ISO 9141).
func (r SlowInitResult) KeyWord() int {
	return int(r.KB2&0x7F)<<7 | int(r.KB1&0x7F)
}

// SlowInit performs the 5-baud initialisation common to ISO 9141-2,
// ISO 14230 (slow init) and KW1281: the address goes out at 5 baud, the
// control unit answers 0x55 and two key bytes at its own speed, the
// tester acknowledges with the inverted second key byte. When
// wantAddrEcho is set the control unit finishes with the inverted
// address (ISO 9141/14230); KW1281 units start sending blocks instead.
func SlowInit(line Line, addr byte, wantAddrEcho bool, log journal.Logger) (SlowInitResult, error) {
	var r SlowInitResult
	log = journal.Or(log)
	if err := line.FiveBaud(addr); err != nil {
		return r, err
	}
	var err error
	// W1: 20-300 ms until the sync byte.
	if r.Sync, err = line.RecvByte(320 * time.Millisecond); err != nil {
		return r, fmt.Errorf("k-line: no sync byte after 5-baud address 0x%02X (wrong address, no K-Line connection or ignition off): %w", addr, err)
	}
	if r.Sync != 0x55 {
		return r, fmt.Errorf("k-line: unexpected sync byte 0x%02X (expected 0x55) - the control unit may use a different baud rate than %d", r.Sync, line.Baud())
	}
	// W2, W3: 5-20 ms each.
	if r.KB1, err = line.RecvByte(40 * time.Millisecond); err != nil {
		return r, fmt.Errorf("k-line: key byte 1 missing: %w", err)
	}
	if r.KB2, err = line.RecvByte(40 * time.Millisecond); err != nil {
		return r, fmt.Errorf("k-line: key byte 2 missing: %w", err)
	}
	journal.Rx(log, "kline", fmt.Sprintf("0x%02X", addr), []byte{r.Sync, r.KB1, r.KB2}, "5-baud init: sync + key bytes")
	// W4: 25-50 ms before the tester's answer.
	timing.Sleep(30 * time.Millisecond)
	if err := line.Send([]byte{^r.KB2}, 0); err != nil {
		return r, err
	}
	if wantAddrEcho {
		b, err := line.RecvByte(60 * time.Millisecond)
		if err != nil {
			return r, fmt.Errorf("k-line: control unit did not confirm the init (inverted address missing): %w", err)
		}
		if b != ^addr {
			journal.Warnf(log, "kline", "init confirmation 0x%02X, expected 0x%02X", b, ^addr)
		}
	}
	journal.Infof(log, "kline", "5-baud init OK, key bytes %02X %02X (key word %d)", r.KB1, r.KB2, r.KeyWord())
	return r, nil
}
