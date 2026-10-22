package vehicle

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/can"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/doip"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/ecu"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/elm327"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/iso9141"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/isotp"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/j1939"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/j2534"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/kline"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/kvaser"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/kw1281"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/kwp2000"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/legacy"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/obd"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/pcan"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/rp1210"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/serial"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/sim"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/slcan"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/socketcan"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/tp20"
)

// Progress receives human-readable steps while connecting.
type Progress func(step string)

// Connect opens the adapter and finds the control units.
func Connect(ctx context.Context, cfg Config, log journal.Logger, progress Progress) (*Session, error) {
	if cfg.Profile == "" {
		cfg.Profile = ProfileAuto
	}
	if progress == nil {
		progress = func(string) {}
	}
	s := &Session{Config: cfg, Started: time.Now(), log: journal.Or(log)}
	step := func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		journal.Infof(s.log, "app", "%s", msg)
		progress(msg)
	}
	var err error
	switch cfg.Adapter {
	case AdapterSim:
		err = s.connectSim(ctx, step)
	case AdapterKKL:
		err = s.connectKKL(ctx, step)
	case AdapterELM, AdapterELMWiFi:
		err = s.connectELM(ctx, step)
	case AdapterJ2534:
		err = s.connectJ2534(ctx, step)
	case AdapterSLCAN, AdapterSocketCAN, AdapterPCAN, AdapterKvaser:
		err = s.connectCAN(ctx, step, nil)
	case AdapterRP1210:
		err = s.connectRP1210(ctx, step)
	case AdapterDoIP:
		err = s.connectDoIP(ctx, step)
	case AdapterHSFZ:
		err = s.connectHSFZ(ctx, step)
	default:
		err = fmt.Errorf("unknown adapter %q", cfg.Adapter)
	}
	if err == nil && len(s.ECUs()) == 0 && cfg.Profile != ProfileMonitor {
		err = errors.New("no control unit answered")
	}
	if err != nil {
		s.Close()
		journal.Errorf(s.log, "app", "connection failed: %v", err)
		return nil, err
	}
	s.Touch()
	s.startKeepAlive()
	step("connected: %d control unit(s)", len(s.ECUs()))
	return s, nil
}

type stepFunc func(format string, args ...any)

// hexValue parses a hex string with a default.
func hexValue(s string, def uint32) uint32 {
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(s, "0x"), "0X"))
	if s == "" {
		return def
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return def
	}
	return uint32(v)
}

// attempt runs connection strategies until one finds a control unit.
func attempt(ctx context.Context, tries []func() error) error {
	var errs []string
	for _, t := range tries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := t(); err == nil {
			return nil
		} else {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) == 0 {
		return errors.New("this profile is not supported by the selected adapter")
	}
	return fmt.Errorf("no answer: %s", strings.Join(errs, "; "))
}

func (s *Session) definition(id string) *ecu.Definition {
	if id == "" {
		return nil
	}
	d, _ := ecu.Builtin().Get(id)
	return d
}

// ---------- K-Line ----------

func (s *Session) connectKKL(ctx context.Context, step stepFunc) error {
	opt := kline.DefaultOptions()
	switch s.Config.Echo {
	case "on":
		opt.Echo = kline.EchoOn
	case "off":
		opt.Echo = kline.EchoOff
	}
	if s.Config.DTR != nil {
		opt.DTR = *s.Config.DTR
	}
	if s.Config.RTS != nil {
		opt.RTS = *s.Config.RTS
	}
	if s.Config.InitMode == "baud360" {
		opt.Init = kline.InitBaud360
	}
	step("opening K-Line cable on %s", s.Config.Port)
	line, err := kline.OpenSerial(s.Config.Port, opt, s.log)
	if err != nil {
		return err
	}
	s.closers = append(s.closers, line)
	s.Adapter = "K-Line cable on " + s.Config.Port
	return s.klineProfiles(ctx, line, step)
}

