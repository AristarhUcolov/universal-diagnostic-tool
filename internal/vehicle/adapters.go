package vehicle

import (
	"runtime"

	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/i18n"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/j2534"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/kvaser"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/pcan"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/rp1210"
	"github.com/AristarhUcolov/universal-diagnostic-tool/internal/socketcan"
)

var kLineProfiles = []string{ProfileAuto, ProfileOBD, ProfileRuAuto, ProfileRuKWP, ProfileRuLegacy, ProfileVAG, ProfileKWP}
var canProfiles = []string{ProfileAuto, ProfileOBD, ProfileUDS, ProfileTP20, ProfileJ1939, ProfileMonitor}

// Adapters lists the adapter kinds and whether they can be used here.
func Adapters() []AdapterInfo {
	win := runtime.GOOS == "windows"
	lin := runtime.GOOS == "linux"
	j2534Devs, _ := j2534.ListDevices()
	rpDevs, _ := rp1210.ListAdapters()
	out := []AdapterInfo{
		{ID: AdapterSim, Name: i18n.T("Simulator (no car needed)", "Симулятор (без машины)"),
			Examples: "VAZ Январь-7.2, GAZ Микас-7.1, OBD-II CAN car, VW KW1281, truck J1939",
			Port:     "none", Profiles: append(append([]string{}, kLineProfiles...), ProfileTP20, ProfileUDS, ProfileJ1939, ProfileMonitor), Available: true},
		{ID: AdapterKKL, Name: i18n.T("K-Line cable (KKL)", "K-Line кабель (KKL)"),
			Examples: "VAG KKL 409.1 (FTDI, CH340), K+L-Line, VAG-COM, K+DCAN (K mode), RS232 KKL, GAZ/VAZ 12-pin adapters",
			Port:     "serial", Profiles: kLineProfiles, Available: true,
			Note: i18n.T("Supports every K-Line protocol, including Микас-5.4/7.1 and VW KW1281.", "Поддерживает все протоколы K-Line, включая Микас-5.4/7.1 и VW KW1281.")},
		{ID: AdapterELM, Name: i18n.T("ELM327 / OBDLink / STN (USB, Bluetooth)", "ELM327 / OBDLink / STN (USB, Bluetooth)"),
			Examples: "ELM327 v1.5/v2.1, OBDLink SX/LX/MX+/EX, STN1110/STN2120, vLinker, Veepeak",
			Port:     "serial", Profiles: []string{ProfileAuto, ProfileOBD, ProfileRuKWP, ProfileKWP, ProfileUDS}, Available: true,
			Note: i18n.T("Pair Bluetooth adapters in the OS first; they appear as a COM port.", "Bluetooth-адаптеры сначала сопрягите в системе; они появятся как COM-порт.")},
		{ID: AdapterELMWiFi, Name: i18n.T("ELM327 Wi-Fi", "ELM327 Wi-Fi"),
			Examples: "ELM327 Wi-Fi (192.168.0.10:35000), OBDLink MX+ Wi-Fi",
			Port:     "network", Profiles: []string{ProfileAuto, ProfileOBD, ProfileRuKWP, ProfileKWP, ProfileUDS}, Available: true},
		{ID: AdapterJ2534, Name: i18n.T("J2534 PassThru interface", "Интерфейс J2534 PassThru"),
			Examples: "Tactrix OpenPort 2.0, Mini-VCI, VCM II, MDI, MVCI, VXDIAG, Scanmatik, Godiag GD101, MaxiFlash, RLink, Bosch MDI",
			Port:     "j2534", Profiles: []string{ProfileAuto, ProfileOBD, ProfileRuKWP, ProfileKWP, ProfileUDS, ProfileTP20, ProfileJ1939, ProfileMonitor},
			Available: win && len(j2534Devs) > 0,
			Note:      i18n.T("Uses the driver installed by the interface manufacturer.", "Использует драйвер, установленный производителем интерфейса.")},
		{ID: AdapterSLCAN, Name: i18n.T("USB-CAN (slcan / Lawicel)", "USB-CAN (slcan / Lawicel)"),
			Examples: "CANable, CANtact, Lawicel CANUSB, USB-CAN (slcan firmware)",
			Port:     "serial", Profiles: canProfiles, Available: true},
		{ID: AdapterSocketCAN, Name: i18n.T("SocketCAN (Linux)", "SocketCAN (Linux)"),
			Examples: "PCAN-USB, Kvaser, CANable candleLight, Raspberry Pi + MCP2515, vcan",
			Port:     "can", Profiles: canProfiles, Available: lin && len(socketcan.Interfaces()) > 0},
		{ID: AdapterPCAN, Name: i18n.T("PEAK PCAN-USB (PCAN-Basic)", "PEAK PCAN-USB (PCAN-Basic)"),
			Examples: "PCAN-USB, PCAN-USB FD, PCAN-USB Pro", Port: "pcan", Profiles: canProfiles, Available: pcan.Available()},
		{ID: AdapterKvaser, Name: i18n.T("Kvaser (CANlib)", "Kvaser (CANlib)"),
			Examples: "Kvaser Leaf, USBcan, Memorator", Port: "kvaser", Profiles: canProfiles, Available: kvaser.Channels() > 0},
		{ID: AdapterRP1210, Name: i18n.T("RP1210 truck adapter", "Грузовой адаптер RP1210"),
			Examples: "Nexiq USB-Link, Noregon DLA+, Dearborn DPA5, Cummins INLINE, CAT CA3, Jaltest Link, TEXA, VOCOM",
			Port:     "rp1210", Profiles: []string{ProfileJ1939}, Available: win && len(rpDevs) > 0},
		{ID: AdapterDoIP, Name: i18n.T("Ethernet DoIP (ENET cable)", "Ethernet DoIP (ENET-кабель)"),
			Examples: "OBD-RJ45 (ENET): VAG MQB evo, Volvo SPA, JLR, Mercedes, EV", Port: "network", Profiles: []string{ProfileUDS, ProfileOBD}, Available: true},
		{ID: AdapterHSFZ, Name: i18n.T("BMW ENET (HSFZ)", "BMW ENET (HSFZ)"),
			Examples: "BMW F/G/I series, ENET OBD-RJ45", Port: "network", Profiles: []string{ProfileUDS}, Available: true},
	}
	return out
}
