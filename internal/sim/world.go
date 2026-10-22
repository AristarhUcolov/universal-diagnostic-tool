package sim

import (
	"fmt"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
)

// Variant describes a simulated vehicle.
type Variant struct {
	ID   string    `json:"id"`
	Name i18n.Text `json:"name"`
	Bus  string    `json:"bus"` // kline, can, doip
}

// Variants lists the simulated vehicles.
var Variants = []Variant{
	{"gaz-mikas", i18n.T("GAZ-3110 Volga, ZMZ-406, Микас-7.1 (legacy K-Line)", "ГАЗ-3110 «Волга», ЗМЗ-406, Микас-7.1 (ранний K-Line)"), "kline"},
	{"vaz-j72", i18n.T("VAZ-2112, Январь-7.2 (KWP2000 + OBD)", "ВАЗ-2112, Январь-7.2 (KWP2000 + OBD)"), "kline"},
	{"vaz-m154", i18n.T("VAZ-2110, Bosch M1.5.4N Euro-2 (KWP2000)", "ВАЗ-2110, Bosch M1.5.4N Euro-2 (KWP2000)"), "kline"},
	{"vaz-r83", i18n.T("VAZ-21093, Январь-5 Russia-83 (KWP2000, no O2 sensor)", "ВАЗ-21093, Январь-5 Россия-83 (KWP2000, без ДК)"), "kline"},
	{"obd-kline", i18n.T("European car 2002, OBD-II on ISO 9141-2", "Европейский автомобиль 2002, OBD-II по ISO 9141-2"), "kline"},
	{"vag-kline", i18n.T("VW Golf IV 1.8T, KW1281", "VW Golf IV 1.8T, KW1281"), "kline"},
	{"can", i18n.T("Modern car, OBD-II + UDS on CAN 500k", "Современный автомобиль, OBD-II + UDS по CAN 500k"), "can"},
	{"vag-can", i18n.T("VW Passat B6, TP 2.0 on CAN", "VW Passat B6, TP 2.0 по CAN"), "can"},
	{"truck", i18n.T("Truck, SAE J1939 on CAN 250k", "Грузовик, SAE J1939 по CAN 250k"), "can"},
	{"doip", i18n.T("Car with Ethernet diagnostics (DoIP)", "Автомобиль с диагностикой по Ethernet (DoIP)"), "doip"},
}

// World is a running simulated vehicle.
type World struct {
	Variant string
	Engine  *Engine
	KLine   *KBus
	CAN     *CANBus
	Truck   *TruckEngine
	DoIP    *DoIPServer
}

// NewWorld builds a simulated vehicle.
func NewWorld(variant string) (*World, error) {
	e := NewEngine()
	w := &World{Variant: variant, Engine: e}
	switch variant {
	case "gaz-mikas":
		w.KLine = NewKBus()
		w.KLine.Attach(NewLegacyNode(e))
	case "vaz-j72", "vaz-m154", "vaz-r83":
		v := map[string]VAZVariant{"vaz-j72": VariantJ72, "vaz-m154": VariantEuro2, "vaz-r83": VariantR83}[variant]
		app := NewVAZApp(e, v)
		node := NewKWPNode(0x10, app)
		if v == VariantJ72 {
			app.OBD = NewOBDApp(e, "ECM-Jan72")
			node.Functional = true
			node.LenByteFrom = 20
		}
		w.KLine = NewKBus()
		w.KLine.Attach(node)
	case "obd-kline":
		w.KLine = NewKBus()
		w.KLine.Attach(NewISO9141Node(0x10, NewOBDApp(e, "ECM")))
	case "vag-kline":
		w.KLine = NewKBus()
		w.KLine.Attach(NewKW1281Node(e))
	case "can":
		w.CAN = NewCANBus(500000)
		eng := NewOBDApp(e, "ECM-EngineControl")
		eng.UDS = true
		tcm := NewOBDApp(e, "TCM-TransmissionCtl")
		tcm.Minor = true
		w.CAN.AttachISOTP("ecm", 0x7E8, 0x7E0, 0x7DF, false, eng)
		w.CAN.AttachISOTP("tcm", 0x7E9, 0x7E1, 0x7DF, false, tcm)
	case "vag-can":
		w.CAN = NewCANBus(500000)
		w.CAN.AttachTP20(0x01, &VAGApp{Engine: e})
		eng := NewOBDApp(e, "ECM")
		w.CAN.AttachISOTP("ecm", 0x7E8, 0x7E0, 0x7DF, false, eng)
	case "truck":
		w.CAN = NewCANBus(250000)
		w.Truck = w.CAN.AttachJ1939Engine(e)
	case "doip":
		app := NewOBDApp(e, "ECM")
		app.UDS = true
		srv, err := StartDoIP("127.0.0.1:0", app)
		if err != nil {
			return nil, err
		}
		w.DoIP = srv
	default:
		return nil, fmt.Errorf("unknown simulated vehicle %q", variant)
	}
	return w, nil
}

// Close stops the simulation.
func (w *World) Close() {
	if w.KLine != nil {
		w.KLine.Close()
	}
	if w.CAN != nil {
		w.CAN.Close()
	}
	if w.DoIP != nil {
		w.DoIP.Close()
	}
}
