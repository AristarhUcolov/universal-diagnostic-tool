<a id="ru"></a>

**Русский** · [English](#eng)

# Диагностические разъёмы

> **Сначала мультиметр.** Перед подключением адаптера к незнакомому разъёму
> или самодельному переходнику найдите массу (звонится на кузов) и +12 В
> (напряжение относительно массы). Масса адаптера соединена с массой
> компьютера через USB: перепутанные провода могут сжечь и адаптер, и порт
> компьютера. На грузовиках и автобусах бортовая сеть 24 В — убедитесь, что
> адаптер на это рассчитан: большинство кабелей KKL и клонов ELM327 —
> только 12 В.

## OBD-II / EOBD (SAE J1962), 16 контактов

Вид на гнездо машины:

```
  ┌───────────────────────────────┐
  │  1   2   3   4   5   6   7   8 │
   \ 9  10  11  12  13  14  15  16/
    └───────────────────────────┘
```

| Контакт | Назначение |
|---|---|
| 2 | SAE J1850 «+» (PWM и VPW) |
| 4 | масса кузова |
| 5 | сигнальная масса |
| 6 | CAN High (ISO 15765-4) |
| 7 | K-Line (ISO 9141-2, ISO 14230-4) |
| 10 | SAE J1850 «−» (только PWM) |
| 14 | CAN Low |
| 15 | L-Line (ISO 9141-2, ISO 14230-4) |
| 16 | +12 В от аккумулятора (на 24-вольтовых машинах — +24 В, разъём тип B) |
| 1, 3, 8, 9, 11, 12, 13 | на усмотрение производителя |

Частые варианты «свободных» контактов:

- **GM:** 1 — однопроводной CAN (GMLAN, 33,3 кбит/с).
- **Ford, Mazda:** 3 и 11 — MS-CAN (125 кбит/с).
- **Ethernet (DoIP по ISO 13400-4, BMW ENET):** 3, 11, 12, 13 — пары
  Ethernet, 8 — линия активации.

Самодельный ENET-кабель для BMW F/G/I: OBD 12 → RJ45 1, OBD 13 → RJ45 2,
OBD 3 → RJ45 3, OBD 11 → RJ45 6, резистор 510 Ом между контактами OBD 8 и
16. Так делают большинство таких кабелей, но сверьтесь со схемой своей
машины.

## Колодка ВАЗ / ГАЗ / УАЗ, 12 контактов (тип GM)

Колодка того же вида, что GM ALDL: контакты обозначены буквами, нижний ряд
`F E D C B A`, верхний `G H J K L M` (буквы I нет). Разводка менялась от
модели к модели. Чаще всего **A — масса, M — K-Line**, а питания от
аккумулятора в такой колодке нет. Подключение кабеля KKL через переходник и
проверка мультиметром: [GAZ-VOLGA.md](GAZ-VOLGA.md).

## GM ALDL, 12 контактов (до 1996 года)

Те же буквы, что выше; питания от аккумулятора нет. Протоколы ALDL (160 и
8192 бод) программа пока не поддерживает.

## Грузовики: Deutsch 9 контактов (SAE J1939-13)

Буквы контактов выдавлены на корпусе разъёма.

| Контакт | Назначение |
|---|---|
| A | масса |
| B | + аккумулятора |
| C | J1939 CAN High |
| D | J1939 CAN Low |
| E | экран CAN |
| F | J1708 «+» |
| G | J1708 «−» |
| H, J | на усмотрение производителя (часто второй CAN) |

Чёрный разъём (тип 1) — J1939 на 250 кбит/с. Зелёный (тип 2) рассчитан и на
500 кбит/с. Программа пробует обе скорости.

## Грузовики: Deutsch 6 контактов (SAE J1708 / J1587)

| Контакт | Назначение |
|---|---|
| A | J1708 «+» |
| B | J1708 «−» |
| C | + аккумулятора |
| E | масса |
| D, F | не используются |

J1708/J1587 программа пока не поддерживает.

## Другие разъёмы

К большинству старых разъёмов продаются переходники на OBD-II. Что будет
дальше, решает протокол блока, а не форма разъёма:

| Разъём | Что внутри | В программе |
|---|---|---|
| BMW 20 контактов (круглый, до ~2000) | K-Line | переходник 20 → 16 + KKL или K+DCAN в режиме K-Line |
| Mercedes-Benz 38 контактов (круглый) | K-Line отдельно для каждой системы | переходник 38 → 16 + KKL; переходник выводит на контакт 7 линию одной из систем |
| VAG 2×2 (до ~1997) | K-Line, L-Line | переходник + KKL, профиль VW KW1281 |
| Toyota 17 и 22 контакта | ранний протокол Toyota | ✕; коды можно вывести миганием лампы по руководству |
| Nissan 12 и 14 контактов | Nissan Consult | ✕ |
| Fiat / Alfa Romeo / Lancia 3 контакта | K-Line, L-Line | переходник + KKL; протокол зависит от блока |
| PSA 30, Renault 12 и 30 контактов | фирменные протоколы | ✕ |
| Honda 3 и 6 контактов, Suzuki 6, Kawasaki 4, Yamaha 4 | у многих K-Line (KWP2000) | переходник + KKL, профиль «KWP2000 — свой адрес»; не проверено |
| Harley-Davidson 4 и 6 контактов | J1850 VPW или CAN | ~ через ELM327 или J2534; не проверено |
| 12 контактов спецтехники | J1939 или фирменный CAN | переходник + CAN-адаптер или RP1210 |

---
---

<a id="eng"></a>

[Русский](#ru) · **English**

# Diagnostic connectors

> **Multimeter first.** Before connecting an adapter to an unfamiliar socket
> or a home-made lead, find ground (continuity to the body) and +12 V
> (voltage against ground). The adapter's ground is tied to the computer's
> ground through USB: swapped wires can burn both the adapter and the
> computer's port. Trucks and buses run on 24 V — make sure the adapter is
> rated for it: most KKL cables and ELM327 clones are 12 V only.

## OBD-II / EOBD (SAE J1962), 16 pins

Looking at the vehicle socket:

```
  ┌───────────────────────────────┐
  │  1   2   3   4   5   6   7   8 │
   \ 9  10  11  12  13  14  15  16/
    └───────────────────────────┘
```

| Pin | Use |
|---|---|
| 2 | SAE J1850 "+" (PWM and VPW) |
| 4 | chassis ground |
| 5 | signal ground |
| 6 | CAN High (ISO 15765-4) |
| 7 | K-Line (ISO 9141-2, ISO 14230-4) |
| 10 | SAE J1850 "−" (PWM only) |
| 14 | CAN Low |
| 15 | L-Line (ISO 9141-2, ISO 14230-4) |
| 16 | battery +12 V (+24 V on 24-volt vehicles, type B connector) |
| 1, 3, 8, 9, 11, 12, 13 | manufacturer's choice |

Common uses of the free pins:

- **GM:** 1 — single-wire CAN (GMLAN, 33.3 kbit/s).
- **Ford, Mazda:** 3 and 11 — MS-CAN (125 kbit/s).
- **Ethernet (DoIP per ISO 13400-4, BMW ENET):** 3, 11, 12, 13 — the Ethernet
  pairs, 8 — the activation line.

A home-made ENET cable for BMW F/G/I: OBD 12 → RJ45 1, OBD 13 → RJ45 2,
OBD 3 → RJ45 3, OBD 11 → RJ45 6, a 510 Ω resistor between OBD pins 8 and 16.
That is how most such cables are made, but check your car's wiring diagram.

## VAZ / GAZ / UAZ 12-pin socket (GM type)

The same shape as GM ALDL: contacts are lettered, bottom row
`F E D C B A`, top row `G H J K L M` (no letter I). The wiring changed from
model to model. Most often **A is ground and M is K-Line**, and there is no
battery supply in this socket. Connecting a KKL cable through an adapter and
checking with a multimeter: [GAZ-VOLGA.md](GAZ-VOLGA.md).

## GM ALDL, 12 pins (before 1996)

The same letters as above; no battery supply. The ALDL protocols (160 and
8192 baud) are not supported yet.

## Trucks: Deutsch 9-pin (SAE J1939-13)

The pin letters are moulded into the connector housing.

| Pin | Use |
|---|---|
| A | ground |
| B | battery + |
| C | J1939 CAN High |
| D | J1939 CAN Low |
| E | CAN shield |
| F | J1708 "+" |
| G | J1708 "−" |
| H, J | manufacturer's choice (often a second CAN) |

The black connector (type 1) carries J1939 at 250 kbit/s; the green one
(type 2) also supports 500 kbit/s. The program tries both speeds.

## Trucks: Deutsch 6-pin (SAE J1708 / J1587)

| Pin | Use |
|---|---|
| A | J1708 "+" |
| B | J1708 "−" |
| C | battery + |
| E | ground |
| D, F | unused |

J1708/J1587 is not supported yet.

## Other connectors

Adapters to OBD-II exist for most older connectors. What happens next depends
on the control unit's protocol, not on the connector's shape:

| Connector | Inside | In the program |
|---|---|---|
| BMW 20-pin (round, until ~2000) | K-Line | 20 → 16 adapter + KKL, or K+DCAN in K-Line mode |
| Mercedes-Benz 38-pin (round) | a separate K-Line per system | 38 → 16 adapter + KKL; the adapter routes one system's line to pin 7 |
| VAG 2×2 (until ~1997) | K-Line, L-Line | adapter + KKL, VW KW1281 profile |
| Toyota 17- and 22-pin | early Toyota protocol | ✕; codes can be flashed out by the lamp per the manual |
| Nissan 12- and 14-pin | Nissan Consult | ✕ |
| Fiat / Alfa Romeo / Lancia 3-pin | K-Line, L-Line | adapter + KKL; the protocol depends on the unit |
| PSA 30-pin, Renault 12- and 30-pin | brand protocols | ✕ |
| Honda 3- and 6-pin, Suzuki 6, Kawasaki 4, Yamaha 4 | many use K-Line (KWP2000) | adapter + KKL, "KWP2000 — custom address" profile; untested |
| Harley-Davidson 4- and 6-pin | J1850 VPW or CAN | ~ through ELM327 or J2534; untested |
| 12-pin machinery sockets | J1939 or brand CAN | adapter + CAN adapter or RP1210 |
