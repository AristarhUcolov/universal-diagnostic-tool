package elm327

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
)

// Answer is one message returned by an emulator backend.
type Answer struct {
	Src  uint32 // CAN id (7E8) or K-Line source address (0x10)
	Data []byte
}

// Backend executes the requests an emulated ELM327 receives.
type Backend interface {
	// Transact sends one request. header is the value set with ATSH
	// (nil for the OBD default, i.e. a functional request).
	Transact(ctx context.Context, proto byte, header []byte, data []byte) ([]Answer, error)
	// Protocol returns the protocol to report when the client asks for
	// automatic selection.
	Protocol() byte
	// Voltage returns the supply voltage for ATRV.
	Voltage() float64
}

// Emulator speaks the ELM327 command language on a byte stream.
type Emulator struct {
	be       Backend
	Version  string
	Desc     string
	echo     bool
	linefeed bool
	spaces   bool
	headers  bool
	auto     bool
	proto    byte
	header   []byte
	last     string
}

// NewEmulator creates an emulator with ELM327 v1.5 defaults.
func NewEmulator(be Backend) *Emulator {
	e := &Emulator{be: be, Version: "ELM327 v1.5", Desc: "Universal Diagnostic Tool"}
	e.reset()
	return e
}

func (e *Emulator) reset() {
	e.echo, e.linefeed, e.spaces, e.headers = true, true, true, false
	e.auto, e.proto, e.header = true, '0', nil
}

// Serve processes commands until the stream ends or ctx is cancelled.
func (e *Emulator) Serve(ctx context.Context, rw io.ReadWriter) error {
	r := bufio.NewReader(rw)
	var line []byte
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		b, err := r.ReadByte()
		if err != nil {
			return err
		}
		if b == '\n' {
			continue
		}
		if b != '\r' {
			line = append(line, b)
			continue
		}
		cmd := string(line)
		line = line[:0]
		out := e.Process(ctx, cmd)
		if _, err := io.WriteString(rw, out); err != nil {
			return err
		}
	}
}

func (e *Emulator) eol() string {
	if e.linefeed {
		return "\r\n"
	}
	return "\r"
}

// Process handles one command line and returns the full output
// including the prompt.
func (e *Emulator) Process(ctx context.Context, raw string) string {
	cmd := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(raw), " ", ""))
	var sb strings.Builder
	if e.echo {
		sb.WriteString(raw + e.eol())
	}
	if cmd == "" {
		cmd = e.last
	}
	e.last = cmd
	var lines []string
	if strings.HasPrefix(cmd, "AT") {
		lines = e.at(cmd[2:])
	} else if strings.HasPrefix(cmd, "ST") {
		lines = []string{"?"}
	} else {
		lines = e.data(ctx, cmd)
	}
	for _, l := range lines {
		sb.WriteString(l + e.eol())
	}
	sb.WriteString(e.eol() + ">")
	return sb.String()
}

func (e *Emulator) at(c string) []string {
	switch {
	case c == "Z" || c == "WS":
		e.reset()
		return []string{"", e.Version}
	case c == "D":
		e.reset()
		return []string{"OK"}
	case c == "I":
		return []string{e.Version}
	case c == "@1":
		return []string{e.Desc}
	case c == "RV":
		return []string{fmt.Sprintf("%.1fV", e.be.Voltage())}
	case c == "E0" || c == "E1":
		e.echo = c == "E1"
		return []string{"OK"}
	case c == "L0" || c == "L1":
		e.linefeed = c == "L1"
		return []string{"OK"}
	case c == "S0" || c == "S1":
		e.spaces = c == "S1"
		return []string{"OK"}
	case c == "H0" || c == "H1":
		e.headers = c == "H1"
		return []string{"OK"}
	case strings.HasPrefix(c, "SP") || strings.HasPrefix(c, "TP"):
		p := strings.TrimPrefix(c[2:], "A")
		if len(p) != 1 {
			return []string{"?"}
		}
		e.proto = p[0]
		e.auto = p[0] == '0' || strings.HasPrefix(c[2:], "A")
		return []string{"OK"}
	case c == "DP":
		name := ProtocolName(e.current())
		if e.auto {
			return []string{"AUTO, " + name}
		}
		return []string{name}
	case c == "DPN":
		p := string(e.current())
		if e.auto {
			return []string{"A" + p}
		}
		return []string{p}
	case strings.HasPrefix(c, "SH"):
		v := c[2:]
		if len(v)%2 == 1 {
			v = "0" + v // "7E0" is written with three digits
		}
		h, err := hexutil.Parse(v)
		if err != nil {
			return []string{"?"}
		}
		e.header = h
		return []string{"OK"}
	case c == "PC" || c == "FI" || c == "SI" || c == "MA":
		return []string{"OK"}
	}
	// Everything else (timeouts, flow control, filters, CAN options) is
	// accepted and ignored: the backend handles the transport itself.
	return []string{"OK"}
}