func (s *Session) klineProfiles(ctx context.Context, line kline.Line, step stepFunc) error {
	cfg := s.Config
	ruKWP := func() error { return s.tryKWP(ctx, line, step, byte(hexValue(cfg.Address, 0x10)), "ru-kwp") }
	customKWP := func() error { return s.tryKWP(ctx, line, step, byte(hexValue(cfg.Address, 0x10)), "") }
	legacyTry := func() error { return s.tryLegacy(ctx, line, step) }
	obdTry := func() error { return s.tryOBDKLine(ctx, line, step) }
	vag := func() error { return s.tryKW1281(ctx, line, step, byte(hexValue(cfg.Address, 0x01))) }
	var tries []func() error
	switch cfg.Profile {
	case ProfileRuKWP:
		tries = []func() error{ruKWP}
	case ProfileRuLegacy:
		tries = []func() error{legacyTry}
	case ProfileRuAuto:
		tries = []func() error{ruKWP, legacyTry, obdTry}
	case ProfileOBD:
		tries = []func() error{obdTry}
	case ProfileVAG:
		tries = []func() error{vag}
	case ProfileKWP:
		tries = []func() error{customKWP}
	case ProfileAuto:
		tries = []func() error{obdTry, ruKWP, legacyTry, vag}
	}
	return attempt(ctx, tries)
}

func (s *Session) tryKWP(ctx context.Context, line kline.Line, step stepFunc, addr byte, profile string) error {
	init := kwp2000.InitFast
	if s.Config.Init == "slow" {
		init = kwp2000.InitSlow
	}
	step("KWP2000 %s, address 0x%02X", init, addr)
	p3 := 55 * time.Millisecond
	if profile == "ru-kwp" {
		p3 = 100 * time.Millisecond
	}
	link, err := kwp2000.Open(ctx, line, kwp2000.Config{Target: addr, Tester: 0xF1, Init: init, StartComm: true, P3Min: p3}, s.log)
	if err != nil {
		return err
	}
	def := s.definition(s.Config.ECU)
	if def == nil {
		step("identifying the control unit")
		def = ecu.Builtin().MatchKWP(ctx, link, "kwp2000", profile, s.log)
	}
	if def != nil && def.Link.P3MinMs > 0 {
		link.SetP3Min(time.Duration(def.Link.P3MinMs) * time.Millisecond)
	}
	s.addECU(ecu.NewDefHandler(link, def, "kwp2000", fmt.Sprintf("KWP2000 control unit 0x%02X", addr)))
	return nil
}

func (s *Session) tryLegacy(ctx context.Context, line kline.Line, step stepFunc) error {
	step("Микас / Январь-4 protocol, 9600 baud")
	link, err := legacy.Open(line, 9600, s.log)
	if err != nil {
		return err
	}
	var v byte
	for i := 0; i < 2; i++ {
		if v, err = link.Version(ctx); err == nil {
			break
		}
	}
	if err != nil {
		return fmt.Errorf("legacy protocol: %w", err)
	}
	def := s.definition(s.Config.ECU)
	if def == nil {
		def = ecu.Builtin().MatchLegacy(v)
	}
	s.addECU(ecu.NewLegacyHandler(link, def, v, line.Name()))
	return nil
}

func (s *Session) tryOBDKLine(ctx context.Context, line kline.Line, step stepFunc) error {
	step("OBD-II on ISO 14230-4 (KWP2000 fast init)")
	link, err := kwp2000.Open(ctx, line, kwp2000.Config{Target: 0x33, Tester: 0xF1, Functional: true, Init: kwp2000.InitFast, StartComm: true}, s.log)
	if err == nil {
		h, herr := obd.NewHandler(ctx, link)
		if herr == nil {
			s.addECU(h)
			return nil
		}
		err = herr
	}
	step("OBD-II on ISO 9141-2 (5-baud init)")
	link2, err2 := iso9141.Open(ctx, line, s.log)
	if err2 == nil {
		h, herr := obd.NewHandler(ctx, link2)
		if herr == nil {
			s.addECU(h)
			return nil
		}
		err2 = herr
	}
	return fmt.Errorf("OBD-II: ISO 14230: %v; ISO 9141: %v", err, err2)
}

