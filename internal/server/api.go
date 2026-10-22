package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/bridge"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/desktop"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/diag"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/dtc"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/ecu"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/hexutil"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/j2534"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/journal"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/pcan"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/rp1210"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/serial"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/settings"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/sim"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/socketcan"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/vehicle"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/version"
)

var errNotConnected = errors.New("not connected to a vehicle")

func (s *Server) apiInfo(w http.ResponseWriter, r *http.Request) {
	db := dtc.Default()
	fams := map[string]int{}
	for _, f := range db.Families() {
		fams[f] = db.Count(f)
	}
	writeJSON(w, map[string]any{
		"name": version.Name, "version": version.Version, "os": runtime.GOOS, "arch": runtime.GOARCH,
		"configDir": settings.Dir(), "logFile": s.journal.FilePath(), "systemLanguage": desktop.SystemLanguage(),
		"definitions": len(ecu.Builtin().All()), "definitionErrors": ecu.Builtin().Errors(),
		"dtcFamilies": fams, "lanUrl": s.LANURL(),
	})
}

func (s *Server) apiPorts(w http.ResponseWriter, r *http.Request) {
	ports, err := serial.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if ports == nil {
		ports = []serial.PortInfo{}
	}
	writeJSON(w, ports)
}

func (s *Server) apiAdapters(w http.ResponseWriter, r *http.Request) {
	jd, _ := j2534.ListDevices()
	rp, _ := rp1210.ListAdapters()
	writeJSON(w, map[string]any{
		"adapters": vehicle.Adapters(), "profiles": vehicle.Profiles, "simVariants": sim.Variants,
		"j2534": jd, "rp1210": rp, "socketcan": socketcan.Interfaces(), "pcan": pcan.Channels,
	})
}

func (s *Server) apiDefinitions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"definitions": ecu.Builtin().Summaries(), "errors": ecu.Builtin().Errors(),
		"folder": settings.Sub("definitions")})
}

func (s *Server) apiGetSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.settings)
}

func (s *Server) apiSetSettings(w http.ResponseWriter, r *http.Request) {
	var in settings.Settings
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if in.LiveParams == nil {
		in.LiveParams = s.settings.LiveParams
	}
	*s.settings = in
	if err := s.settings.Save(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, s.settings)
}

func (s *Server) apiConnect(w http.ResponseWriter, r *http.Request) {
	var cfg vehicle.Config
	if err := readJSON(r, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if cfg.Adapter == "" {
		writeError(w, http.StatusBadRequest, errors.New("choose an adapter"))
		return
	}
	s.settings.Last = cfg
	_ = s.settings.Save()
	s.mgr.connect(cfg, func(sess *vehicle.Session) { go s.pumpSession(sess) })
	writeJSON(w, map[string]string{"state": StateConnecting})
}

func (s *Server) apiDisconnect(w http.ResponseWriter, r *http.Request) {
	s.mgr.disconnect()
	writeJSON(w, map[string]string{"state": StateIdle})
}

type ecuView struct {
	ID        string          `json:"id"`
	Desc      ecu.Description `json:"desc"`
	Params    int             `json:"params"`
	Actuators int             `json:"actuators"`
}

func (s *Server) statusView() map[string]any {
	state, errText, progress := s.mgr.status()
	out := map[string]any{"state": state, "error": errText, "progress": progress, "version": version.Version}
	if sess := s.mgr.session(); sess != nil {
		var ecus []ecuView
		for _, e := range sess.ECUs() {
			h := e.Handler()
			ecus = append(ecus, ecuView{ID: e.ID, Desc: h.Describe(), Params: len(h.Params()), Actuators: len(h.Actuators())})
		}
		out["ecus"] = ecus
		out["adapter"] = sess.Adapter
		out["config"] = sess.Config
		out["started"] = sess.Started
		out["canBus"] = sess.Bus() != nil
		out["simulator"] = sess.World() != nil
		if live, on := sess.LiveRunning(); on {
			out["live"] = live
		}
	}
	s.bridgeMu.Lock()
	if s.bridge != nil {
		out["bridge"] = map[string]any{"addr": s.bridge.Addr(), "clients": s.bridge.Clients()}
	}
	s.bridgeMu.Unlock()
	return out
}

func (s *Server) apiStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.statusView())
}