func (e *Emulator) current() byte {
	if e.proto == '0' || e.auto {
		return e.be.Protocol()
	}
	return e.proto
}

func (e *Emulator) data(ctx context.Context, cmd string) []string {
	// A trailing digit after the request is the "number of responses"
	// hint used by some apps (010C1); strip it.
	if len(cmd)%2 == 1 {
		cmd = cmd[:len(cmd)-1]
	}
	data, err := hexutil.Parse(cmd)
	if err != nil || len(data) == 0 {
		return []string{"?"}
	}
	proto := e.current()
	var lines []string
	if e.auto && e.proto == '0' {
		lines = append(lines, "SEARCHING...")
		e.proto = proto
	}
	tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	answers, err := e.be.Transact(tctx, proto, e.header, data)
	if err != nil || len(answers) == 0 {
		return append(lines, "NO DATA")
	}
	for _, a := range answers {
		lines = append(lines, e.format(proto, a)...)
	}
	return lines
}

func (e *Emulator) hex(b []byte) string {
	if e.spaces {
		return hexutil.Format(b)
	}
	return hexutil.Compact(b)
}

// format renders an answer the way an ELM327 prints it.
func (e *Emulator) format(proto byte, a Answer) []string {
	if !IsCAN(proto) {
		if !e.headers {
			return []string{e.hex(a.Data)}
		}
		var head []byte
		switch proto {
		case '4', '5':
			head = []byte{0x80 | byte(len(a.Data)), 0xF1, byte(a.Src)}
		case '1':
			head = []byte{0x41, 0x6B, byte(a.Src)}
		default:
			head = []byte{0x48, 0x6B, byte(a.Src)}
		}
		frame := append(head, a.Data...)
		var cs byte
		for _, v := range frame {
			cs += v
		}
		return []string{e.hex(append(frame, cs))}
	}
	id := fmt.Sprintf("%03X", a.Src)
	if Is29Bit(proto) || a.Src > 0x7FF {
		b := []byte{byte(a.Src >> 24), byte(a.Src >> 16), byte(a.Src >> 8), byte(a.Src)}
		id = e.hex(b)
	}
	sep := ""
	if e.spaces {
		sep = " "
	}
	if len(a.Data) <= 7 {
		if !e.headers {
			return []string{e.hex(a.Data)}
		}
		frame := append([]byte{byte(len(a.Data))}, a.Data...)
		for len(frame) < 8 {
			frame = append(frame, 0x00)
		}
		return []string{id + sep + e.hex(frame)}
	}
	var out []string
	if !e.headers {
		// ELM327 multi-frame format: total length, then numbered lines.
		out = append(out, fmt.Sprintf("%03X", len(a.Data)))
		chunks := [][]byte{a.Data[:min(6, len(a.Data))]}
		for i := 6; i < len(a.Data); i += 7 {
			chunks = append(chunks, a.Data[i:min(i+7, len(a.Data))])
		}
		for i, ch := range chunks {
			out = append(out, strconv.FormatInt(int64(i%16), 16)+":"+sep+e.hex(ch))
		}
		return out
	}
	ff := append([]byte{0x10 | byte(len(a.Data)>>8), byte(len(a.Data))}, a.Data[:6]...)
	out = append(out, id+sep+e.hex(ff))
	seq := byte(1)
	for i := 6; i < len(a.Data); i += 7 {
		cf := append([]byte{0x20 | seq&0x0F}, a.Data[i:min(i+7, len(a.Data))]...)
		for len(cf) < 8 {
			cf = append(cf, 0x00)
		}
		out = append(out, id+sep+e.hex(cf))
		seq++
	}
	return out
}