func (s *Session) tryKW1281(ctx context.Context, line kline.Line, step stepFunc, addr byte) error {
	step("VAG KW1281, module 0x%02X (5-baud init)", addr)
	baud := 0
	if s.Config.Baud == 9600 || s.Config.Baud == 10400 || s.Config.Baud == 4800 || s.Config.Baud == 1200 {
		baud = s.Config.Baud
	}
	c, err := kw1281.Connect(ctx, line, addr, baud, s.log)
	if err != nil {
		return err
	}
	s.addECU(ecu.NewKW1281Handler(c))
	return nil
}

// ---------- ELM327 ----------

func (s *Session) connectELM(ctx context.Context, step stepFunc) error {
	var dev *elm327.Device
	if s.Config.Adapter == AdapterELMWiFi {
		addr := s.Config.Port
		if addr == "" {
			addr = "192.168.0.10:35000"
		}
		step("connecting to Wi-Fi adapter %s", addr)
		p, err := elm327.DialTCP(addr, 5*time.Second)
		if err != nil {
			return err
		}
		if dev, err = elm327.Open(ctx, p, s.log); err != nil {
			p.Close()
			return err
		}
	} else {
		bauds := []int{38400, 9600, 115200, 57600, 230400, 500000}
		if s.Config.Baud > 0 {
			bauds = []int{s.Config.Baud}
		}
		var lastErr error
		for _, b := range bauds {
			step("ELM327 on %s at %d baud", s.Config.Port, b)
			p, err := serial.Open(s.Config.Port, serial.Config{Baud: b, DTR: true, RTS: true})
			if err != nil {
				return err
			}
			d, err := elm327.Open(ctx, p, s.log)
			if err == nil {
				dev = d
				break
			}
			p.Close()
			lastErr = err
		}
		if dev == nil {
			return fmt.Errorf("no ELM327 answered on %s: %v", s.Config.Port, lastErr)
		}
	}
	s.closers = append(s.closers, dev)
	s.Adapter = dev.Name()
	s.voltage = dev.Voltage
	cfg := s.Config
	switch cfg.Profile {
	case ProfileAuto, ProfileOBD:
		step("searching for the OBD-II protocol (ELM327 automatic)")
		ch, err := dev.Open(ctx, elm327.Spec{Proto: '0', Functional: true})
		if err != nil {
			return err
		}
		h, err := obd.NewHandler(ctx, ch)
		if err != nil {
			return err
		}
		s.addECU(h)
		return nil
	case ProfileRuKWP, ProfileKWP:
		addr := byte(hexValue(cfg.Address, 0x10))
		proto := byte('5')
		if cfg.Init == "slow" {
			proto = '4'
		}
		step("KWP2000 address 0x%02X through the ELM327", addr)
		ch, err := dev.Open(ctx, elm327.Spec{Proto: proto, Header: fmt.Sprintf("81 %02X F1", addr),
			InitAddr: fmt.Sprintf("%02X", addr), WakeMsg: fmt.Sprintf("81 %02X F1 3E", addr), FastInit: proto == '5',
			Name: fmt.Sprintf("ISO 14230 KWP2000, address 0x%02X", addr)})
		if err != nil {
			return err
		}
		if _, err := diag.Request(ctx, ch, []byte{0x3E, 0x01}, diag.DefaultTiming()); err != nil && !isNRC(err) {
			if _, err2 := diag.Request(ctx, ch, []byte{0x1A, 0x80}, diag.DefaultTiming()); err2 != nil && !isNRC(err2) {
				return fmt.Errorf("control unit 0x%02X does not answer through the ELM327: %v", addr, err)
			}
		}
		profile := ""
		if cfg.Profile == ProfileRuKWP {
			profile = "ru-kwp"
		}
		def := s.definition(cfg.ECU)
		if def == nil {
			def = ecu.Builtin().MatchKWP(ctx, ch, "kwp2000", profile, s.log)
		}
		s.addECU(ecu.NewDefHandler(ch, def, "kwp2000", fmt.Sprintf("KWP2000 control unit 0x%02X", addr)))
		return nil
	case ProfileUDS:
		tx, rx := hexValue(cfg.TxID, 0x7E0), hexValue(cfg.RxID, 0x7E8)
		proto := byte('6')
		ext := tx > 0x7FF
		switch {
		case ext && cfg.Bitrate == 250000:
			proto = '9'
		case ext:
			proto = '7'
		case cfg.Bitrate == 250000:
			proto = '8'
		}
		step("UDS %X/%X through the ELM327", tx, rx)
		ch, err := dev.Open(ctx, elm327.Spec{Proto: proto, Header: fmt.Sprintf("%X", tx), RxFilter: fmt.Sprintf("%X", rx), FlowHeader: fmt.Sprintf("%X", tx)})
		if err != nil {
			return err
		}
		return s.addUDS(ctx, ch, step)
	}
	return fmt.Errorf("the ELM327 cannot do profile %q: Микас-5.4/7.1 and VW KW1281 need a KKL cable, J1939 and TP 2.0 a CAN adapter", cfg.Profile)
}

