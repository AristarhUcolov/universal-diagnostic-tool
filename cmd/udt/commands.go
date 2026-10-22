package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/bridge"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/desktop"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/dtc"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/ecu"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/j2534"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/rp1210"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/serial"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/sim"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/vehicle"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/version"
)

const helpText = `Universal Diagnostic Tool %s

  udt                      open the graphical interface
  udt ports                list serial ports (COM / tty) with recognised adapters
  udt adapters             list adapter kinds and installed J2534 / RP1210 drivers
  udt profiles             list vehicle profiles and simulated vehicles
  udt definitions          list control unit descriptions
  udt scan    [conn]       connect and show the control units with identification
  udt dtc     [conn] [-clear]       read (and clear) trouble codes
  udt live    [conn] [-params a,b] [-interval 500ms] [-count N]   print live values
  udt raw     [conn] "21 01" ...    send raw requests (hex)
  udt bridge  [conn] [-listen 0.0.0.0:35000]   share the connection as an ELM327 for apps
  udt lookup  P0130 ...    describe trouble codes offline
  udt version

Connection flags [conn]:
  -adapter sim|kkl|elm327|elm327-wifi|j2534|slcan|socketcan|pcan|kvaser|rp1210|doip|hsfz
  -port COM3 | /dev/ttyUSB0 | 192.168.0.10:35000 | can0 | <J2534 device name>
  -profile auto|obd|ru-auto|ru-kwp|ru-legacy|vag-kw1281|vag-tp20|uds|kwp|j1939|monitor
  -sim gaz-mikas|vaz-j72|vaz-m154|vaz-r83|obd-kline|vag-kline|can|vag-can|truck|doip
  -address 10   -init fast|slow   -bitrate 500000   -txid 7E0 -rxid 7E8   -ecu <definition id>
  -baud 38400   -lang ru|en   -v (print the journal)

Examples:
  udt scan -adapter kkl -port COM3 -profile ru-auto        (GAZ/VAZ/UAZ with a KKL cable)
  udt dtc  -adapter elm327 -port COM5                      (any OBD-II car)
  udt live -adapter sim -sim gaz-mikas -params twat,freq,uacc

Support the project:
  https://ko-fi.com/AristarhUcolov
  https://buymeacoffee.com/aristarh.ucolov
  https://www.donationalerts.com/c/aristarh_ucolov
`

func runCommand(cmd string, args []string) int {
	switch cmd {
	case "help", "-h", "--help":
		fmt.Printf(helpText, version.Version)
		return 0
	case "version":
		fmt.Println(version.Name, version.Version)
		return 0
	case "ports":
		return cmdPorts()
	case "adapters":
		return cmdAdapters(langFlag("adapters", args))
	case "profiles":
		return cmdProfiles(langFlag("profiles", args))
	case "definitions":
		return cmdDefinitions()
	case "lookup":
		return cmdLookup(args)
	case "scan", "dtc", "live", "raw", "bridge":
		return cmdSession(cmd, args)
	}
	fmt.Fprintf(os.Stderr, "unknown command %q; run \"udt help\"\n", cmd)
	return 2
}

// langFlag reads the only flag of the listing commands.
func langFlag(name string, args []string) string {
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	l := fs.String("lang", "", "ru or en")
	_ = fs.Parse(args)
	return lang(*l)
}

func lang(flagValue string) string {
	if flagValue != "" {
		return i18n.Normalize(flagValue)
	}
	return i18n.Normalize(desktop.SystemLanguage())
}

func table() *tabwriter.Writer { return tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0) }

func cmdPorts() int {
	ports, err := serial.List()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if len(ports) == 0 {
		fmt.Println("No serial ports found. Plug the adapter in and install its driver (FTDI, CH340, CP210x, PL2303).")
		return 0
	}
	w := table()
	fmt.Fprintln(w, "PORT\tDESCRIPTION\tUSB ID\tCHIP / TYPICAL ADAPTERS")
	for _, p := range ports {
		id := ""
		if p.VID != "" {
			id = p.VID + ":" + p.PID
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Name, p.Description, id, strings.Trim(p.Chip+" - "+p.Hint, " -"))
	}
	w.Flush()
	return 0
}

func cmdAdapters(l string) int {
	w := table()
	fmt.Fprintln(w, "ID\tADAPTER\tAVAILABLE\tEXAMPLES")
	for _, a := range vehicle.Adapters() {
		fmt.Fprintf(w, "%s\t%s\t%v\t%s\n", a.ID, a.Name.Get(l), a.Available, a.Examples)
	}
	w.Flush()
	if devs, _ := j2534.ListDevices(); len(devs) > 0 {
		fmt.Println("\nInstalled J2534 drivers:")
		for _, d := range devs {
			note := ""
			if !d.Usable {
				note = " - " + d.Reason
			}
			fmt.Printf("  %s (%s, %s)%s\n", d.Name, d.Vendor, d.Arch, note)
		}
	}
	if ads, _ := rp1210.ListAdapters(); len(ads) > 0 {
		fmt.Println("\nInstalled RP1210 adapters (use -port impl:deviceId):")
		for _, a := range ads {
			fmt.Printf("  %s:%d  %s (%s) %v\n", a.Implementation, a.DeviceID, a.Description, a.Vendor, a.Protocols)
		}
	}
	return 0
}

