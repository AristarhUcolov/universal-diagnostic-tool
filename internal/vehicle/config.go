// Package vehicle connects everything: it opens the chosen adapter, finds
// out which protocol the vehicle speaks (or uses the one the user picked),
// creates the control unit handlers and runs live data polling and
// keep-alive. The rest of the program (web interface, command line, ELM327
// bridge) only talks to a Session.
package vehicle

import (
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
)

// Adapter kinds.
const (
	AdapterSim       = "sim"    // built-in vehicle simulator
	AdapterKKL       = "kkl"    // K-Line cable on a serial port (VAG KKL 409.1, K+DCAN in K mode)
	AdapterELM       = "elm327" // ELM327 / STN / OBDLink on a serial or Bluetooth port
	AdapterELMWiFi   = "elm327-wifi"
	AdapterELMBLE    = "elm327-ble" // ELM327 over Bluetooth Low Energy
	AdapterJ2534     = "j2534"      // any installed J2534 PassThru driver (Windows)
	AdapterSLCAN     = "slcan"      // Lawicel / CANable / USB-CAN on a serial port
	AdapterSocketCAN = "socketcan"  // Linux can0
	AdapterPCAN      = "pcan"       // PEAK PCAN-Basic (Windows)
	AdapterKvaser    = "kvaser"     // Kvaser CANlib (Windows)
	AdapterRP1210    = "rp1210"     // truck adapters (Windows)
	AdapterDoIP      = "doip"       // Ethernet (ENET) cable, ISO 13400
	AdapterHSFZ      = "hsfz"       // BMW ENET (HSFZ)
)

// Vehicle profiles: what kind of control units to look for.
const (
	ProfileAuto     = "auto"       // try everything that fits the adapter
	ProfileOBD      = "obd"        // generic OBD-II / EOBD (all cars 1996+/2001+)
	ProfileRuKWP    = "ru-kwp"     // VAZ/GAZ/UAZ KWP2000 controllers (Январь-5/7, Bosch M1.5.4/M7.9.7, Микас-10/11)
	ProfileRuLegacy = "ru-legacy"  // Микас-5.4/7.1 and Январь-4 (pre-KWP K-Line protocol)
	ProfileRuAuto   = "ru-auto"    // VAZ/GAZ/UAZ: try KWP2000, then legacy, then OBD
	ProfileVAG      = "vag-kw1281" // VW/Audi/Seat/Skoda K-Line (KW1281)
	ProfileTP20     = "vag-tp20"   // VW/Audi/Seat/Skoda CAN 2004-2012 (KWP2000 on TP 2.0)
	ProfileUDS      = "uds"        // any UDS control unit on CAN (custom identifiers)
	ProfileKWP      = "kwp"        // any KWP2000 control unit on the K-Line (custom address)
	ProfileJ1939    = "j1939"      // trucks, buses, tractors
	ProfileMonitor  = "monitor"    // CAN bus monitor only
)

// Config describes a connection request.
type Config struct {
	Adapter string `json:"adapter"`
	Port    string `json:"port,omitempty"`    // COM3, /dev/ttyUSB0, 192.168.0.10:35000, can0, J2534 device name
	Baud    int    `json:"baud,omitempty"`    // serial speed to the adapter (ELM327, slcan); K-Line baud for kkl
	Profile string `json:"profile"`           // see Profile* constants
	ECU     string `json:"ecu,omitempty"`     // force a definition id
	Bitrate int    `json:"bitrate,omitempty"` // CAN bit rate (0 = try 500k then 250k)
	// CANFD runs UDS over CAN FD (frames up to 64 bytes), as many cars
	// from about 2020 need; DataBitrate is the data phase bit rate
	// (default 2 Mbit/s). Adapters: PCAN-USB FD, a SocketCAN FD
	// interface, CANable 2.0 (slcan) and the simulator.
	CANFD       bool `json:"canFd,omitempty"`
	DataBitrate int  `json:"dataBitrate,omitempty"`
	// Address is the control unit address for K-Line profiles (hex,
	// 10 = engine) or the module for KW1281/TP 2.0 (01 = engine).
	Address string `json:"address,omitempty"`
	// TxID/RxID select the UDS identifiers (default 7E0/7E8).
	TxID string `json:"txId,omitempty"`
	RxID string `json:"rxId,omitempty"`
	// Init forces the K-Line initialisation: fast, slow, none.
	Init string `json:"init,omitempty"`
	// KLine options for KKL cables.
	Echo     string `json:"echo,omitempty"`     // auto, on, off
	DTR      *bool  `json:"dtr,omitempty"`      // default true
	RTS      *bool  `json:"rts,omitempty"`      // default false
	InitMode string `json:"initMode,omitempty"` // break, baud360
	// SimVariant selects the simulated car: vaz (Январь/M1.5.4 KWP2000),
	// mikas (Микас-7.1 legacy), obd-kline, can (modern OBD/UDS car),
	// vag-kline, vag-can, truck.
	SimVariant string `json:"simVariant,omitempty"`
}