func isNRC(err error) bool {
	var nr *diag.NegativeResponse
	return errors.As(err, &nr)
}

// addUDS probes a UDS control unit and adds it.
func (s *Session) addUDS(ctx context.Context, ch diag.Channel, step stepFunc) error {
	if _, err := diag.Request(ctx, ch, []byte{0x3E, 0x00}, diag.Timing{P2: time.Second, P2Ext: 3 * time.Second}); err != nil && !isNRC(err) {
		if _, err2 := diag.Request(ctx, ch, []byte{0x22, 0xF1, 0x90}, diag.DefaultTiming()); err2 != nil && !isNRC(err2) {
			return fmt.Errorf("UDS control unit does not answer: %v", err)
		}
	}
	def := s.definition(s.Config.ECU)
	s.addECU(ecu.NewDefHandler(ch, def, "uds", "UDS control unit"))
	return nil
}

// ---------- J2534 ----------

func (s *Session) connectJ2534(ctx context.Context, step stepFunc) error {
	devs, err := j2534.ListDevices()
	if err != nil {
		return err
	}
	var dev *j2534.Device
	for i := range devs {
		if s.Config.Port == "" || devs[i].Name == s.Config.Port || devs[i].Library == s.Config.Port {
			dev = &devs[i]
			break
		}
	}
	if dev == nil {
		return errors.New("no J2534 interface is installed (install the driver from the interface manufacturer)")
	}
	step("opening J2534 interface %s", dev.Name)
	api, err := j2534.Open(*dev, s.log)
	if err != nil {
		return err
	}
	s.closers = append(s.closers, api)
	s.Adapter = dev.Name + " (J2534)"
	s.voltage = func(context.Context) (float64, error) { return api.Voltage() }
	cfg := s.Config
	bitrates := []int{500000, 250000}
	if cfg.Bitrate > 0 {
		bitrates = []int{cfg.Bitrate}
	}
	obdCAN := func() error {
		for _, br := range bitrates {
			for _, ext := range []bool{false, true} {
				step("OBD-II on CAN %d kbit/s, %s identifiers", br/1000, map[bool]string{false: "11-bit", true: "29-bit"}[ext])
				fc := isotp.OBDFunctional(ext)
				ch, err := api.OpenISO15765(br, ext, fc.TxID, fc.RxIDs[:min(len(fc.RxIDs), 16)], fc.FlowTx, true)
				if err != nil {
					continue
				}
				h, err := obd.NewHandler(ctx, ch)
				if err == nil {
					s.addECU(h)
					return nil
				}
				ch.Close()
			}
		}
		return errors.New("no OBD-II answer on CAN")
	}
	obdK := func() error {
		step("OBD-II on ISO 14230-4 (fast init)")
		if ch, err := api.OpenKLine(j2534.ProtoISO14230, 10400, []byte{0xC1, 0x33, 0xF1}, true, 0x33); err == nil {
			if h, err := obd.NewHandler(ctx, ch); err == nil {
				s.addECU(h)
				return nil
			}
			ch.Close()
		}
		step("OBD-II on ISO 9141-2 (5-baud init)")
		if ch, err := api.OpenKLine(j2534.ProtoISO9141, 10400, []byte{0x68, 0x6A, 0xF1}, false, 0x33); err == nil {
			if h, err := obd.NewHandler(ctx, ch); err == nil {
				s.addECU(h)
				return nil
			}
			ch.Close()
		}
		for _, p := range []uint32{j2534.ProtoJ1850VPW, j2534.ProtoJ1850PWM} {
			step("OBD-II on SAE J1850 (%d)", p)
			if ch, err := api.OpenJ1850(p); err == nil {
				if h, err := obd.NewHandler(ctx, ch); err == nil {
					s.addECU(h)
					return nil
				}
				ch.Close()
			}
		}
		return errors.New("no OBD-II answer on K-Line or J1850")
	}
	switch cfg.Profile {
	case ProfileAuto, ProfileOBD:
		return attempt(ctx, []func() error{obdCAN, obdK})
	case ProfileRuKWP, ProfileKWP:
		addr := byte(hexValue(cfg.Address, 0x10))
		step("KWP2000 address 0x%02X through J2534", addr)
		ch, err := api.OpenKLine(j2534.ProtoISO14230, 10400, []byte{0x81, addr, 0xF1}, cfg.Init != "slow", addr)
		if err != nil {
			return err
		}
		profile := ""
		if cfg.Profile == ProfileRuKWP {
			profile = "ru-kwp"
		}
		def := s.definition(cfg.ECU)
		if def == nil {
			def = ecu.Builtin().MatchKWP(ctx, ch, "kwp2000", profile, s.log)
		}
		s.addECU(ecu.NewDefHandler(ch, def, "kwp2000", fmt.Sprintf("KWP2000 control unit 0x%02X", addr)))
		return nil
	case ProfileUDS:
		tx, rx := hexValue(cfg.TxID, 0x7E0), hexValue(cfg.RxID, 0x7E8)
		ext := tx > 0x7FF
		ch, err := api.OpenISO15765(bitrates[0], ext, tx, []uint32{rx}, func(uint32) uint32 { return tx }, false)
		if err != nil {
			return err
		}
		return s.addUDS(ctx, ch, step)
	case ProfileTP20, ProfileJ1939, ProfileMonitor:
		return s.connectCAN(ctx, step, func(br int) (can.Bus, error) { return api.OpenCAN(br) })
	}
	return fmt.Errorf("J2534 cannot do profile %q (Микас and KW1281 need a KKL cable)", cfg.Profile)
}

