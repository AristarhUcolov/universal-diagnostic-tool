//go:build windows

package j2534

import (
	"context"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
)

type passThruMsg struct {
	ProtocolID     uint32
	RxStatus       uint32
	TxFlags        uint32
	Timestamp      uint32
	DataSize       uint32
	ExtraDataIndex uint32
	Data           [4128]byte
}

type sConfig struct {
	Parameter uint32
	Value     uint32
}

type sConfigList struct {
	NumOfParams uint32
	ConfigPtr   *sConfig
}

type sByteArray struct {
	NumOfBytes uint32
	BytePtr    *byte
}

// ListDevices reads the installed interfaces from the registry (both the
// 64-bit and the 32-bit view).
func ListDevices() ([]Device, error) {
	var out []Device
	seen := map[string]bool{}
	for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
		root, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\PassThruSupport.04.04`, registry.READ|view)
		if err != nil {
			continue
		}
		names, _ := root.ReadSubKeyNames(-1)
		for _, n := range names {
			k, err := registry.OpenKey(root, n, registry.READ|view)
			if err != nil {
				continue
			}
			d := Device{Protocols: map[string]bool{}}
			d.Name, _, _ = k.GetStringValue("Name")
			d.Vendor, _, _ = k.GetStringValue("Vendor")
			d.Library, _, _ = k.GetStringValue("FunctionLibrary")
			if d.Name == "" {
				d.Name = n
			}
			for _, p := range []string{"CAN", "ISO15765", "ISO9141", "ISO14230", "J1850VPW", "J1850PWM", "SCI_A_ENGINE", "SW_CAN_PS", "CAN_FD_PS"} {
				if v, _, err := k.GetIntegerValue(p); err == nil && v != 0 {
					d.Protocols[p] = true
				}
			}
			k.Close()
			if d.Library == "" || seen[strings.ToLower(d.Library)] {
				continue
			}
			seen[strings.ToLower(d.Library)] = true
			d.Arch, d.Usable, d.Reason = checkArch(d.Library)
			out = append(out, d)
		}
		root.Close()
	}
	return out, nil
}

// checkArch compares the DLL's architecture with this program's.
func checkArch(path string) (string, bool, string) {
	f, err := pe.Open(path)
	if err != nil {
		return "?", false, "driver DLL not found: " + path
	}
	defer f.Close()
	arch := "?"
	switch f.Machine {
	case pe.IMAGE_FILE_MACHINE_I386:
		arch = "x86"
	case pe.IMAGE_FILE_MACHINE_AMD64:
		arch = "x64"
	case pe.IMAGE_FILE_MACHINE_ARM64:
		arch = "arm64"
	}
	want := map[string]string{"386": "x86", "amd64": "x64", "arm64": "arm64"}[runtime.GOARCH]
	if arch != want {
		return arch, false, fmt.Sprintf("the driver is %s; use the %s build of this program (udt-windows-%s.exe)", arch, arch,
			map[string]string{"x86": "386", "x64": "amd64", "arm64": "arm64"}[arch])
	}
	return arch, true, ""
}

// worker runs every DLL call on one locked OS thread: several J2534
// drivers keep per-thread state.
type worker struct{ ch chan func() }

func newWorker() *worker {
	w := &worker{ch: make(chan func())}
	go func() {
		runtime.LockOSThread()
		for f := range w.ch {
			f()
		}
	}()
	return w
}

func (w *worker) do(f func()) {
	done := make(chan struct{})
	w.ch <- func() { f(); close(done) }
	<-done
}

// API is a loaded J2534 driver with one open device.
type API struct {
	dll      *windows.LazyDLL
	w        *worker
	dev      uint32
	name     string
	log      journal.Logger
	procs    map[string]*windows.LazyProc
	Firmware string
	DLLVer   string
	APIVer   string
}

// Open loads the driver and opens its device.
func Open(d Device, log journal.Logger) (*API, error) {
	if !d.Usable && d.Reason != "" {
		return nil, fmt.Errorf("j2534: %s: %s", d.Name, d.Reason)
	}
	a := &API{dll: windows.NewLazyDLL(d.Library), w: newWorker(), name: d.Name, log: journal.Or(log), procs: map[string]*windows.LazyProc{}}
	if err := a.dll.Load(); err != nil {
		return nil, fmt.Errorf("j2534: cannot load %s: %w", d.Library, err)
	}
	for _, n := range []string{"PassThruOpen", "PassThruClose", "PassThruConnect", "PassThruDisconnect", "PassThruReadMsgs",
		"PassThruWriteMsgs", "PassThruStartMsgFilter", "PassThruStopMsgFilter", "PassThruIoctl", "PassThruReadVersion", "PassThruGetLastError"} {
		p := a.dll.NewProc(n)
		if p.Find() != nil {
			if n == "PassThruOpen" || n == "PassThruClose" {
				return nil, fmt.Errorf("j2534: %s does not export %s (not a J2534 v04.04 driver?)", d.Library, n)
			}
			continue
		}
		a.procs[n] = p
	}
	var st uint32
	a.w.do(func() {
		r, _, _ := a.procs["PassThruOpen"].Call(0, uintptr(unsafe.Pointer(&a.dev)))
		st = uint32(r)
	})
	if st != statusNoError {
		return nil, a.err("PassThruOpen", st)
	}
	if p := a.procs["PassThruReadVersion"]; p != nil {
		var fw, dll, api [80]byte
		a.w.do(func() {
			p.Call(uintptr(a.dev), uintptr(unsafe.Pointer(&fw[0])), uintptr(unsafe.Pointer(&dll[0])), uintptr(unsafe.Pointer(&api[0])))
		})
		a.Firmware, a.DLLVer, a.APIVer = cstr(fw[:]), cstr(dll[:]), cstr(api[:])
	}
	journal.Infof(a.log, "j2534", "%s opened (firmware %s, DLL %s, API %s)", d.Name, a.Firmware, a.DLLVer, a.APIVer)
	return a, nil
}

func cstr(b []byte) string {
	if i := strings.IndexByte(string(b), 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(string(b))
}

func (a *API) err(fn string, st uint32) error {
	e := &Error{Func: fn, Status: st}
	if p := a.procs["PassThruGetLastError"]; p != nil && st != errTimeout && st != errBufferEmpty {
		var buf [80]byte
		a.w.do(func() { p.Call(uintptr(unsafe.Pointer(&buf[0]))) })
		e.Detail = cstr(buf[:])
	}
	return e
}

func (a *API) call(fn string, args ...uintptr) uint32 {
	p := a.procs[fn]
	if p == nil {
		return 0x01 // not supported
	}
	var st uint32
	a.w.do(func() {
		r, _, _ := p.Call(args...)
		st = uint32(r)
	})
	return st
}

// Name returns the device name.
func (a *API) Name() string { return a.name }

// Voltage reads the battery voltage on OBD pin 16.
func (a *API) Voltage() (float64, error) {
	var mv uint32
	st := a.call("PassThruIoctl", uintptr(a.dev), ioctlReadVBatt, 0, uintptr(unsafe.Pointer(&mv)))
	if st != statusNoError {
		return 0, a.err("READ_VBATT", st)
	}
	return float64(mv) / 1000, nil
}

// Close closes the device.
func (a *API) Close() error {
	st := a.call("PassThruClose", uintptr(a.dev))
	close(a.w.ch)
	if st != statusNoError {
		return a.err("PassThruClose", st)
	}
	return nil
}

// channel is an open protocol channel.
type channel struct {
	a     *API
	id    uint32
	proto uint32
	flags uint32
}

func (a *API) connect(proto, flags, baud uint32) (*channel, error) {
	var id uint32
	st := a.call("PassThruConnect", uintptr(a.dev), uintptr(proto), uintptr(flags), uintptr(baud), uintptr(unsafe.Pointer(&id)))
	if st != statusNoError {
		return nil, a.err("PassThruConnect", st)
	}
	return &channel{a: a, id: id, proto: proto, flags: flags}, nil
}

func (c *channel) setConfig(params ...uint32) error {
	list := make([]sConfig, 0, len(params)/2)
	for i := 0; i+1 < len(params); i += 2 {
		list = append(list, sConfig{Parameter: params[i], Value: params[i+1]})
	}
	cl := sConfigList{NumOfParams: uint32(len(list)), ConfigPtr: &list[0]}
	st := c.a.call("PassThruIoctl", uintptr(c.id), ioctlSetConfig, uintptr(unsafe.Pointer(&cl)), 0)
	runtime.KeepAlive(&cl)
	runtime.KeepAlive(list)
	if st != statusNoError {
		return c.a.err("SET_CONFIG", st)
	}
	return nil
}

func (c *channel) msg(data []byte, txFlags uint32) *passThruMsg {
	m := &passThruMsg{ProtocolID: c.proto, TxFlags: txFlags, DataSize: uint32(len(data))}
	copy(m.Data[:], data)
	return m
}

func (c *channel) filter(ftype uint32, mask, pattern, flow []byte, txFlags uint32) error {
	var fid uint32
	mm, pm := c.msg(mask, txFlags), c.msg(pattern, txFlags)
	var fp uintptr
	var fm *passThruMsg
	if flow != nil {
		fm = c.msg(flow, txFlags)
		fp = uintptr(unsafe.Pointer(fm))
	}
	st := c.a.call("PassThruStartMsgFilter", uintptr(c.id), uintptr(ftype), uintptr(unsafe.Pointer(mm)), uintptr(unsafe.Pointer(pm)), fp, uintptr(unsafe.Pointer(&fid)))
	// The DLL receives raw addresses; keep the Go objects alive until it
	// has returned.
	runtime.KeepAlive(mm)
	runtime.KeepAlive(pm)
	runtime.KeepAlive(fm)
	if st != statusNoError {
		return c.a.err("PassThruStartMsgFilter", st)
	}
	return nil
}

func (c *channel) write(data []byte, txFlags uint32, timeout time.Duration) error {
	m := c.msg(data, txFlags)
	n := uint32(1)
	st := c.a.call("PassThruWriteMsgs", uintptr(c.id), uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(&n)), uintptr(timeout/time.Millisecond))
	runtime.KeepAlive(m)
	if st != statusNoError {
		return c.a.err("PassThruWriteMsgs", st)
	}
	return nil
}

// read returns the next received (not transmitted) message.
func (c *channel) read(timeout time.Duration) (*passThruMsg, error) {
	deadline := time.Now().Add(timeout)
	for {
		m := &passThruMsg{}
		n := uint32(1)
		wait := time.Until(deadline)
		if wait < 0 {
			wait = 0
		}
		st := c.a.call("PassThruReadMsgs", uintptr(c.id), uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(&n)), uintptr(wait/time.Millisecond))
		if st != statusNoError && !(st == errTimeout && n > 0) {
			return nil, c.a.err("PassThruReadMsgs", st)
		}
		if n == 0 {
			if time.Now().After(deadline) {
				return nil, &Error{Func: "PassThruReadMsgs", Status: errTimeout}
			}
			continue
		}
		if m.RxStatus&(rxTxMsgType|rxStartOfMessage|rxTxIndication) != 0 {
			continue
		}
		return m, nil
	}
}

func (c *channel) close() error {
	st := c.a.call("PassThruDisconnect", uintptr(c.id))
	if st != statusNoError {
		return c.a.err("PassThruDisconnect", st)
	}
	return nil
}

// ISO15765 is an ISO-TP channel run by the interface (diag.Channel).
type ISO15765 struct {
	c          *channel
	tx         uint32
	ext        bool
	functional bool
	baud       int
	log        journal.Logger
}

// OpenISO15765 opens CAN diagnostics. For functional OBD requests pass
// tx 0x7DF (or 0x18DB33F1) and the answering ids in rx.
func (a *API) OpenISO15765(baud int, ext bool, tx uint32, rx []uint32, flow func(uint32) uint32, functional bool) (*ISO15765, error) {
	flags := uint32(0)
	if ext {
		flags |= flagCAN29Bit
	}
	c, err := a.connect(ProtoISO15765, flags, uint32(baud))
	if err != nil {
		return nil, err
	}
	_ = c.setConfig(cfgLoopback, 0, cfgISO15765BS, 0, cfgISO15765STm, 0)
	id := func(v uint32) []byte { b := make([]byte, 4); binary.BigEndian.PutUint32(b, v); return b }
	for _, r := range rx {
		if err := c.filter(filterFlow, id(0xFFFFFFFF), id(r), id(flow(r)), flags); err != nil {
			c.close()
			return nil, err
		}
	}
	return &ISO15765{c: c, tx: tx, ext: ext, functional: functional, baud: baud, log: a.log}, nil
}

func (s *ISO15765) Send(ctx context.Context, data []byte) error {
	b := make([]byte, 4, 4+len(data))
	binary.BigEndian.PutUint32(b, s.tx)
	flags := uint32(flagISO15765FramePad)
	if s.ext {
		flags |= flagCAN29Bit
	}
	journal.Tx(s.log, "j2534", fmt.Sprintf("%X", s.tx), data, diag.ServiceName(data[0]))
	return s.c.write(append(b, data...), flags, time.Second)
}

func (s *ISO15765) Recv(ctx context.Context, timeout time.Duration) (diag.Message, error) {
	m, err := s.c.read(timeout)
	if err != nil {
		if IsTimeout(err) {
			return diag.Message{}, diag.ErrTimeout
		}
		return diag.Message{}, err
	}
	if m.DataSize < 5 {
		return diag.Message{}, diag.ErrTimeout
	}
	src := binary.BigEndian.Uint32(m.Data[:4])
	data := append([]byte(nil), m.Data[4:m.DataSize]...)
	journal.Rx(s.log, "j2534", fmt.Sprintf("%X", src), data, diag.ServiceName(data[0]))
	return diag.Message{Src: src, Dst: s.tx, Data: data, Time: time.Now()}, nil
}

func (s *ISO15765) Info() diag.ChannelInfo {
	bits := "11-bit"
	if s.ext {
		bits = "29-bit"
	}
	return diag.ChannelInfo{Protocol: fmt.Sprintf("ISO 15765 CAN %s, %d kbit/s via J2534", bits, s.baud/1000), ProtoID: "iso15765",
		Adapter: s.c.a.name, Target: s.tx, Functional: s.functional, Baud: s.baud, CAN: true}
}

func (s *ISO15765) Close() error { return s.c.close() }

// KLine is an ISO 9141 / ISO 14230 channel (diag.Channel). The interface
// performs the initialisation and adds checksums.
type KLine struct {
	c      *channel
	header []byte
	kb     []byte
	name   string
	log    journal.Logger
	j1850  bool
}

// OpenKLine connects ISO 14230 (fast or 5-baud init) or ISO 9141.
// header is the request header (e.g. 81 10 F1 or C1 33 F1); for
// 5-baud init initAddr is sent.
func (a *API) OpenKLine(proto uint32, baud int, header []byte, fast bool, initAddr byte) (*KLine, error) {
	c, err := a.connect(proto, 0, uint32(baud))
	if err != nil {
		return nil, err
	}
	_ = c.setConfig(cfgLoopback, 0, cfgDataRate, uint32(baud))
	if err := c.filter(filterPass, []byte{0}, []byte{0}, nil, 0); err != nil {
		c.close()
		return nil, err
	}
	k := &KLine{c: c, header: header, log: a.log}
	if fast {
		start := append(append([]byte(nil), header...), 0x81)
		start[0] = start[0]&0xC0 | 1
		in, out := c.msg(start, 0), &passThruMsg{}
		st := a.call("PassThruIoctl", uintptr(c.id), ioctlFastInit, uintptr(unsafe.Pointer(in)), uintptr(unsafe.Pointer(out)))
		runtime.KeepAlive(in)
		if st != statusNoError {
			c.close()
			return nil, a.err("FAST_INIT", st)
		}
		resp := out.Data[:out.DataSize]
		if i := indexByte(resp, 0xC1); i >= 0 && i+2 < len(resp) {
			k.kb = []byte{resp[i+1], resp[i+2]}
		}
		k.name = fmt.Sprintf("ISO 14230 KWP2000 (fast init) via J2534, key bytes % X", k.kb)
	} else {
		addr := []byte{initAddr}
		kb := make([]byte, 2)
		in := sByteArray{NumOfBytes: 1, BytePtr: &addr[0]}
		out := sByteArray{NumOfBytes: 2, BytePtr: &kb[0]}
		st := a.call("PassThruIoctl", uintptr(c.id), ioctlFiveBaudInit, uintptr(unsafe.Pointer(&in)), uintptr(unsafe.Pointer(&out)))
		runtime.KeepAlive(&in)
		runtime.KeepAlive(&out)
		runtime.KeepAlive(addr)
		runtime.KeepAlive(kb)
		if st != statusNoError {
			c.close()
			return nil, a.err("FIVE_BAUD_INIT", st)
		}
		k.kb = kb
		if proto == ProtoISO9141 {
			k.name = "ISO 9141-2 (5-baud init) via J2534"
		} else {
			k.name = "ISO 14230 KWP2000 (5-baud init) via J2534"
		}
	}
	return k, nil
}

// OpenJ1850 connects SAE J1850 VPW (proto 1, 10400) or PWM (proto 2,
// 41600) for OBD requests.
func (a *API) OpenJ1850(proto uint32) (*KLine, error) {
	baud, header := uint32(10400), []byte{0x68, 0x6A, 0xF1}
	name := "SAE J1850 VPW via J2534"
	if proto == ProtoJ1850PWM {
		baud, header, name = 41600, []byte{0x61, 0x6A, 0xF1}, "SAE J1850 PWM via J2534"
	}
	c, err := a.connect(proto, 0, baud)
	if err != nil {
		return nil, err
	}
	_ = c.setConfig(cfgLoopback, 0)
	if proto == ProtoJ1850PWM {
		addr := []byte{0x6B}
		in := sByteArray{NumOfBytes: 1, BytePtr: &addr[0]}
		a.call("PassThruIoctl", uintptr(c.id), ioctlAddToFunctLookup, uintptr(unsafe.Pointer(&in)), 0)
		runtime.KeepAlive(&in)
		runtime.KeepAlive(addr)
	}
	if err := c.filter(filterPass, []byte{0}, []byte{0}, nil, 0); err != nil {
		c.close()
		return nil, err
	}
	return &KLine{c: c, header: header, name: name, log: a.log, j1850: true}, nil
}

func indexByte(b []byte, v byte) int {
	for i, x := range b {
		if x == v {
			return i
		}
	}
	return -1
}

func (k *KLine) Send(ctx context.Context, data []byte) error {
	h := append([]byte(nil), k.header...)
	if !k.j1850 && len(h) > 0 && h[0]&0xC0 != 0 && len(data) <= 63 {
		h[0] = h[0]&0xC0 | byte(len(data))
	}
	journal.Tx(k.log, "j2534", "", append(h, data...), diag.ServiceName(data[0]))
	return k.c.write(append(h, data...), 0, time.Second)
}

func (k *KLine) Recv(ctx context.Context, timeout time.Duration) (diag.Message, error) {
	m, err := k.c.read(timeout)
	if err != nil {
		if IsTimeout(err) {
			return diag.Message{}, diag.ErrTimeout
		}
		return diag.Message{}, err
	}
	raw := m.Data[:m.DataSize]
	head := 3
	if !k.j1850 && len(raw) > 0 {
		head = 1
		if raw[0]&0xC0 != 0 {
			head = 3
		}
		if raw[0]&0x3F == 0 {
			head++
		}
	}
	if len(raw) <= head {
		return diag.Message{}, diag.ErrTimeout
	}
	src := uint32(0)
	if head >= 3 {
		src = uint32(raw[2])
	}
	data := append([]byte(nil), raw[head:]...)
	journal.Rx(k.log, "j2534", fmt.Sprintf("0x%02X", src), raw, diag.ServiceName(data[0]))
	return diag.Message{Src: src, Data: data, Time: time.Now()}, nil
}

func (k *KLine) Info() diag.ChannelInfo {
	functional := len(k.header) == 3 && (k.header[0]&0xC0 == 0xC0 || k.header[1] == 0x6A || k.header[1] == 0x33)
	target := uint32(0)
	if len(k.header) >= 2 {
		target = uint32(k.header[1])
	}
	return diag.ChannelInfo{Protocol: k.name, ProtoID: "j2534-kline", Adapter: k.c.a.name, Target: target,
		Tester: 0xF1, Functional: functional, KeyBytes: k.kb}
}

func (k *KLine) Close() error { return k.c.close() }

// CANBus is a raw CAN channel (can.Bus) for sniffing, J1939 and VW TP 2.0.
type CANBus struct {
	c    *channel
	baud int
}

// OpenCAN opens a raw CAN channel receiving all frames.
func (a *API) OpenCAN(baud int) (*CANBus, error) {
	c, err := a.connect(ProtoCAN, flagCANIDBoth, uint32(baud))
	if err != nil {
		c2, err2 := a.connect(ProtoCAN, 0, uint32(baud))
		if err2 != nil {
			return nil, err
		}
		c = c2
	}
	_ = c.setConfig(cfgLoopback, 0)
	zero := []byte{0, 0, 0, 0}
	if err := c.filter(filterPass, zero, zero, nil, 0); err != nil {
		c.close()
		return nil, err
	}
	_ = c.filter(filterPass, zero, zero, nil, flagCAN29Bit)
	return &CANBus{c: c, baud: baud}, nil
}

func (b *CANBus) Send(ctx context.Context, f can.Frame) error {
	d := make([]byte, 4, 4+int(f.Len))
	binary.BigEndian.PutUint32(d, f.ID)
	flags := uint32(0)
	if f.Ext {
		flags = flagCAN29Bit
	}
	return b.c.write(append(d, f.Payload()...), flags, time.Second)
}

func (b *CANBus) Recv(ctx context.Context, timeout time.Duration) (can.Frame, error) {
	m, err := b.c.read(timeout)
	if err != nil {
		if IsTimeout(err) {
			return can.Frame{}, can.ErrTimeout
		}
		return can.Frame{}, err
	}
	if m.DataSize < 4 {
		return can.Frame{}, can.ErrTimeout
	}
	f := can.NewFrame(binary.BigEndian.Uint32(m.Data[:4]), m.RxStatus&flagCAN29Bit != 0, m.Data[4:m.DataSize])
	f.Time = time.Now()
	return f, nil
}

func (b *CANBus) Info() can.BusInfo {
	return can.BusInfo{Name: b.c.a.name + " (J2534 CAN)", Bitrate: b.baud}
}

func (b *CANBus) Close() error { return b.c.close() }