func cmdProfiles(l string) int {
	w := table()
	fmt.Fprintln(w, "PROFILE\tDESCRIPTION")
	for _, p := range vehicle.Profiles {
		fmt.Fprintf(w, "%s\t%s\n", p.ID, p.Name.Get(l))
	}
	w.Flush()
	fmt.Println("\nSimulated vehicles (-adapter sim -sim <id>):")
	for _, v := range sim.Variants {
		fmt.Printf("  %-10s %s\n", v.ID, v.Name.Get(l))
	}
	return 0
}

func cmdDefinitions() int {
	w := table()
	fmt.Fprintln(w, "ID\tNAME\tPROTOCOL\tSTATUS\tPARAMETERS")
	for _, d := range ecu.Builtin().Summaries() {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\n", d.ID, d.Name, d.Protocol, d.Status, d.Params)
	}
	w.Flush()
	for _, e := range ecu.Builtin().Errors() {
		fmt.Fprintln(os.Stderr, "definition error:", e)
	}
	return 0
}

func cmdLookup(args []string) int {
	fs := flag.NewFlagSet("lookup", flag.ExitOnError)
	langFlag := fs.String("lang", "", "ru or en")
	family := fs.String("family", "", "description set: ru-kwp, gaz-mikas, vag, ru-r83")
	_ = fs.Parse(args)
	l := lang(*langFlag)
	db := dtc.Default()
	for _, code := range fs.Args() {
		if *family != "" {
			if t, fam, ok := db.Describe(code, *family); ok {
				fmt.Printf("%s  %s  [%s]\n", strings.ToUpper(code), t.Get(l), fam)
				continue
			}
		}
		if ms := db.Lookup(code); len(ms) > 0 {
			for _, m := range ms {
				fmt.Printf("%s  %s  [%s]\n", m.Code, m.Desc.Get(l), m.Family)
			}
			continue
		}
		if dtc.IsSAE(code) {
			fmt.Printf("%s  (%s)\n", strings.ToUpper(code), dtc.SystemOf(strings.ToUpper(code)).Get(l))
		} else {
			fmt.Printf("%s  not found\n", code)
		}
	}
	return 0
}

type connFlags struct {
	cfg     vehicle.Config
	lang    string
	verbose bool
}

func bindConn(fs *flag.FlagSet) *connFlags {
	c := &connFlags{}
	fs.StringVar(&c.cfg.Adapter, "adapter", "sim", "adapter kind")
	fs.StringVar(&c.cfg.Port, "port", "", "port, address or device")
	fs.StringVar(&c.cfg.Profile, "profile", "auto", "vehicle profile")
	fs.StringVar(&c.cfg.SimVariant, "sim", "", "simulated vehicle")
	fs.StringVar(&c.cfg.Address, "address", "", "control unit address (hex)")
	fs.StringVar(&c.cfg.Init, "init", "", "K-Line init: fast or slow")
	fs.IntVar(&c.cfg.Bitrate, "bitrate", 0, "CAN bit rate")
	fs.StringVar(&c.cfg.TxID, "txid", "", "UDS request id (hex)")
	fs.StringVar(&c.cfg.RxID, "rxid", "", "UDS response id (hex)")
	fs.StringVar(&c.cfg.ECU, "ecu", "", "force a control unit description")
	fs.IntVar(&c.cfg.Baud, "baud", 0, "serial speed of the adapter")
	fs.StringVar(&c.lang, "lang", "", "output language: ru or en")
	fs.BoolVar(&c.verbose, "v", false, "print the communication journal")
	return c
}

// printLogger prints journal entries to stderr.
type printLogger struct{}

func (printLogger) Log(e journal.Entry) {
	if len(e.Data) > 0 {
		fmt.Fprintf(os.Stderr, "%s %-5s %-7s %-6s %s  %s\n", time.Now().Format("15:04:05.000"), e.Kind, e.Layer, e.Addr, hexutil.Format(e.Data), e.Text)
		return
	}
	fmt.Fprintf(os.Stderr, "%s %-5s %-7s %s\n", time.Now().Format("15:04:05.000"), e.Kind, e.Layer, e.Text)
}