// ---------- raw CAN ----------

func (s *Session) openCANBus(br int) (can.Bus, error) {
	switch s.Config.Adapter {
	case AdapterSLCAN:
		baud := s.Config.Baud
		if baud == 0 {
			baud = 115200
		}
		p, err := serial.Open(s.Config.Port, serial.Config{Baud: baud})
		if err != nil {
			return nil, err
		}
		b, err := slcan.Open(p, br, s.Config.Profile == ProfileMonitor, s.log)
		if err != nil {
			p.Close()
			return nil, err
		}
		return b, nil
	case AdapterSocketCAN:
		return socketcan.Open(s.Config.Port)
	case AdapterPCAN:
		ch := s.Config.Port
		if ch == "" {
			ch = "PCAN_USBBUS1"
		}
		return pcan.Open(ch, br)
	case AdapterKvaser:
		n, _ := strconv.Atoi(s.Config.Port)
		return kvaser.Open(n, br)
	}
	return nil, fmt.Errorf("adapter %s is not a CAN adapter", s.Config.Adapter)
}

func (s *Session) connectCAN(ctx context.Context, step stepFunc, open func(int) (can.Bus, error)) error {
	if open == nil {
		open = s.openCANBus
	}
	cfg := s.Config
	bitrates := []int{500000, 250000}
	if cfg.Profile == ProfileJ1939 {
		bitrates = []int{250000, 500000}
	}
	if cfg.Bitrate > 0 {
		bitrates = []int{cfg.Bitrate}
	}
	var errs []string
	for _, br := range bitrates {
		step("opening CAN at %d kbit/s", br/1000)
		bus, err := open(br)
		if err != nil {
			return err
		}
		mux := can.NewMux(bus, s.log)
		s.mux = mux
		if s.Adapter == "" {
			s.Adapter = bus.Info().Name
		}
		err = s.canProfiles(ctx, mux, br, step)
		if err == nil {
			return nil
		}
		errs = append(errs, fmt.Sprintf("%d kbit/s: %v", br/1000, err))
		s.mu.Lock()
		s.ecus = nil
		s.mu.Unlock()
		mux.Close()
		s.mux = nil
	}
	return errors.New(strings.Join(errs, "; "))
}