// AdapterInfo describes an adapter kind for the user interface.
type AdapterInfo struct {
	ID        string    `json:"id"`
	Name      i18n.Text `json:"name"`
	Examples  string    `json:"examples"`
	Port      string    `json:"port"` // serial, network, j2534, can, rp1210, none
	Profiles  []string  `json:"profiles"`
	Available bool      `json:"available"`
	Note      i18n.Text `json:"note,omitempty"`
}

// ProfileInfo describes a vehicle profile.
type ProfileInfo struct {
	ID   string    `json:"id"`
	Name i18n.Text `json:"name"`
	Hint i18n.Text `json:"hint"`
}

// Profiles lists the vehicle profiles.
var Profiles = []ProfileInfo{
	{ProfileAuto, i18n.T("Automatic (find everything)", "Автоматически (найти всё)"),
		i18n.T("Tries the protocols the adapter supports one after another.", "Перебирает все протоколы, которые поддерживает адаптер.")},
	{ProfileOBD, i18n.T("OBD-II / EOBD (any car since 1996-2001)", "OBD-II / EOBD (любые авто с 1996-2001 г.)"),
		i18n.T("Standard emissions diagnostics: codes, live data, readiness.", "Стандартная диагностика: ошибки, параметры, готовность.")},
	{ProfileRuAuto, i18n.T("VAZ / GAZ / UAZ (Январь, Bosch, Микас) - automatic", "ВАЗ / ГАЗ / УАЗ (Январь, Bosch, Микас) - автоматически"),
		i18n.T("Tries KWP2000 (address 0x10), then the Микас-5.4/7.1 protocol, then OBD-II.", "Пробует KWP2000 (адрес 0x10), затем протокол Микас-5.4/7.1, затем OBD-II.")},
	{ProfileRuKWP, i18n.T("VAZ / GAZ / UAZ - KWP2000 (Январь-5/7, M1.5.4, M7.9.7, Микас-10/11)", "ВАЗ / ГАЗ / УАЗ - KWP2000 (Январь-5/7, M1.5.4, M7.9.7, Микас-10/11)"),
		i18n.T("K-Line, fast init, address 0x10, 10400 baud.", "K-Line, быстрая инициализация, адрес 0x10, 10400 бод.")},
	{ProfileRuLegacy, i18n.T("GAZ / UAZ - Микас-5.4 / 7.1, VAZ - Январь-4", "ГАЗ / УАЗ - Микас-5.4 / 7.1, ВАЗ - Январь-4"),
		i18n.T("Early K-Line protocol, 9600 baud, no initialisation (ZMZ-406 Volga, GAZelle).", "Ранний протокол K-Line, 9600 бод, без инициализации (Волга ЗМЗ-406, ГАЗель).")},
	{ProfileVAG, i18n.T("VW / Audi / Seat / Skoda - K-Line (KW1281, until ~2005)", "VW / Audi / Seat / Skoda - K-Line (KW1281, до ~2005)"),
		i18n.T("5-baud init to a module address (01 engine, 17 instruments...).", "5-бодовая инициализация по адресу модуля (01 двигатель, 17 приборы...).")},
	{ProfileTP20, i18n.T("VW / Audi / Seat / Skoda - CAN TP 2.0 (2004-2012)", "VW / Audi / Seat / Skoda - CAN TP 2.0 (2004-2012)"),
		i18n.T("Experimental. Needs a raw CAN adapter (J2534, CANable, PCAN, Kvaser).", "Экспериментально. Нужен CAN-адаптер (J2534, CANable, PCAN, Kvaser).")},
	{ProfileUDS, i18n.T("UDS (ISO 14229) on CAN - custom identifiers", "UDS (ISO 14229) по CAN - свои идентификаторы"),
		i18n.T("Any modern control unit; default 7E0/7E8.", "Любой современный блок; по умолчанию 7E0/7E8.")},
	{ProfileKWP, i18n.T("KWP2000 on K-Line - custom address", "KWP2000 по K-Line - свой адрес"),
		i18n.T("Any KWP2000 unit (ABS, airbag, immobiliser...).", "Любой KWP2000-блок (АБС, подушки, иммобилизатор...).")},
	{ProfileJ1939, i18n.T("Trucks, buses, tractors - SAE J1939", "Грузовики, автобусы, тракторы - SAE J1939"),
		i18n.T("Heavy-duty 9-pin Deutsch connector, 250 or 500 kbit/s.", "9-контактный разъём Deutsch, 250 или 500 кбит/с.")},
	{ProfileMonitor, i18n.T("CAN bus monitor only", "Только монитор шины CAN"),
		i18n.T("Listen to all CAN traffic (sniffer).", "Прослушивание всего трафика CAN (сниффер).")},
}