func cmdSession(cmd string, args []string) int {
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	c := bindConn(fs)
	clearFlag := fs.Bool("clear", false, "clear the trouble codes after reading (dtc)")
	params := fs.String("params", "", "comma-separated parameter ids (live); empty = the first 8")
	interval := fs.Duration("interval", 500*time.Millisecond, "live update interval")
	count := fs.Int("count", 0, "number of live updates (0 = until Ctrl+C)")
	listen := fs.String("listen", "0.0.0.0:35000", "bridge address")
	_ = fs.Parse(args)
	l := lang(c.lang)

	ctx, cancel := signalContext()
	defer cancel()
	var log journal.Logger = journal.Discard
	if c.verbose {
		log = printLogger{}
	}
	fmt.Fprintf(os.Stderr, "connecting (%s, %s)...\n", c.cfg.Adapter, c.cfg.Profile)
	sess, err := vehicle.Connect(ctx, c.cfg, log, func(step string) { fmt.Fprintln(os.Stderr, "  "+step) })
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	defer sess.Close()
	e, _ := sess.ECU("")
	h := e.Handler()

	switch cmd {
	case "scan":
		for _, en := range sess.ECUs() {
			d := en.Handler().Describe()
			fmt.Printf("\n[%s] %s\n  protocol: %s\n  address:  %s\n  status:   %s\n", en.ID, d.Name, d.Protocol, d.Address, d.Status)
			items, err := en.Handler().Identify(ctx)
			if err != nil {
				fmt.Println("  identification failed:", err)
			}
			for _, it := range items {
				fmt.Printf("  %-28s %s\n", it.Name.Get(l)+":", it.Value)
			}
		}
		if v, err := sess.Voltage(ctx); err == nil {
			fmt.Printf("\nsupply voltage: %.1f V\n", v)
		}
	case "dtc":
		for _, en := range sess.ECUs() {
			codes, err := en.Handler().ReadDTC(ctx)
			if err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				return 1
			}
			fmt.Printf("%s: %d trouble code(s)\n", en.Handler().Describe().Name, len(codes))
			for _, d := range codes {
				var flags []string
				for _, f := range d.Flags {
					flags = append(flags, dtc.FlagNames[f].Get(l))
				}
				fmt.Printf("  %-12s %s", d.Code, d.Desc.Get(l))
				if !d.Detail.Empty() {
					fmt.Printf(" - %s", d.Detail.Get(l))
				}
				if len(flags) > 0 {
					fmt.Printf(" [%s]", strings.Join(flags, ", "))
				}
				fmt.Println()
			}
			if *clearFlag {
				if err := en.Handler().ClearDTC(ctx); err != nil {
					fmt.Fprintln(os.Stderr, "clear failed:", err)
					return 1
				}
				fmt.Println("  trouble codes cleared")
			}
		}
	case "live":
		ps := h.Params()
		index := ecu.ParamIndex(ps)
		var ids []string
		if *params != "" {
			for _, id := range strings.Split(*params, ",") {
				ids = append(ids, strings.TrimSpace(id))
			}
		} else {
			for _, p := range ps {
				if len(ids) < 8 && p.Kind == "num" {
					ids = append(ids, p.ID)
				}
			}
		}
		if len(ids) == 0 {
			fmt.Println("this control unit has no live parameters; available:", len(ps))
			return 1
		}
		for _, id := range ids {
			if p, ok := index[id]; ok {
				fmt.Printf("%s = %s [%s]\n", id, p.Name.Get(l), p.Unit)
			}
		}
		for n := 0; *count == 0 || n < *count; n++ {
			vals, err := h.Read(ctx, ids)
			if err != nil {
				if ctx.Err() != nil {
					return 0
				}
				fmt.Fprintln(os.Stderr, "read error:", err)
				time.Sleep(time.Second)
				continue
			}
			var parts []string
			for _, v := range vals {
				s := "-"
				switch {
				case v.OK && v.Text != "":
					s = v.Text
				case v.OK:
					s = strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", v.V), "0"), ".")
				}
				parts = append(parts, v.ID+"="+s)
			}
			fmt.Println(time.Now().Format("15:04:05.000"), strings.Join(parts, "  "))
			select {
			case <-ctx.Done():
				return 0
			case <-time.After(*interval):
			}
		}
	case "raw":
		for _, a := range fs.Args() {
			req, err := hexutil.Parse(a)
			if err != nil || len(req) == 0 {
				fmt.Fprintln(os.Stderr, "bad hex:", a)
				continue
			}
			resp, err := h.Raw(ctx, req)
			fmt.Printf("> %s\n", hexutil.Format(req))
			if len(resp) > 0 {
				fmt.Printf("< %s   %s\n", hexutil.Format(resp), hexutil.Printable(resp))
			}
			if err != nil {
				fmt.Println("!", err)
			}
		}
	case "bridge":
		b, err := bridge.Start(*listen, func() *vehicle.Session { return sess }, log)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer b.Close()
		fmt.Printf("ELM327 bridge on %s - connect your app to this computer's IP, port %s. Ctrl+C to stop.\n",
			b.Addr(), b.Addr()[strings.LastIndex(b.Addr(), ":")+1:])
		<-ctx.Done()
	}
	return 0
}
