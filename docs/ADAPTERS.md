<a id="ru"></a>

**Русский** · [English](#eng)

# Совместимость адаптеров

Сотни диагностических адаптеров сводятся к нескольким типам интерфейса. Если
программа умеет тип, она умеет и адаптер — поэтому таблицы ниже сгруппированы
по типам.

> **Честно.** Каждый тип интерфейса проверен автотестами на встроенном
> симуляторе, но **ни один адаптер из этих таблиц пока не проверен на
> реальной машине**. Отметка «✓» значит «программа это поддерживает», а не
> «проверено на железе». Проверили свой адаптер — напишите в
> [issues](https://github.com/AristarhUcolov/universal-diagnostic-tool/issues),
> и мы отметим его как проверенный.

**Обозначения**

| Знак | Что значит |
|---|---|
| ✓ | поддерживается: драйвер этого типа встроен в программу |
| J | через драйвер J2534 PassThru, который устанавливает производитель адаптера |
| R | через драйвер RP1210 (грузовые адаптеры) |
| ~ | частично или неизвестно: работает в одном из режимов, зависит от прошивки или драйвера, не проверено |
| ✕ | не поддерживается: закрытый протокол, адаптер работает только со своей программой |
| — | вне задач проекта: прошивка блоков, ключи, иммобилайзер, пробег, программаторы памяти |

**Как проверить свой адаптер.** Команда `udt adapters` (или страница
«Подключение») показывает установленные в Windows драйверы J2534 и RP1210 и
их разрядность. Большинство таких драйверов 32-битные — для них нужна
программа `udt-x86.exe`. Если вашего адаптера нет в списке драйверов, значит
производитель не дал стандартного драйвера, и сторонние программы, включая
эту, с ним работать не смогут.

## K-Line кабели (KKL)

| Адаптер | Статус | Примечание |
|---|---|---|
| VAG KKL 409.1 на FTDI (FT232RL) | ✓ | лучший выбор для K-Line; поставьте Latency Timer = 1 мс |
| VAG KKL на CH340 / CH341 | ✓ | обычно работает; 5-бодовая инициализация на части клонов бывает нестабильной |
| VAG KKL RS232 (COM-порт) | ✓ | питание схемы часто берётся с DTR — оставьте «DTR включён» |
| K+L-Line кабели | ✓ | L-линия управляется сигналом RTS |
| VAG-COM (кабель 409.1) | ✓ | это тот же KKL-кабель |
| BMW K+DCAN, BMW INPA Interface | ~ | режим K-Line работает как KKL; режим D-CAN не поддерживается |
| Переходники ГАЗ/ВАЗ/УАЗ 12 pin → OBD-II | ✓ | с любым KKL; проверьте разводку — [GAZ-VOLGA.md](GAZ-VOLGA.md) |
| Tactrix OpenPort 1.3U | ~ | USB-кабель K-Line на FTDI; может работать как KKL, не проверено |
| Tactrix OpenPort 1.3 | ~ | то же |
| K+CAN Commander 1.4 | ~ | K-Line — как KKL; CAN — закрытый протокол, ✕ |
| AutoDia K509 | ~ | K-Line кабель; не проверено |
| Galletto 1260, FGTech Galletto | ~ | внутри FTDI; режим K-Line может работать как KKL; прошивка — вне задач |
| VCP (VAG CAN PRO) | ✕ | закрытый протокол, только программа VCP |
| VCDS HEX-V2, HEX-NET, HEX+CAN | ✕ | работают только с VCDS (Ross-Tech) |
| VAG Commander | — | это программа; кабель KKL к ней подходит и сюда |
| VagTacho, VAG EEPROM Programmer | — | одометр и иммобилайзер — вне задач |

## ELM327 и совместимые

| Адаптер | Статус | Примечание |
|---|---|---|
| ELM327 USB (v1.5, v2.1) | ✓ | у многих клонов «v2.1» урезан набор команд; клоны v1.5 часто надёжнее |
| ELM327 Bluetooth | ✓ | сопрягите в Windows и выберите исходящий COM-порт |
| ELM327 Wi-Fi | ✓ | обычно 192.168.0.10:35000 |
| ELM327 Bluetooth LE (BLE 4.0): Vgate iCar Pro BLE, Veepeak OBDCheck BLE, vLinker MC+ / FS BLE, KONNWEI KW902 | ~ | Windows и Linux: адаптер «ELM327 Bluetooth LE» → «Найти адаптеры»; на настоящих адаптерах ещё не проверялось; в сборке для macOS нет |
| ELM323 | ~ | только OBD-II по K-Line; не проверено |
| OBDLink SX, OBDLink S | ✓ | чип STN, совместим с ELM327 |
| OBDLink LX, MX+ (Bluetooth) | ✓ | |
| OBDLink EX | ✓ | |
| OBDLink CX | ~ | только Bluetooth LE — адаптер «ELM327 Bluetooth LE»; ещё не проверялось |
| Адаптеры на STN1110 / STN2120 | ✓ | |
| Ford ELMConfig Adapter, FORScan Adapter | ~ | HS-CAN работает как ELM327; переключение на MS-CAN не поддерживается |

ELM327 говорит только на стандартных протоколах OBD-II и KWP2000. Ранний
протокол Микас-5.4/7.1 (9600 бод) и VW KW1281 через ELM327 недоступны —
для них нужен кабель KKL.

## J2534 PassThru

| Адаптер | Статус | Примечание |
|---|---|---|
| Tactrix OpenPort 2.0 | J | драйвер 32-битный → `udt-x86.exe` |
| Scanmatik 2, Scanmatik 2 Pro, Scanmatik 3 | J | |
| Godiag GD101 | J | |
| Toyota Mini-VCI, Toyota Techstream J2534 | J | драйвер 32-битный |
| Toyota Intelligent Tester II / III, Techstream Lite / 2.0 | ~ | J, если интерфейс установил драйвер J2534 |
| Ford VCM II, VCM III, Ford FJDS J2534 | J | |
| Ford VCM, Ford VMM | ~ | зависит от наличия драйвера J2534 |
| GM MDI, MDI 2, GM GDS2 Interface, Opel MDI | J | |
| Bosch MDI, Bosch M-VCI | J | |
| Honda MVCI, Honda DST-i, Denso DST-i | J | |
| Honda HIM | ~ | J, если установлен драйвер J2534 |
| Mazda VCM, Mazda IDS Interface, Mazda M-MDS | J | |
| Hyundai / Kia GDS VCI | ~ | VCI II — J; первая версия — закрытый протокол |
| Subaru SSM Interface, Select Monitor III / IV | ~ | J для OBD-II и UDS; собственный протокол Subaru SSM не реализован |
| Chrysler wiTECH Micropod II | J | |
| Autel MaxiFlash | J | |
| Autel MaxiVCI (VCI100 / VCI200 / VCI300), VCI Mini | ~ | J только у моделей с драйвером J2534 |
| TOPDON RLink, RLink Lite, RLink X7 | J | |
| Launch SmartLink C | ~ | J, если установлен драйвер |
| VXDIAG, Xhorse MVCI | J | |
| VAS 6154A / 6154B, ODIS Interface | ~ | J, если установлен драйвер PassThru |
| VAS 5054A | ✕ | работает через D-PDU API (ISO 22900-2) — пока не поддерживается |
| Bosch KTS 520 / 540 / 550 / 560 / 570 / 590 | ~ | J у моделей с драйвером PassThru; остальные работают только с ESI[tronic] |
| Intrepid ValueCAN, neoVI | J | через драйвер J2534 от Intrepid |
| Isuzu IDSS, Isuzu USB Link | J / R | обычно это Nexiq USB-Link (см. грузовые) |

Через J2534 программа умеет: OBD-II (CAN, ISO 9141-2, ISO 14230-4, J1850
VPW и PWM), KWP2000 по K-Line с любым адресом, UDS по CAN, VW TP 2.0,
J1939 и монитор шины CAN.

## CAN-адаптеры

| Адаптер | Статус | Примечание |
|---|---|---|
| CANable (прошивка slcan), CANtact | ✓ | |
| CANable 2.0 (прошивка slcan с CAN FD) | ✓ | в том числе CAN FD: данные 2 или 5 Мбит/с; прошивка не подтверждает команды — программа это учитывает |
| CANable (прошивка candleLight) | ~ | на Linux — через SocketCAN ✓; на Windows прошейте slcan |
| Lawicel CANUSB | ✓ | |
| USB-CAN / CAN-USB с протоколом slcan | ✓ | |
| дешёвые «USB-CAN Analyzer» (CH341, свой протокол) | ✕ | протокол не slcan |
| PEAK PCAN-USB, PCAN-USB FD, PEAK PCAN | ✓ | драйвер PCAN-Basic; PCAN-USB FD — также CAN FD (данные 1–8 Мбит/с) |
| Kvaser Leaf, Kvaser USBcan | ✓ | драйвер CANlib |
| Любой адаптер SocketCAN на Linux (в том числе MCP2515 на Raspberry Pi) | ✓ | |
| Vector CANcaseXL, VN1610, VN1630, VN5610 | ✕ | XL Driver Library пока не поддерживается |
| Адаптеры CAN FD | ~ | UDS по CAN FD: PCAN-USB FD, CANable 2.0 и SocketCAN с FD (проверено на симуляторе); Kvaser и J2534 — только классический CAN |

## Ethernet

| Адаптер | Статус | Примечание |
|---|---|---|
| Кабель OBD–RJ45 (DoIP, ISO 13400) | ✓ | VAG MQB evo, Volvo SPA, JLR, Mercedes, электромобили |
| BMW ENET | ✓ | серии F, G, I (HSFZ, порт 6801) |
| BMW ICOM, ICOM A / B / C, ICOM Next, Next A / B | ✕ | для F/G/I используйте ENET-кабель; для серии E — K+DCAN |
| BMW OPS, OPPS, DIS Interface | ✕ | |

## Дилерские интерфейсы с закрытым протоколом

| Адаптер | Статус | Примечание |
|---|---|---|
| Mercedes-Benz STAR C3, C4, C5, SDconnect | ✕ | |
| Mercedes-Benz STAR C6, XENTRY VCI, XENTRY Diagnosis VCI | ~ | J, если установлен драйвер J2534 |
| Renault CAN Clip, Renault Reprog Interface | ✕ | |
| Renault Alliance VI, VI3, Nissan VI2 | ~ | J, если установлен драйвер J2534 |
| PSA Lexia-3, PP2000, Diagbox Interface, XS Evolution, Actia XS | ✕ | |
| Opel OP-COM | ✕ | |
| GM Tech 2, Chrysler StarSCAN, StarMOBILE, DRB III | ✕ | самостоятельные приборы |
| Nissan Consult-I, II, III, III+ | ✕ | собственный протокол Nissan |
| Mitsubishi MUT-II Interface, MUT-III | ✕ | протокол MUT не реализован; OBD-II — через KKL или ELM327 |
| Suzuki SDT-II, SDT-2 | ~ | J, если установлен драйвер J2534 |
| Hino DX Interface | R | обычно работает через RP1210-адаптер |

## Сканеры и приложения с собственным адаптером

Эти адаптеры работают только со своей программой или приложением на
телефоне: протокол закрыт или привязан к серверу производителя.

| Адаптер | Статус |
|---|---|
| Launch X431, DBScar, DBScar VII | ✕ |
| Autel (планшеты MaxiSys и их VCI без J2534) | ✕ |
| TOPDON (сканеры) | ✕ |
| Foxwell NT-series | ✕ |
| Thinkcar ThinkLink, Thinkdiag, Thinkdiag 2 | ✕ |
| OTOFIX VCI, OTOFIX D1 | ✕ |
| BlueDriver, Carista EVO, OBDeleven, Carly Universal Adapter, FIXD Sensor | ✕ |
| ANCEL VCI, iCarsoft VCI | ✕ |
| Delphi DS150E, Autocom CDP+, Würth WOW Snooper | ✕ |

## Грузовики, автобусы, спецтехника

| Адаптер | Статус | Примечание |
|---|---|---|
| Nexiq USB-Link, USB-Link 2, USB-Link 3 | R | драйвер обычно 32-битный → `udt-x86.exe` |
| Noregon DLA+, DLA+ 2.0 | R | |
| Dearborn DPA 4 Plus, DPA 5, DPA XL, CNH DPA5 | R | |
| Cummins INLINE 5, 6, 7, INLINE Mini | R | |
| CAT Communication Adapter II, 3, 3+ | R | |
| Jaltest Link, Link V9 | R | |
| TEXA Navigator TXB, Nano S, TXT, TXTs, TXB Evolution | ~ | R или J, если установлен драйвер |
| Volvo VOCOM, VOCOM II, VOCOM 2, 88890300 | ~ | R или J, если установлен драйвер |
| Volvo DiCE | J | |
| Scania VCI1 – VCI4 | ~ | зависит от драйвера |
| DAF VCI, DAVIE Interface | ~ | зависит от драйвера |
| MAN-cats II / III, MAN T200, Iveco Eltrac, Iveco EASY | ✕ | |
| Allison DOC, Eaton ServiceRanger, Perkins EST, JCB ServiceMaster | R | это программы; работают через RP1210-адаптер — тот же адаптер подходит и сюда |
| WABCO, Knorr-Bremse, Haldex | ~ | J1939 — через RP1210-адаптер; фирменные функции ✕ |
| John Deere Service ADVISOR VCI, EDL | ~ | J1939 — если есть драйвер RP1210 |
| Kubota Diagmaster, Komatsu, Hitachi Dr. ZX, Yanmar, MTU DiaSys, Deutz SerDia, Parker IQAN | ~ | общие параметры J1939 — через любой RP1210/CAN-адаптер; фирменные функции ✕ |

Через RP1210 программа работает по SAE J1939: параметры, активные и
сохранённые ошибки, стирание. J1708/J1587 (6-контактный Deutsch) пока не
поддерживается.

## Мотоциклы

| Адаптер | Статус | Примечание |
|---|---|---|
| Kawasaki KDS, Suzuki SDS, Honda, Yamaha | ~ | многие используют KWP2000 по K-Line: KKL + переходник на разъём мотоцикла + профиль «KWP2000 — свой адрес»; не проверено |
| BMW Motorrad, Ducati DDS, KTM, Harley-Davidson Digital Technician | ✕ | |

## Прошивка, ключи, программаторы — вне задач

KESS, KESS V2, KESS3, K-TAG, K-TAG Master, MPPS (V13 – V18), CMDFlash,
AutoTuner, Magicmotorsport FLEX, Dimsport New Genius и New Trasdata,
PCMflash, BitBox, Combiloader, OpenECU, EcuTek, HP Tuners MPVI2 / MPVI3, EFI
Live FlashScan / AutoCal, SCT X4 / BDX, DiabloSport Trinity / Predator,
Alientech Powergate, FoxFlash, KT200, FlexBox, DFOX, Trasdata, CMD Bench —
инструменты для прошивки блоков. Если такое устройство устанавливает драйвер
J2534, программа сможет читать через него ошибки и параметры; прошивки в
программе нет (см. [README](../README.md#честно-о-возможностях)).

Xhorse VVDI Prog / VVDI2 / Multi Prog, CGDI Prog / MB / BMW, Orange5,
XPROG-M, CarProg, UPA-USB, TL866, RT809H, SMOK JTAG, Abrites AVDI, Zed-FULL,
Tango, Lonsdor K518, Autel XP400 Pro и IM608, OBDSTAR X300 DP Plus и Key
Master, VVDI Key Tool Plus, XTOOL KC501 / KC100 — программаторы памяти, ключей
и иммобилайзеров. Программа с ними не работает и работать не будет.

Godiag GT100 и GT105 — коммутационные коробки и переходники для стола; это не
адаптеры связи, с ними работает любой адаптер из таблиц выше.

---
---

<a id="eng"></a>

[Русский](#ru) · **English**

# Adapter compatibility

Hundreds of diagnostic adapters come down to a few interface types. If the
program supports the type, it supports the adapter — so the tables below are
grouped by type.

> **Honestly.** Every interface type is covered by automated tests against
> the built-in simulator, but **no adapter in these tables has been tried on
> a real vehicle yet**. "✓" means "the program supports it", not "tested on
> hardware". If you test your adapter, please report in
> [issues](https://github.com/AristarhUcolov/universal-diagnostic-tool/issues)
> and it will be marked as tested.

**Legend**

| Mark | Meaning |
|---|---|
| ✓ | supported: the driver for this type is built into the program |
| J | through the J2534 PassThru driver installed by the adapter's maker |
| R | through the RP1210 driver (truck adapters) |
| ~ | partly or unknown: works in one of its modes, depends on firmware or driver, untested |
| ✕ | not supported: closed protocol, the adapter works only with its own software |
| — | out of scope: ECU flashing, keys, immobilisers, odometers, memory programmers |

**Checking your adapter.** `udt adapters` (or the Connect page) lists the
J2534 and RP1210 drivers installed in Windows and their bitness. Most of them
are 32-bit — use `udt-x86.exe` for those. If your adapter is not in the
driver list, its maker provides no standard driver, and no third-party
program, this one included, can use it.

## K-Line cables (KKL)

| Adapter | Status | Note |
|---|---|---|
| VAG KKL 409.1 with FTDI (FT232RL) | ✓ | the best choice for K-Line; set Latency Timer to 1 ms |
| VAG KKL with CH340 / CH341 | ✓ | usually works; 5-baud init can be unreliable on some clones |
| VAG KKL RS232 (serial port) | ✓ | the circuit is often powered from DTR — keep "DTR on" |
| K+L-Line cables | ✓ | the L-Line is driven by RTS |
| VAG-COM (409.1 cable) | ✓ | the same KKL cable |
| BMW K+DCAN, BMW INPA Interface | ~ | K-Line mode works as a KKL; D-CAN mode is not supported |
| GAZ/VAZ/UAZ 12-pin → OBD-II adapters | ✓ | with any KKL; check the wiring — [GAZ-VOLGA.md](GAZ-VOLGA.md) |
| Tactrix OpenPort 1.3U | ~ | FTDI USB K-Line cable; may work as a KKL, untested |
| Tactrix OpenPort 1.3 | ~ | same |
| K+CAN Commander 1.4 | ~ | K-Line as a KKL; CAN uses a closed protocol, ✕ |
| AutoDia K509 | ~ | K-Line cable; untested |
| Galletto 1260, FGTech Galletto | ~ | FTDI inside; K-Line mode may work as a KKL; flashing is out of scope |
| VCP (VAG CAN PRO) | ✕ | closed protocol, VCP software only |
| VCDS HEX-V2, HEX-NET, HEX+CAN | ✕ | work only with VCDS (Ross-Tech) |
| VAG Commander | — | this is software; its KKL cable works here too |
| VagTacho, VAG EEPROM Programmer | — | odometer and immobiliser — out of scope |

## ELM327 and compatibles

| Adapter | Status | Note |
|---|---|---|
| ELM327 USB (v1.5, v2.1) | ✓ | many "v2.1" clones lack commands; v1.5 clones are often more reliable |
| ELM327 Bluetooth | ✓ | pair it in Windows and pick the outgoing COM port |
| ELM327 Wi-Fi | ✓ | usually 192.168.0.10:35000 |
| ELM327 Bluetooth LE (BLE 4.0): Vgate iCar Pro BLE, Veepeak OBDCheck BLE, vLinker MC+ / FS BLE, KONNWEI KW902 | ~ | Windows and Linux: adapter "ELM327 Bluetooth LE" → "Find adapters"; not tried on real adapters yet; not in the macOS build |
| ELM323 | ~ | OBD-II on K-Line only; untested |
| OBDLink SX, OBDLink S | ✓ | STN chip, ELM327-compatible |
| OBDLink LX, MX+ (Bluetooth) | ✓ | |
| OBDLink EX | ✓ | |
| OBDLink CX | ~ | Bluetooth LE only — adapter "ELM327 Bluetooth LE"; untested |
| STN1110 / STN2120 adapters | ✓ | |
| Ford ELMConfig Adapter, FORScan Adapter | ~ | HS-CAN works as an ELM327; switching to MS-CAN is not supported |

An ELM327 speaks only the standard OBD-II and KWP2000 protocols. The early
Микас-5.4/7.1 protocol (9600 baud) and VW KW1281 are not reachable through
an ELM327 — they need a KKL cable.

## J2534 PassThru

| Adapter | Status | Note |
|---|---|---|
| Tactrix OpenPort 2.0 | J | 32-bit driver → `udt-x86.exe` |
| Scanmatik 2, Scanmatik 2 Pro, Scanmatik 3 | J | |
| Godiag GD101 | J | |
| Toyota Mini-VCI, Toyota Techstream J2534 | J | 32-bit driver |
| Toyota Intelligent Tester II / III, Techstream Lite / 2.0 | ~ | J if the interface installed a J2534 driver |
| Ford VCM II, VCM III, Ford FJDS J2534 | J | |
| Ford VCM, Ford VMM | ~ | depends on a J2534 driver |
| GM MDI, MDI 2, GM GDS2 Interface, Opel MDI | J | |
| Bosch MDI, Bosch M-VCI | J | |
| Honda MVCI, Honda DST-i, Denso DST-i | J | |
| Honda HIM | ~ | J if a J2534 driver is installed |
| Mazda VCM, Mazda IDS Interface, Mazda M-MDS | J | |
| Hyundai / Kia GDS VCI | ~ | VCI II — J; the first version uses a closed protocol |
| Subaru SSM Interface, Select Monitor III / IV | ~ | J for OBD-II and UDS; Subaru's own SSM protocol is not implemented |
| Chrysler wiTECH Micropod II | J | |
| Autel MaxiFlash | J | |
| Autel MaxiVCI (VCI100 / VCI200 / VCI300), VCI Mini | ~ | J only for models with a J2534 driver |
| TOPDON RLink, RLink Lite, RLink X7 | J | |
| Launch SmartLink C | ~ | J if a driver is installed |
| VXDIAG, Xhorse MVCI | J | |
| VAS 6154A / 6154B, ODIS Interface | ~ | J if the PassThru driver is installed |
| VAS 5054A | ✕ | uses the D-PDU API (ISO 22900-2) — not supported yet |
| Bosch KTS 520 / 540 / 550 / 560 / 570 / 590 | ~ | J for models with a PassThru driver; the rest work only with ESI[tronic] |
| Intrepid ValueCAN, neoVI | J | through Intrepid's J2534 driver |
| Isuzu IDSS, Isuzu USB Link | J / R | usually a Nexiq USB-Link (see trucks) |

Through J2534 the program does OBD-II (CAN, ISO 9141-2, ISO 14230-4, J1850
VPW and PWM), KWP2000 on K-Line at any address, UDS on CAN, VW TP 2.0, J1939
and the CAN bus monitor.

## CAN adapters

| Adapter | Status | Note |
|---|---|---|
| CANable (slcan firmware), CANtact | ✓ | |
| CANable 2.0 (slcan firmware with CAN FD) | ✓ | also CAN FD: data phase 2 or 5 Mbit/s; the firmware does not acknowledge commands, which the program allows for |
| CANable (candleLight firmware) | ~ | on Linux through SocketCAN ✓; on Windows flash slcan |
| Lawicel CANUSB | ✓ | |
| USB-CAN / CAN-USB speaking slcan | ✓ | |
| cheap "USB-CAN Analyzer" (CH341, own protocol) | ✕ | not slcan |
| PEAK PCAN-USB, PCAN-USB FD, PEAK PCAN | ✓ | PCAN-Basic driver; the PCAN-USB FD also on CAN FD (data 1–8 Mbit/s) |
| Kvaser Leaf, Kvaser USBcan | ✓ | CANlib driver |
| Any SocketCAN adapter on Linux (including MCP2515 on a Raspberry Pi) | ✓ | |
| Vector CANcaseXL, VN1610, VN1630, VN5610 | ✕ | the XL Driver Library is not supported yet |
| CAN FD adapters | ~ | UDS on CAN FD: PCAN-USB FD, CANable 2.0 and SocketCAN with FD (tested against the simulator); Kvaser and J2534 classic CAN only |

## Ethernet

| Adapter | Status | Note |
|---|---|---|
| OBD–RJ45 cable (DoIP, ISO 13400) | ✓ | VAG MQB evo, Volvo SPA, JLR, Mercedes, electric vehicles |
| BMW ENET | ✓ | F, G, I series (HSFZ, port 6801) |
| BMW ICOM, ICOM A / B / C, ICOM Next, Next A / B | ✕ | use an ENET cable for F/G/I, K+DCAN for the E series |
| BMW OPS, OPPS, DIS Interface | ✕ | |

## Dealer interfaces with closed protocols

| Adapter | Status | Note |
|---|---|---|
| Mercedes-Benz STAR C3, C4, C5, SDconnect | ✕ | |
| Mercedes-Benz STAR C6, XENTRY VCI, XENTRY Diagnosis VCI | ~ | J if a J2534 driver is installed |
| Renault CAN Clip, Renault Reprog Interface | ✕ | |
| Renault Alliance VI, VI3, Nissan VI2 | ~ | J if a J2534 driver is installed |
| PSA Lexia-3, PP2000, Diagbox Interface, XS Evolution, Actia XS | ✕ | |
| Opel OP-COM | ✕ | |
| GM Tech 2, Chrysler StarSCAN, StarMOBILE, DRB III | ✕ | stand-alone testers |
| Nissan Consult-I, II, III, III+ | ✕ | Nissan's own protocol |
| Mitsubishi MUT-II Interface, MUT-III | ✕ | the MUT protocol is not implemented; OBD-II through a KKL or ELM327 |
| Suzuki SDT-II, SDT-2 | ~ | J if a J2534 driver is installed |
| Hino DX Interface | R | usually works through an RP1210 adapter |

## Scanners and apps with their own adapter

These adapters work only with their own software or phone app: the protocol
is closed or tied to the maker's server.

| Adapter | Status |
|---|---|
| Launch X431, DBScar, DBScar VII | ✕ |
| Autel (MaxiSys tablets and their VCIs without J2534) | ✕ |
| TOPDON (scanners) | ✕ |
| Foxwell NT-series | ✕ |
| Thinkcar ThinkLink, Thinkdiag, Thinkdiag 2 | ✕ |
| OTOFIX VCI, OTOFIX D1 | ✕ |
| BlueDriver, Carista EVO, OBDeleven, Carly Universal Adapter, FIXD Sensor | ✕ |
| ANCEL VCI, iCarsoft VCI | ✕ |
| Delphi DS150E, Autocom CDP+, Würth WOW Snooper | ✕ |

## Trucks, buses, machinery

| Adapter | Status | Note |
|---|---|---|
| Nexiq USB-Link, USB-Link 2, USB-Link 3 | R | the driver is usually 32-bit → `udt-x86.exe` |
| Noregon DLA+, DLA+ 2.0 | R | |
| Dearborn DPA 4 Plus, DPA 5, DPA XL, CNH DPA5 | R | |
| Cummins INLINE 5, 6, 7, INLINE Mini | R | |
| CAT Communication Adapter II, 3, 3+ | R | |
| Jaltest Link, Link V9 | R | |
| TEXA Navigator TXB, Nano S, TXT, TXTs, TXB Evolution | ~ | R or J if a driver is installed |
| Volvo VOCOM, VOCOM II, VOCOM 2, 88890300 | ~ | R or J if a driver is installed |
| Volvo DiCE | J | |
| Scania VCI1 – VCI4 | ~ | depends on the driver |
| DAF VCI, DAVIE Interface | ~ | depends on the driver |
| MAN-cats II / III, MAN T200, Iveco Eltrac, Iveco EASY | ✕ | |
| Allison DOC, Eaton ServiceRanger, Perkins EST, JCB ServiceMaster | R | these are programs running on an RP1210 adapter — the same adapter works here |
| WABCO, Knorr-Bremse, Haldex | ~ | J1939 through an RP1210 adapter; brand-specific functions ✕ |
| John Deere Service ADVISOR VCI, EDL | ~ | J1939 if an RP1210 driver exists |
| Kubota Diagmaster, Komatsu, Hitachi Dr. ZX, Yanmar, MTU DiaSys, Deutz SerDia, Parker IQAN | ~ | generic J1939 values through any RP1210/CAN adapter; brand-specific functions ✕ |

Through RP1210 the program speaks SAE J1939: values, active and stored codes,
clearing. J1708/J1587 (6-pin Deutsch) is not supported yet.

## Motorcycles

| Adapter | Status | Note |
|---|---|---|
| Kawasaki KDS, Suzuki SDS, Honda, Yamaha | ~ | many use KWP2000 on K-Line: a KKL, an adapter to the bike's connector and the "KWP2000 — custom address" profile; untested |
| BMW Motorrad, Ducati DDS, KTM, Harley-Davidson Digital Technician | ✕ | |

## Flashing, keys, programmers — out of scope

KESS, KESS V2, KESS3, K-TAG, K-TAG Master, MPPS (V13 – V18), CMDFlash,
AutoTuner, Magicmotorsport FLEX, Dimsport New Genius and New Trasdata,
PCMflash, BitBox, Combiloader, OpenECU, EcuTek, HP Tuners MPVI2 / MPVI3, EFI
Live FlashScan / AutoCal, SCT X4 / BDX, DiabloSport Trinity / Predator,
Alientech Powergate, FoxFlash, KT200, FlexBox, DFOX, Trasdata, CMD Bench are
ECU flashing tools. If such a device installs a J2534 driver, the program can
read codes and values through it; there is no flashing in the program (see
the [README](../README.md#honestly-about-the-limits)).

Xhorse VVDI Prog / VVDI2 / Multi Prog, CGDI Prog / MB / BMW, Orange5,
XPROG-M, CarProg, UPA-USB, TL866, RT809H, SMOK JTAG, Abrites AVDI, Zed-FULL,
Tango, Lonsdor K518, Autel XP400 Pro and IM608, OBDSTAR X300 DP Plus and Key
Master, VVDI Key Tool Plus, XTOOL KC501 / KC100 are memory, key and
immobiliser programmers. The program does not and will not work with them.

Godiag GT100 and GT105 are breakout boxes and bench adapters, not
communication adapters; any adapter from the tables above works with them.