func (s *Server) apiVoltage(w http.ResponseWriter, r *http.Request) {
	sess := s.mgr.session()
	if sess == nil {
		writeError(w, http.StatusConflict, errNotConnected)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	v, err := sess.Voltage(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, map[string]float64{"voltage": v})
}

// handler resolves the control unit of a request.
func (s *Server) handler(w http.ResponseWriter, r *http.Request) (ecu.Handler, *vehicle.Session, bool) {
	sess := s.mgr.session()
	if sess == nil {
		writeError(w, http.StatusConflict, errNotConnected)
		return nil, nil, false
	}
	e, err := sess.ECU(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return nil, nil, false
	}
	sess.Touch()
	return e.Handler(), sess, true
}

func ctxFor(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

func (s *Server) apiIdentify(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.handler(w, r)
	if !ok {
		return
	}
	ctx, cancel := ctxFor(r, 30*time.Second)
	defer cancel()
	items, err := h.Identify(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, map[string]any{"desc": h.Describe(), "items": items})
}

func (s *Server) apiDTC(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.handler(w, r)
	if !ok {
		return
	}
	ctx, cancel := ctxFor(r, 30*time.Second)
	defer cancel()
	codes, err := h.ReadDTC(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if codes == nil {
		codes = []dtc.DTC{}
	}
	journal.Infof(s.journal, "app", "read %d trouble code(s)", len(codes))
	writeJSON(w, map[string]any{"codes": codes, "flagNames": dtc.FlagNames, "time": time.Now()})
}

func (s *Server) apiClearDTC(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.handler(w, r)
	if !ok {
		return
	}
	ctx, cancel := ctxFor(r, 30*time.Second)
	defer cancel()
	if err := h.ClearDTC(ctx); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	journal.Infof(s.journal, "app", "trouble codes cleared")
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) apiFreeze(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.handler(w, r)
	if !ok {
		return
	}
	ff, isFF := h.(ecu.FreezeFrameReader)
	if !isFF {
		writeJSON(w, map[string]any{"supported": false})
		return
	}
	ctx, cancel := ctxFor(r, 30*time.Second)
	defer cancel()
	vals, err := ff.FreezeFrame(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, map[string]any{"supported": true, "values": vals, "params": ecu.ParamIndex(h.Params())})
}

func (s *Server) apiReadiness(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.handler(w, r)
	if !ok {
		return
	}
	rr, isRR := h.(ecu.ReadinessReader)
	if !isRR {
		writeJSON(w, map[string]any{"supported": false})
		return
	}
	ctx, cancel := ctxFor(r, 20*time.Second)
	defer cancel()
	mil, n, mons, err := rr.Readiness(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, map[string]any{"supported": true, "mil": mil, "count": n, "monitors": mons})
}

func (s *Server) apiParams(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.handler(w, r)
	if !ok {
		return
	}
	ps := h.Params()
	if ps == nil {
		ps = []ecu.Param{}
	}
	desc := h.Describe()
	key := desc.Definition
	if key == "" {
		key = desc.ID
	}
	writeJSON(w, map[string]any{"params": ps, "selected": s.settings.LiveParams[key], "key": key})
}

func (s *Server) apiRead(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.handler(w, r)
	if !ok {
		return
	}
	var in struct {
		IDs []string `json:"ids"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := ctxFor(r, 20*time.Second)
	defer cancel()
	vals, err := h.Read(ctx, in.IDs)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, map[string]any{"values": vals})
}

func (s *Server) apiActuators(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.handler(w, r)
	if !ok {
		return
	}
	acts := h.Actuators()
	if acts == nil {
		acts = []ecu.Actuator{}
	}
	writeJSON(w, map[string]any{"actuators": acts})
}

func (s *Server) apiActuate(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.handler(w, r)
	if !ok {
		return
	}
	var in struct {
		ID     string  `json:"id"`
		Action string  `json:"action"`
		Value  float64 `json:"value"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := ctxFor(r, 15*time.Second)
	defer cancel()
	journal.Infof(s.journal, "app", "output test %s: %s %v", in.ID, in.Action, in.Value)
	resp, err := h.Actuate(ctx, in.ID, in.Action, in.Value)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, map[string]any{"ok": true, "response": resp})
}

func (s *Server) apiRaw(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.handler(w, r)
	if !ok {
		return
	}
	var in struct {
		Hex string `json:"hex"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	req, err := hexutil.Parse(in.Hex)
	if err != nil || len(req) == 0 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("enter bytes in hex, e.g. 21 01: %v", err))
		return
	}
	ctx, cancel := ctxFor(r, 20*time.Second)
	defer cancel()
	out := map[string]any{"request": hexutil.Format(req), "service": diag.ServiceName(req[0])}
	type rawAll interface {
		RawAll(context.Context, []byte) ([]diag.Message, error)
	}
	if ra, isAll := h.(rawAll); isAll {
		msgs, err := ra.RawAll(ctx, req)
		var answers []map[string]string
		for _, m := range msgs {
			answers = append(answers, map[string]string{"from": fmt.Sprintf("%X", m.Src), "hex": hexutil.Format(m.Data), "text": describeAnswer(m.Data)})
		}
		out["answers"] = answers
		if err != nil {
			out["error"] = err.Error()
		}
		writeJSON(w, out)
		return
	}
	resp, err := h.Raw(ctx, req)
	if len(resp) > 0 {
		out["answers"] = []map[string]string{{"hex": hexutil.Format(resp), "text": describeAnswer(resp), "ascii": hexutil.Printable(resp)}}
	}
	if err != nil {
		out["error"] = err.Error()
	}
	writeJSON(w, out)
}

func describeAnswer(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if b[0] == 0x7F && len(b) >= 3 {
		return fmt.Sprintf("negative: %s (%s)", diag.NRCName(b[2]), diag.ServiceName(b[1]))
	}
	return diag.ServiceName(b[0])
}

func (s *Server) apiBlocks(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.handler(w, r)
	if !ok {
		return
	}
	br, isBR := h.(ecu.BlockReader)
	if !isBR {
		writeJSON(w, map[string]any{"supported": false})
		return
	}
	writeJSON(w, map[string]any{"supported": true, "blocks": br.Blocks()})
}

func (s *Server) apiBlock(w http.ResponseWriter, r *http.Request) {
	h, _, ok := s.handler(w, r)
	if !ok {
		return
	}
	br, isBR := h.(ecu.BlockReader)
	if !isBR {
		writeError(w, http.StatusBadRequest, errors.New("this control unit has no raw records"))
		return
	}
	var in struct {
		ID string `json:"id"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := ctxFor(r, 10*time.Second)
	defer cancel()
	data, err := br.ReadBlock(ctx, in.ID)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	bytes := make([]int, len(data))
	for i, b := range data {
		bytes[i] = int(b)
	}
	writeJSON(w, map[string]any{"id": in.ID, "bytes": bytes, "hex": hexutil.Format(data), "time": time.Now()})
}

func (s *Server) apiLiveStart(w http.ResponseWriter, r *http.Request) {
	sess := s.mgr.session()
	if sess == nil {
		writeError(w, http.StatusConflict, errNotConnected)
		return
	}
	var cfg vehicle.LiveConfig
	if err := readJSON(r, &cfg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if e, err := sess.ECU(cfg.ECU); err == nil {
		d := e.Handler().Describe()
		key := d.Definition
		if key == "" {
			key = d.ID
		}
		s.settings.LiveParams[key] = cfg.Params
		_ = s.settings.Save()
	}
	if err := sess.StartLive(cfg); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.notifyStatus()
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) apiLiveStop(w http.ResponseWriter, r *http.Request) {
	if sess := s.mgr.session(); sess != nil {
		sess.StopLive()
	}
	s.notifyStatus()
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) apiRecordStart(w http.ResponseWriter, r *http.Request) {
	sess := s.mgr.session()
	if sess == nil {
		writeError(w, http.StatusConflict, errNotConnected)
		return
	}
	path := filepath.Join(settings.Sub("recordings"), "live-"+time.Now().Format("20060102-150405")+".csv")
	if err := sess.StartRecording(path); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]string{"file": path})
}

func (s *Server) apiRecordStop(w http.ResponseWriter, r *http.Request) {
	path := ""
	if sess := s.mgr.session(); sess != nil {
		path = sess.StopRecording()
	}
	writeJSON(w, map[string]string{"file": path})
}

func (s *Server) apiMonitorStart(w http.ResponseWriter, r *http.Request) {
	sess := s.mgr.session()
	if sess == nil {
		writeError(w, http.StatusConflict, errNotConnected)
		return
	}
	if err := sess.StartMonitor(); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) apiMonitorStop(w http.ResponseWriter, r *http.Request) {
	if sess := s.mgr.session(); sess != nil {
		sess.StopMonitor()
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) apiMonitorSend(w http.ResponseWriter, r *http.Request) {
	sess := s.mgr.session()
	if sess == nil {
		writeError(w, http.StatusConflict, errNotConnected)
		return
	}
	var in struct {
		Frame string `json:"frame"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := sess.SendFrame(r.Context(), in.Frame); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	journal.Infof(s.journal, "can", "frame sent by the user: %s", in.Frame)
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) apiDTCSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	writeJSON(w, map[string]any{"results": dtc.Default().Search(q, 200)})
}

func (s *Server) apiJournal(w http.ResponseWriter, r *http.Request) {
	since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 2000
	}
	entries := s.journal.Since(since, limit)
	if entries == nil {
		entries = []journal.Entry{}
	}
	writeJSON(w, map[string]any{"entries": entries, "last": s.journal.Last(), "file": s.journal.FilePath()})
}

func (s *Server) apiJournalCSV(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=udt-journal-"+time.Now().Format("20060102-150405")+".csv")
	_ = s.journal.WriteCSV(w)
}

func (s *Server) apiJournalClear(w http.ResponseWriter, r *http.Request) {
	s.journal.Clear()
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) apiBridgeStart(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Addr string `json:"addr"`
	}
	_ = readJSON(r, &in)
	if in.Addr == "" {
		in.Addr = s.settings.BridgeAddr
	}
	s.stopBridge()
	b, err := bridge.Start(in.Addr, s.mgr.session, s.journal)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.bridgeMu.Lock()
	s.bridge = b
	s.bridgeMu.Unlock()
	s.notifyStatus()
	writeJSON(w, map[string]any{"addr": b.Addr(), "lanIp": localIP()})
}

func (s *Server) apiBridgeStop(w http.ResponseWriter, r *http.Request) {
	s.stopBridge()
	s.notifyStatus()
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) stopBridge() {
	s.bridgeMu.Lock()
	defer s.bridgeMu.Unlock()
	if s.bridge != nil {
		s.bridge.Close()
		s.bridge = nil
	}
}

func (s *Server) apiSimControl(w http.ResponseWriter, r *http.Request) {
	sess := s.mgr.session()
	if sess == nil || sess.World() == nil {
		writeError(w, http.StatusConflict, errors.New("the simulator is not running"))
		return
	}
	var in struct {
		Action string `json:"action"`
		Code   string `json:"code"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	e := sess.World().Engine
	switch in.Action {
	case "rev":
		e.Rev(3*time.Second, 0.35)
	case "start":
		e.SetRunning(true)
	case "stop":
		e.SetRunning(false)
	case "drive":
		e.SetDriving(true)
	case "park":
		e.SetDriving(false)
	case "fault":
		code := in.Code
		if code == "" {
			code = "P0300"
		}
		e.AddFault(sim.Fault{Code: code, Mikas: 53, VAG: 16684, Active: true})
	default:
		writeError(w, http.StatusBadRequest, fmt.Errorf("unknown action %q", in.Action))
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *Server) apiOpenFolder(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind string `json:"kind"`
	}
	_ = readJSON(r, &in)
	if !isLoopback(r) {
		writeError(w, http.StatusForbidden, errors.New("only on this computer"))
		return
	}
	p := openFolderPath(in.Kind)
	if _, err := os.Stat(p); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	_ = desktop.OpenBrowser(p)
	writeJSON(w, map[string]string{"path": p})
}

// externalLinks are the pages the interface may open in the system
// browser. The program's own window (WebView2) does not open links by
// itself, and anything else passed to the shell could start a program.
var externalLinks = []string{
	"https://ko-fi.com/AristarhUcolov",
	"https://buymeacoffee.com/aristarh.ucolov",
	"https://www.donationalerts.com/c/aristarh_ucolov",
	"https://github.com/AristarhUcolov/universal-diagnostic-tool",
}

// openExternal opens a page in the system browser (replaced in tests).
var openExternal = desktop.OpenBrowser

func allowedLink(u string) bool {
	for _, c := range u {
		if c <= ' ' || strings.ContainsRune("\"'<>\\^`{|}", c) {
			return false
		}
	}
	for _, l := range externalLinks {
		if u == l || strings.HasPrefix(u, l+"/") {
			return true
		}
	}
	return false
}

func (s *Server) apiOpenURL(w http.ResponseWriter, r *http.Request) {
	var in struct {
		URL string `json:"url"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if !isLoopback(r) || !allowedLink(in.URL) {
		writeError(w, http.StatusForbidden, errors.New("this link cannot be opened from here"))
		return
	}
	if err := openExternal(in.URL); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// apiEvents streams status changes, live values, bus monitor snapshots
// and journal entries as server-sent events.
func (s *Server) apiEvents(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	send := func(typ string, v any) bool {
		b, err := json.Marshal(v)
		if err != nil {
			return true
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", typ, b); err != nil {
			return false
		}
		fl.Flush()
		return true
	}
	statusCh := make(chan struct{}, 1)
	s.statusMu.Lock()
	s.statusSubs[statusCh] = true
	s.statusMu.Unlock()
	defer func() {
		s.statusMu.Lock()
		delete(s.statusSubs, statusCh)
		s.statusMu.Unlock()
	}()
	jid, jch := s.journal.Subscribe(1024)
	defer s.journal.Unsubscribe(jid)

	var sess *vehicle.Session
	var sid int
	var sch <-chan vehicle.Event
	attach := func() {
		cur := s.mgr.session()
		if cur == sess {
			return
		}
		if sess != nil {
			sess.Unsubscribe(sid)
		}
		sess, sch = cur, nil
		if sess != nil {
			sid, sch = sess.Subscribe()
		}
	}
	defer func() {
		if sess != nil {
			sess.Unsubscribe(sid)
		}
	}()
	attach()
	if !send("status", s.statusView()) {
		return
	}
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	flushJournal := time.NewTicker(300 * time.Millisecond)
	defer flushJournal.Stop()
	var pending []journal.Entry
	for {
		select {
		case <-r.Context().Done():
			return
		case <-statusCh:
			attach()
			if !send("status", s.statusView()) {
				return
			}
		case ev, ok := <-sch:
			if !ok {
				sch = nil
				continue
			}
			if !send(ev.Type, ev) {
				return
			}
		case e, ok := <-jch:
			if ok {
				pending = append(pending, e)
			}
		case <-flushJournal.C:
			if len(pending) > 0 {
				if !send("journal", pending) {
					return
				}
				pending = nil
			}
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

// pumpSession forwards session status changes (live data stopping) to
// the event streams.
func (s *Server) pumpSession(sess *vehicle.Session) {
	id, ch := sess.Subscribe()
	defer sess.Unsubscribe(id)
	for ev := range ch {
		if ev.Type == "status" {
			s.notifyStatus()
		}
	}
}
