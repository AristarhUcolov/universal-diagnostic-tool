<a id="ru"></a>

**Русский** · [English](#eng)

# Протоколы и «конвертер»

**Разделы:** [Как устроена программа](#как-устроена-программа) ·
[Протоколы](#протоколы) · [Автоопределение](#автоопределение) ·
[Что конвертируется](#что-конвертируется-а-что-нет) ·
[Мост ELM327](#мост-elm327)

---

## Как устроена программа

```
 адаптер          KKL · ELM327 · J2534 · slcan · PCAN · Kvaser · SocketCAN · RP1210 · Ethernet
    │
 физическая линия  K-Line (ISO 9141) · CAN · J1850 · Ethernet
    │
 транспорт        K-Line: KWP2000-кадры, ISO 9141-2, Микас, KW1281
                  CAN: ISO-TP (ISO 15765-2), VW TP 2.0, J1939 BAM
                  Ethernet: DoIP (ISO 13400), BMW HSFZ
    │
 канал            Send / Recv: «запрос — ответ» с ожиданием 0x78 и повторами
    │
 прикладной       OBD-II (SAE J1979) · KWP2000 (ISO 14230-3) · UDS (ISO 14229)
 протокол         Микас / Январь-4 · KW1281 · J1939 (DM1/DM2/DM3/DM11)
    │
 блок управления  описание из JSON: адреса, запросы, формулы, коды
    │
 интерфейс        окно программы · командная строка · мост ELM327
```

Каждый уровень знает только соседей. Поэтому, например, обработчик OBD-II
один и тот же для K-Line, J1850, CAN и Ethernet, а описание блока Январь-7.2
работает через KKL-кабель, ELM327 и J2534 без изменений.

## Протоколы

| Протокол | Где встречается | Статус | Через какие адаптеры |
|---|---|---|---|
| SAE J1979 / ISO 15031 (OBD-II, EOBD) | все автомобили с 1996–2004 года | ✓ | все |
| ISO 9141-2 (K-Line, 10400 бод, 5-бодовая инициализация) | европейские и азиатские машины до ~2008 | ✓ | KKL, ELM327, J2534 |
| ISO 14230 (KWP2000) на K-Line | ВАЗ/ГАЗ/УАЗ, многие европейские машины | ✓ | KKL, ELM327, J2534 |
| ISO 15765-2 / -4 (CAN, 11 и 29 бит, 250 / 500 кбит/с) | все машины с ~2008 года | ✓ | ELM327, J2534, slcan, PCAN, Kvaser, SocketCAN |
| ISO 14229 (UDS) | современные блоки | ✓ | CAN-адаптеры, J2534, ELM327, DoIP, HSFZ |
| SAE J1850 PWM и VPW | Ford, GM, Chrysler до ~2008 | ✓ только OBD-II | ELM327, J2534 |
| Ранний протокол Микас-5.4/7.1, Январь-4 | ГАЗ, УАЗ, ВАЗ 1990-х — начала 2000-х | ✓ | KKL |
| VW KW1281 | VW, Audi, Seat, Skoda до ~2005 | ✓ | KKL |
| VW TP 2.0 (KWP2000 по CAN) | VAG 2004–2012 | экспериментально | J2534, slcan, PCAN, Kvaser, SocketCAN |
| SAE J1939 | грузовики, автобусы, тракторы | ✓ | CAN-адаптеры, J2534, RP1210 |
| ISO 13400 (DoIP) | машины с диагностикой по Ethernet | ✓ (проверено на симуляторе) | кабель OBD–RJ45 |
| BMW HSFZ | BMW серий F, G, I | ✓ (проверено на симуляторе) | ENET-кабель |
| L-Line | инициализация на части старых машин | ✓ | кабели K+L (через RTS) |
| CAN FD | новые машины | ✕ | — |
| SAE J1708 / J1587 | старые грузовики | ✕ | — |
| GM ALDL (160 и 8192 бод) | GM до 1996 | ✕ | — |
| Chrysler SCI, CCD | Chrysler до ~2004 | ✕ | — |
| Nissan Consult, Subaru SSM, Mitsubishi MUT | фирменные протоколы | ✕ | — |
| FlexRay, MOST, LIN | внутренние шины машины | ✕ | см. ниже |
| ISO 22900 (MVCI, D-PDU API) | дилерские интерфейсы | ✕ | — |

FlexRay, MOST и LIN — внутренние шины: диагностический разъём к ним не
подключён, а блоки на них отвечают через шлюз по CAN или Ethernet. Поэтому для
диагностики эти шины обычно и не нужны. Chrysler PCI — это J1850 VPW, Ford
SCP — J1850 PWM: OBD-II по ним работает.

## Автоопределение

Профиль «автоматически» перебирает варианты в таком порядке:

- **K-Line кабель:** OBD-II по ISO 14230-4 → OBD-II по ISO 9141-2 (5 бод) →
  KWP2000 по адресу 0x10 с быстрой инициализацией → ранний протокол Микас
  (9600 бод) → VW KW1281.
- **ВАЗ / ГАЗ / УАЗ — автоматически:** KWP2000 (0x10) → Микас → OBD-II.
- **CAN-адаптеры:** 500 кбит/с, затем 250 кбит/с (для J1939 — наоборот);
  OBD-II с 11-битными, затем 29-битными идентификаторами; затем
  прослушивание J1939.
- **J2534:** CAN → K-Line (ISO 14230, ISO 9141) → J1850 VPW и PWM.
- **ELM327:** протокол выбирает сам адаптер (ATSP0).

Каждый шаг виден в «Ходе подключения» и в журнале.

## Что конвертируется, а что нет

**Конвертируется — программно:**

1. **Один протокол поверх разных линий.** KWP2000 — по K-Line или по CAN (VW
   TP 2.0); OBD-II — по пяти физическим линиям; UDS — по CAN или Ethernet.
   Программа делает это сама: выше канала код один.
2. **Разные блоки — в один вид.** Ошибки Микас, KWP2000, UDS, KW1281 и J1939
   показываются одной таблицей; параметры — в одних единицах с одинаковыми
   графиками; всё попадает в один журнал.
3. **Блок без OBD-II — в OBD-II автомобиль** для сторонних приложений: см.
   [мост ELM327](#мост-elm327).

**Не конвертируется — нужна железка:**

- **K-Line ↔ CAN.** Это разные провода (K-Line — одна линия до +12 В, CAN —
  дифференциальная пара 2,5 В ± 1 В) и разные микросхемы в адаптере. Кабель
  KKL физически не может принять кадр CAN, а CAN-адаптер не может «выдать»
  K-Line. Блок на K-Line не появится на шине CAN без отдельного шлюза.
- **Закрытые адаптеры.** Если производитель зашифровал протокол между
  программой и адаптером, перевести его нельзя — см.
  [ADAPTERS.md](ADAPTERS.md).

Иначе говоря: программа переводит между **протоколами** на любом адаптере,
который физически подключён к нужной **линии**.

## Мост ELM327

Многие программы и приложения понимают только ELM327. Мост делает из
текущего подключения сетевой ELM327 (как Wi-Fi адаптер): **Настройки → Мост
ELM327 → Запустить** или `udt bridge`. В приложении выберите Wi-Fi ELM327 и
укажите адрес компьютера и порт (по умолчанию 35000).

- Если машина сама говорит на OBD-II, запросы проходят насквозь.
- Если нет (Микас, Январь, KW1281), программа строит ответы OBD-II из
  параметров блока: температура охлаждающей жидкости → PID 05, обороты →
  PID 0C, напряжение → PID 42 и так далее — по полю `obd` в описании блока.
- Число ошибок и лампа MIL передаются через PID 01; режим 04 стирает ошибки.
- Коды, у которых нет аналога SAE (например, трёхзначные коды Микас), в
  режиме 03 не передаются — их видно только в самой программе.

---
---

<a id="eng"></a>

[Русский](#ru) · **English**

# Protocols and the "converter"

## How the program is built

```
 adapter           KKL · ELM327 · J2534 · slcan · PCAN · Kvaser · SocketCAN · RP1210 · Ethernet
    │
 physical line     K-Line (ISO 9141) · CAN · J1850 · Ethernet
    │
 transport         K-Line: KWP2000 frames, ISO 9141-2, Микас, KW1281
                   CAN: ISO-TP (ISO 15765-2), VW TP 2.0, J1939 BAM
                   Ethernet: DoIP (ISO 13400), BMW HSFZ
    │
 channel           Send / Recv: request and response, waiting on 0x78, retries
    │
 application       OBD-II (SAE J1979) · KWP2000 (ISO 14230-3) · UDS (ISO 14229)
 protocol          Микас / Январь-4 · KW1281 · J1939 (DM1/DM2/DM3/DM11)
    │
 control unit      JSON description: addresses, requests, formulas, codes
    │
 interface         program window · command line · ELM327 bridge
```

Each layer only knows its neighbours. That is why the OBD-II handler is the
same for K-Line, J1850, CAN and Ethernet, and why the Январь-7.2 description
works through a KKL cable, an ELM327 and a J2534 interface unchanged.

## Protocols

| Protocol | Where | Status | Through which adapters |
|---|---|---|---|
| SAE J1979 / ISO 15031 (OBD-II, EOBD) | every car since 1996–2004 | ✓ | all |
| ISO 9141-2 (K-Line, 10400 baud, 5-baud init) | European and Asian cars until ~2008 | ✓ | KKL, ELM327, J2534 |
| ISO 14230 (KWP2000) on K-Line | VAZ/GAZ/UAZ, many European cars | ✓ | KKL, ELM327, J2534 |
| ISO 15765-2 / -4 (CAN, 11 and 29 bit, 250 / 500 kbit/s) | every car since ~2008 | ✓ | ELM327, J2534, slcan, PCAN, Kvaser, SocketCAN |
| ISO 14229 (UDS) | modern control units | ✓ | CAN adapters, J2534, ELM327, DoIP, HSFZ |
| SAE J1850 PWM and VPW | Ford, GM, Chrysler until ~2008 | ✓ OBD-II only | ELM327, J2534 |
| Early Микас-5.4/7.1, Январь-4 protocol | GAZ, UAZ, VAZ of the 1990s and early 2000s | ✓ | KKL |
| VW KW1281 | VW, Audi, Seat, Skoda until ~2005 | ✓ | KKL |
| VW TP 2.0 (KWP2000 on CAN) | VAG 2004–2012 | experimental | J2534, slcan, PCAN, Kvaser, SocketCAN |
| SAE J1939 | trucks, buses, tractors | ✓ | CAN adapters, J2534, RP1210 |
| ISO 13400 (DoIP) | cars with Ethernet diagnostics | ✓ (simulator-tested) | OBD–RJ45 cable |
| BMW HSFZ | BMW F, G, I series | ✓ (simulator-tested) | ENET cable |
| L-Line | initialisation on some older cars | ✓ | K+L cables (through RTS) |
| CAN FD | new cars | ✕ | — |
| SAE J1708 / J1587 | older trucks | ✕ | — |
| GM ALDL (160 and 8192 baud) | GM before 1996 | ✕ | — |
| Chrysler SCI, CCD | Chrysler until ~2004 | ✕ | — |
| Nissan Consult, Subaru SSM, Mitsubishi MUT | brand protocols | ✕ | — |
| FlexRay, MOST, LIN | in-vehicle buses | ✕ | see below |
| ISO 22900 (MVCI, D-PDU API) | dealer interfaces | ✕ | — |

FlexRay, MOST and LIN are internal buses: the diagnostic connector is not
wired to them, and their control units answer through a gateway over CAN or
Ethernet, so diagnostics rarely needs them. Chrysler PCI is J1850 VPW and Ford
SCP is J1850 PWM: OBD-II works over them.

## Auto-detection

The "automatic" profile tries, in this order:

- **K-Line cable:** OBD-II on ISO 14230-4 → OBD-II on ISO 9141-2 (5 baud) →
  KWP2000 at address 0x10 with fast init → the early Микас protocol
  (9600 baud) → VW KW1281.
- **VAZ / GAZ / UAZ — automatic:** KWP2000 (0x10) → Микас → OBD-II.
- **CAN adapters:** 500 kbit/s, then 250 kbit/s (the other way round for
  J1939); OBD-II with 11-bit, then 29-bit identifiers; then listening for
  J1939.
- **J2534:** CAN → K-Line (ISO 14230, ISO 9141) → J1850 VPW and PWM.
- **ELM327:** the adapter picks the protocol itself (ATSP0).

Every step shows up in the connection progress and in the journal.

## What converts and what does not

**Converts — in software:**

1. **One protocol over different lines.** KWP2000 over K-Line or over CAN (VW
   TP 2.0); OBD-II over five physical lines; UDS over CAN or Ethernet. The
   program does this by itself: above the channel the code is the same.
2. **Different control units into one view.** Микас, KWP2000, UDS, KW1281 and
   J1939 codes share one table; values share units and charts; everything
   goes into one journal.
3. **A unit without OBD-II into an OBD-II car** for third-party apps: see the
   [ELM327 bridge](#elm327-bridge).

**Does not convert — needs hardware:**

- **K-Line ↔ CAN.** These are different wires (K-Line is a single line up to
  +12 V, CAN a differential pair at 2.5 V ± 1 V) and different chips in the
  adapter. A KKL cable physically cannot receive a CAN frame, and a CAN
  adapter cannot drive a K-Line. A K-Line control unit will not appear on a
  CAN bus without a separate gateway.
- **Locked adapters.** If the maker encrypted the protocol between its
  software and the adapter, there is nothing to translate — see
  [ADAPTERS.md](ADAPTERS.md).

In short: the program translates between **protocols** on any adapter that
is physically connected to the right **line**.

## ELM327 bridge

Many programs and apps understand only an ELM327. The bridge turns the
current connection into a network ELM327 (like a Wi-Fi adapter): **Settings →
ELM327 bridge → Start** or `udt bridge`. In the app choose a Wi-Fi ELM327 and
enter the computer's address and the port (35000 by default).

- If the car speaks OBD-II itself, requests pass straight through.
- If not (Микас, Январь, KW1281), the program builds OBD-II answers from the
  control unit's values: coolant temperature → PID 05, engine speed → PID 0C,
  voltage → PID 42 and so on — from the `obd` field of the description.
- The number of codes and the MIL go through PID 01; mode 04 clears codes.
- Codes without an SAE equivalent (such as the three-digit Микас codes) are
  not sent in mode 03 — only this program shows them.