func (s *Session) canProfiles(ctx context.Context, mux *can.Mux, br int, step stepFunc) error {
	cfg := s.Config
	obdTry := func() error {
		for _, ext := range []bool{false, true} {
			step("OBD-II on CAN, %s identifiers", map[bool]string{false: "11-bit", true: "29-bit"}[ext])
			c := isotp.OBDFunctional(ext)
			c.Bitrate = br
			conn := isotp.New(mux, c, s.log)
			h, err := obd.NewHandler(ctx, conn)
			if err == nil {
				s.addECU(h)
				return nil
			}
			conn.Close()
		}
		return errors.New("no OBD-II answer")
	}
	j1939Try := func() error {
		step("listening for J1939 traffic")
		link := j1939.NewCANLink(mux, j1939.AddrTool1, s.log)
		_ = link.ClaimAddress(ctx)
		h := j1939.NewHandler(link)
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if h.Describe().Address != "" {
				s.addECU(h)
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}
		h.Close()
		return errors.New("no J1939 traffic")
	}
	switch cfg.Profile {
	case ProfileAuto:
		return attempt(ctx, []func() error{obdTry, j1939Try})
	case ProfileOBD:
		return obdTry()
	case ProfileUDS:
		tx, rx := hexValue(cfg.TxID, 0x7E0), hexValue(cfg.RxID, 0x7E8)
		c := isotp.DefaultConfig(tx, rx)
		c.Ext, c.Bitrate = tx > 0x7FF, br
		step("UDS %X/%X", tx, rx)
		return s.addUDS(ctx, isotp.New(mux, c, s.log), step)
	case ProfileTP20:
		module := byte(hexValue(cfg.Address, 0x01))
		step("VW TP 2.0 channel to module 0x%02X", module)
		conn, err := tp20.Connect(ctx, mux, module, s.log)
		if err != nil {
			return err
		}
		h, err := ecu.NewVAGKWPHandler(ctx, conn)
		if err != nil {
			conn.Close()
			return err
		}
		s.addECU(h)
		return nil
	case ProfileJ1939:
		return j1939Try()
	case ProfileMonitor:
		return nil
	}
	return fmt.Errorf("CAN adapters cannot do profile %q (K-Line protocols need a KKL cable)", cfg.Profile)
}

// ---------- trucks, Ethernet ----------

func (s *Session) connectRP1210(ctx context.Context, step stepFunc) error {
	ads, err := rp1210.ListAdapters()
	if err != nil || len(ads) == 0 {
		return errors.New("no RP1210 adapter is installed")
	}
	a := ads[0]
	for _, x := range ads {
		if s.Config.Port == fmt.Sprintf("%s:%d", x.Implementation, x.DeviceID) || s.Config.Port == x.Description {
			a = x
		}
	}
	step("opening RP1210 %s", a.Description)
	link, err := rp1210.OpenJ1939(a, j1939.AddrTool1, s.log)
	if err != nil {
		return err
	}
	s.Adapter = link.Name()
	s.addECU(j1939.NewHandler(link))
	return nil
}

func (s *Session) connectDoIP(ctx context.Context, step stepFunc) error {
	addr := s.Config.Port
	target := hexValue(s.Config.Address, 0)
	if addr == "" {
		step("searching for DoIP vehicles on the network")
		ents, err := doip.Discover(ctx, 2*time.Second)
		if err != nil || len(ents) == 0 {
			return fmt.Errorf("no DoIP vehicle found (connect the ENET cable and set a link-local IP): %v", err)
		}
		addr = ents[0].IP
		if target == 0 {
			target = uint32(ents[0].Logical)
		}
		step("found %s (VIN %s, address 0x%04X)", ents[0].IP, ents[0].VIN, ents[0].Logical)
	}
	if target == 0 {
		target = 0x1000
	}
	conn, err := doip.Dial(ctx, addr, doip.DefaultTester, uint16(target), s.log)
	if err != nil {
		return err
	}
	s.Adapter = "DoIP " + addr
	return s.addUDS(ctx, conn, step)
}

func (s *Session) connectHSFZ(ctx context.Context, step stepFunc) error {
	if s.Config.Port == "" {
		ids, _ := doip.DiscoverHSFZ(ctx, 2*time.Second)
		if len(ids) == 0 {
			return errors.New("no BMW found on the network; enter the vehicle IP address")
		}
		s.Config.Port = strings.SplitN(ids[0], ":", 2)[0]
	}
	target := byte(hexValue(s.Config.Address, doip.HSFZGateway))
	step("BMW ENET %s, control unit 0x%02X", s.Config.Port, target)
	conn, err := doip.DialHSFZ(ctx, s.Config.Port, target, s.log)
	if err != nil {
		return err
	}
	s.Adapter = "BMW ENET " + s.Config.Port
	return s.addUDS(ctx, conn, step)
}

// ---------- simulator ----------

// DefaultSimVariant picks the simulated vehicle for a profile.
func DefaultSimVariant(profile string) string {
	switch profile {
	case ProfileRuAuto, ProfileRuLegacy:
		return "gaz-mikas"
	case ProfileRuKWP:
		return "vaz-j72"
	case ProfileKWP:
		return "vaz-m154"
	case ProfileVAG:
		return "vag-kline"
	case ProfileTP20:
		return "vag-can"
	case ProfileJ1939:
		return "truck"
	}
	return "can"
}

func (s *Session) connectSim(ctx context.Context, step stepFunc) error {
	variant := s.Config.SimVariant
	if variant == "" {
		variant = DefaultSimVariant(s.Config.Profile)
	}
	step("starting simulated vehicle: %s", variant)
	w, err := sim.NewWorld(variant)
	if err != nil {
		return err
	}
	s.world = w
	s.Adapter = "Simulator (" + variant + ")"
	s.voltage = func(context.Context) (float64, error) { return w.Engine.State().BatteryV, nil }
	switch {
	case w.KLine != nil:
		if s.Config.Profile == ProfileAuto || s.Config.Profile == ProfileOBD {
			switch variant {
			case "gaz-mikas":
				s.Config.Profile = ProfileRuAuto
			case "vaz-m154", "vaz-r83":
				s.Config.Profile = ProfileRuKWP
			case "vag-kline":
				s.Config.Profile = ProfileVAG
			}
		}
		return s.klineProfiles(ctx, w.KLine, step)
	case w.CAN != nil:
		if s.Config.Profile == ProfileAuto {
			switch variant {
			case "vag-can":
				s.Config.Profile = ProfileTP20
			case "truck":
				s.Config.Profile = ProfileJ1939
			}
		}
		if s.Config.Bitrate == 0 {
			s.Config.Bitrate = w.CAN.Bitrate
		}
		return s.connectCAN(ctx, step, func(int) (can.Bus, error) { return w.CAN.Endpoint("tester"), nil })
	case w.DoIP != nil:
		s.Config.Port = w.DoIP.Addr()
		s.Config.Address = "1000"
		return s.connectDoIP(ctx, step)
	}
	return errors.New("simulator has no bus")
}
