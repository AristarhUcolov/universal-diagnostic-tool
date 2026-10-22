<a id="ru"></a>

**Русский** · [English](#eng)

# Руководство

**Разделы:** [Запуск](#запуск) · [Подключение](#подключение) ·
[Обзор](#обзор) · [Ошибки](#ошибки) · [Параметры](#параметры) ·
[Блок управления](#блок-управления) ·
[Исполнительные механизмы](#исполнительные-механизмы) ·
[Терминал](#терминал) · [Монитор CAN](#монитор-can) · [Журнал](#журнал) ·
[Настройки](#настройки) · [Мост ELM327](#мост-elm327) ·
[С телефона](#с-телефона-или-планшета) ·
[Командная строка](#командная-строка) ·
[Если не связывается](#если-не-связывается) ·
[Безопасность](#безопасность) · [Поддержать проект](#поддержать-проект)

---

## Запуск

- **Windows:** запустите `udt.exe`. Программа откроется в своём окне;
  консольное окно, которое мелькнёт при запуске, само спрячется. Если WebView2
  нет, интерфейс откроется в браузере. Для 32-битных драйверов J2534 и RP1210
  запускайте `udt-x86.exe`.
- **Linux:** `./udt-linux-amd64` — интерфейс откроется в браузере. Для
  доступа к COM-портам: `sudo usermod -aG dialout $USER` и перелогиньтесь.
- **macOS:** `./udt-macos-arm64`. Если система не даёт запустить скачанный
  файл: `xattr -d com.apple.quarantine udt-macos-arm64`.
- Ключи запуска: `-browser` — открыть в браузере вместо окна, `-no-window` —
  только сервер, `-listen 0.0.0.0:8080` — доступ из сети (см.
  [ниже](#с-телефона-или-планшета)).

Настройки, журналы, записи и отчёты лежат в папке `%APPDATA%\udt` (Windows)
или `~/.config/udt` (Linux); открыть их можно из **Настроек → Папки**.

## Подключение

1. Вставьте адаптер в разъём машины и включите зажигание (двигатель можно
   завести).
2. **Адаптер.** Карточки неустановленных драйверов (J2534, RP1210, PCAN,
   Kvaser) помечены «не обнаружен».
3. **Порт.** Для COM-адаптеров программа показывает найденные порты и
   подписывает микросхему: FTDI, CH340, CP210x, PL2303, Bluetooth. Для Wi-Fi
   ELM327 укажите адрес (обычно `192.168.0.10:35000`), для J2534 — драйвер,
   для DoIP — IP машины (или оставьте пустым — программа поищет сама).
4. **Автомобиль / блок управления** — профиль. Не уверены — «Автоматически»;
   для ВАЗ/ГАЗ/УАЗ — «ВАЗ / ГАЗ / УАЗ — автоматически».
5. **Подключиться.** «Ход подключения» показывает каждую попытку.

**Дополнительные настройки:** адрес блока (hex; `10` — двигатель KWP2000,
`01` — двигатель VW), инициализация K-Line (быстрая или 5 бод), скорость CAN,
идентификаторы UDS (по умолчанию `7E0` / `7E8`), принудительное описание
блока, эхо адаптера, способ быстрой инициализации, уровни DTR и RTS,
скорость порта адаптера.

## Обзор

Блок, протокол, статус описания и адаптер; число ошибок с первыми кодами;
основные значения плитками с мини-графиком и шкалой (жёлтый и красный цвет —
выход за пороги, например перегрев или низкое напряжение). Если значения не
опрашиваются, кнопка запустит опрос. Для OBD-II — готовность мониторов и
лампа MIL. Для симулятора — кнопки управления двигателем и «добавить
неисправность».

## Ошибки

- **Прочитать** — коды с расшифровкой на языке интерфейса, статусом (активна,
  подтверждена, ожидает, постоянная, лампа MIL) и блоком-источником. Для
  OBD-II ниже показывается стоп-кадр — значения в момент записи ошибки.
- **Стереть** — с подтверждением. Если неисправность осталась, код вернётся.
- **Отчёт для печати** — откроется страница с машиной, блоками, ошибками и
  параметрами; сохраните её в PDF через печать браузера.
- **Поиск кода** — по номеру (`P0130`, `053`, `00532`) или слову; в
  разделе «Справочник» он работает и без подключения.

## Параметры

Отметьте параметры слева (по группам, с фильтром) и нажмите **Старт**.
Частота: максимально быстро, 4 раза в секунду или раз в секунду.

- **Таблица** — значение, единица, минимум и максимум за сеанс.
- **Графики** — последняя минута; у каждого параметра своя ось. Наведите
  мышь — увидите значение в любой момент.
- **Сырые записи** — для блоков KWP2000: байты записи по кругу, меняющиеся
  подсвечены. Помогает найти и проверить параметры нового блока.
- **Запись CSV** — всё, что опрашивается, пишется в файл (папка «Записи»);
  открывается в Excel или LibreOffice.

Знак «?» у параметра — формула не сверена с реальным блоком.

## Блок управления

Идентификация (номера блока и программы, VIN, если блок его хранит), кнопка
копирования, описание блока: статус, машины, возможности, источники. Внизу —
все блоки сеанса.

## Исполнительные механизмы

Включение форсунок, реле, клапанов, вентилятора — если описание блока их
знает. У каждого — предупреждение. Выполняйте на неподвижной машине; при
отключении программа сама выключит всё, что включила.

## Терминал

Любой запрос в hex (`21 01`, `1A 80`, `22 F1 90`) и ответ блока с
названием сервиса и расшифровкой отказа. Кнопки частых запросов зависят от
протокола. Каждый обмен попадает в журнал.

## Монитор CAN

Только для CAN-адаптеров. Все кадры с идентификатором, данными, числом и
периодом; для J1939 — номер PGN. Отправка своих кадров включается в
**Настройках → Режим эксперта**: неосторожный кадр на шине может повлиять на
работу машины.

## Журнал

Каждый байт в обе стороны (`tx`, `rx`), события (`info`), предупреждения
(`warn`) и ошибки (`error`) со временем до миллисекунды, адресом и смыслом
запроса. Фильтр по типу, поиск, пауза, экспорт в CSV, очистка. Если включено
«Писать журнал в файл», каждый запуск пишет файл `.jsonl` в папку
«Журналы»; хранятся последние 50.

## Настройки

Язык, тема, единицы (метрические или имперские), режим эксперта, запись
журнала в файл, открытие в окне; мост ELM327; доступ из сети; папки; версия,
база кодов и лицензия.

## Мост ELM327

**Настройки → Мост ELM327 → Запустить мост** (или `udt bridge`). Программа
начинает принимать команды ELM327 по сети, как Wi-Fi адаптер. В приложении
на телефоне или компьютере выберите Wi-Fi ELM327 и укажите IP компьютера и
порт (по умолчанию 35000). Компьютер и телефон должны быть в одной сети;
Windows может спросить разрешение для брандмауэра.

Что умеет мост и как он показывает блоки без OBD-II — в
[PROTOCOLS.md](PROTOCOLS.md#мост-elm327).

## С телефона или планшета

Запустите программу с ключом `-listen 0.0.0.0:8080`. В консоли и в
**Настройках → Доступ с телефона** появится ссылка вида
`http://192.168.1.5:8080/#t=…` — откройте её на телефоне. Ключ после `#t=` —
пропуск: без него интерфейс не откроется.

## Командная строка

```
udt                      окно программы
udt ports                COM-порты и распознанные адаптеры
udt adapters             типы адаптеров, установленные драйверы J2534 / RP1210
udt profiles             профили и виртуальные автомобили
udt definitions          описания блоков
udt scan    [связь]      подключиться и показать блоки с идентификацией
udt dtc     [связь] [-clear]
udt live    [связь] [-params a,b] [-interval 500ms] [-count N]
udt raw     [связь] "21 01" ...
udt bridge  [связь] [-listen 0.0.0.0:35000]
udt lookup  P0130 ...    расшифровать коды без подключения
```

Ключи связи: `-adapter` (`sim`, `kkl`, `elm327`, `elm327-wifi`, `j2534`,
`slcan`, `socketcan`, `pcan`, `kvaser`, `rp1210`, `doip`, `hsfz`), `-port`,
`-profile`, `-sim`, `-address`, `-init`, `-bitrate`, `-txid`, `-rxid`,
`-ecu`, `-baud`, `-lang`, `-v` (печатать журнал).

```bash
udt scan -adapter kkl -port COM3 -profile ru-auto     # ВАЗ/ГАЗ/УАЗ через KKL
udt dtc  -adapter elm327 -port COM5                   # любая машина с OBD-II
udt live -adapter sim -sim gaz-mikas -params twat,freq,uacc
```

## Если не связывается

**Всем:** зажигание включено; на контакте 16 разъёма есть +12 В; в
**Журнале** видно, уходят ли запросы (`tx`) и есть ли ответы (`rx`).

- **K-Line (KKL):** Latency Timer = 1 мс для FTDI; явный профиль вместо
  автоматического; «Эхо адаптера», DTR и RTS в дополнительных настройках;
  переключатель режима на кабеле (K+DCAN) — в K-Line. Подробно —
  [GAZ-VOLGA.md](GAZ-VOLGA.md#7-если-не-связывается).
- **ELM327:** Bluetooth — сопрягите адаптер в системе и выберите
  **исходящий** COM-порт; скорость порта программа подбирает сама; дешёвые
  клоны «v2.1» часто не умеют часть команд.
- **J2534 / RP1210:** нет в списке — не установлен драйвер производителя;
  ошибка разрядности — запустите `udt-x86.exe`.
- **CAN:** правильная скорость (легковые — 500 кбит/с, грузовики — 250 или
  500); на стенде без машины нужен резистор 120 Ом на концах шины.
- **DoIP / ENET:** кабель должен включать линию активации; задайте сетевой
  карте адрес из диапазона 169.254.x.x (или получите его автоматически) и
  оставьте адрес машины пустым — программа найдёт её сама.

Не помогло — откройте
[issue](https://github.com/AristarhUcolov/universal-diagnostic-tool/issues):
адаптер, машина, год, блок и файл журнала.

## Безопасность

- Интерфейс по умолчанию доступен **только с этого компьютера**
  (127.0.0.1); каждый запуск создаёт новый ключ доступа, а запросы с чужим
  именем сервера отклоняются.
- С ключом `-listen 0.0.0.0:…` управлять машиной сможет любой, у кого есть
  ссылка с ключом. Включайте это только в своей сети.
- Стирание ошибок, проверка механизмов и терминал меняют состояние блока.
  Делайте это на неподвижной машине.

## Поддержать проект

Раздел **«♥ Поддержать»** в боковом меню: ссылки на Ko-fi, Buy Me a Coffee и
DonationAlerts и способы помочь без денег — проверить программу на своей
машине, прислать журнал, сверить описание блока. Программа бесплатная и
останется такой.

---
---

<a id="eng"></a>

[Русский](#ru) · **English**

# User guide

## Starting

- **Windows:** run `udt.exe`. The program opens in its own window; the
  console window that flashes up at start hides itself. Without WebView2 the
  interface opens in the browser. For 32-bit J2534 and RP1210 drivers run
  `udt-x86.exe`.
- **Linux:** `./udt-linux-amd64` — the interface opens in the browser. For
  serial ports: `sudo usermod -aG dialout $USER`, then log in again.
- **macOS:** `./udt-macos-arm64`. If the system refuses a downloaded file:
  `xattr -d com.apple.quarantine udt-macos-arm64`.
- Flags: `-browser` opens the browser instead of the window, `-no-window`
  runs only the server, `-listen 0.0.0.0:8080` allows network access (see
  [below](#from-a-phone-or-tablet)).

Settings, journals, recordings and reports live in `%APPDATA%\udt` (Windows)
or `~/.config/udt` (Linux); **Settings → Folders** opens them.

## Connecting

1. Plug the adapter into the car and switch the ignition on (the engine may
   run).
2. **Adapter.** Cards whose driver is not installed (J2534, RP1210, PCAN,
   Kvaser) say "not detected".
3. **Port.** For serial adapters the program lists the ports it found and
   names the chip: FTDI, CH340, CP210x, PL2303, Bluetooth. For a Wi-Fi ELM327
   enter the address (usually `192.168.0.10:35000`), for J2534 the driver,
   for DoIP the car's IP (or leave it empty — the program searches).
4. **Vehicle / control unit** — the profile. Not sure — "Automatic"; for
   VAZ/GAZ/UAZ — "VAZ / GAZ / UAZ — automatic".
5. **Connect.** The progress list shows every attempt.

**Advanced:** control unit address (hex; `10` — KWP2000 engine, `01` — VW
engine), K-Line initialisation (fast or 5 baud), CAN bit rate, UDS
identifiers (`7E0` / `7E8` by default), a forced control unit description,
adapter echo, fast-init method, DTR and RTS levels, adapter port speed.

## Overview

Control unit, protocol, description status and adapter; the number of codes
with the first few; key values as tiles with a sparkline and a gauge (yellow
and red mean beyond thresholds — overheating, low voltage). If values are not
being polled, a button starts polling. For OBD-II: monitor readiness and the
MIL. For the simulator: engine controls and "inject a fault".

## Trouble codes

- **Read** — codes with descriptions in the interface language, status
  (active, confirmed, pending, permanent, MIL) and the source unit. For
  OBD-II the freeze frame is shown below — values at the moment the code was
  stored.
- **Clear** — after a confirmation. If the fault remains, the code returns.
- **Printable report** — a page with the vehicle, units, codes and values;
  save it as PDF from the browser's print dialog.
- **Code search** — by number (`P0130`, `053`, `00532`) or by word; in the
  "Reference" section it works without a connection too.

## Live data

Tick parameters on the left (grouped, with a filter) and press **Start**.
Rate: as fast as possible, 4 times a second or once a second.

- **Table** — value, unit, session minimum and maximum.
- **Charts** — the last minute; one axis per parameter. Hover to read any
  moment.
- **Raw records** — for KWP2000 units: the record's bytes in a loop, with
  changing bytes highlighted. Helps to find and check values of a new unit.
- **Record CSV** — everything polled goes to a file (the "Recordings" folder)
  for Excel or LibreOffice.

A "?" next to a value means its formula has not been checked against a real
unit.

## Control unit

Identification (unit and software numbers, the VIN if the unit stores it), a
copy button, the description: status, vehicles, capabilities, sources. All
units of the session are listed at the bottom.

## Output tests

Injectors, relays, valves, the fan — where the description knows them. Each
comes with a warning. Run them with the vehicle stationary; on disconnect the
program switches off whatever it switched on.

## Terminal

Any request in hex (`21 01`, `1A 80`, `22 F1 90`) and the unit's answer with
the service name and the reason of a refusal. Quick-request buttons depend on
the protocol. Every exchange goes to the journal.

## CAN monitor

CAN adapters only. Every frame with identifier, data, count and period; the
PGN for J1939. Sending frames is enabled in **Settings → Expert mode**: a
careless frame on the bus can affect the vehicle.

## Journal

Every byte both ways (`tx`, `rx`), events (`info`), warnings (`warn`) and
errors (`error`) with millisecond time, address and the meaning of the
request. Filter by kind, search, pause, CSV export, clear. With "Write the
journal to a file" on, each run writes a `.jsonl` file into the "Journals"
folder; the latest 50 are kept.

## Settings

Language, theme, units (metric or imperial), expert mode, journal file,
window mode; the ELM327 bridge; network access; folders; version, code
database and licence.

## ELM327 bridge

**Settings → ELM327 bridge → Start** (or `udt bridge`). The program accepts
ELM327 commands over the network like a Wi-Fi adapter. In an app on a phone
or computer choose a Wi-Fi ELM327 and enter the computer's IP and the port
(35000 by default). Both devices must be on the same network; Windows may ask
for a firewall permission.

What the bridge does and how units without OBD-II appear through it:
[PROTOCOLS.md](PROTOCOLS.md#elm327-bridge).

## From a phone or tablet

Start the program with `-listen 0.0.0.0:8080`. The console and **Settings →
Access from a phone** show a link like `http://192.168.1.5:8080/#t=…` — open
it on the phone. The key after `#t=` is the pass: the interface does not open
without it.

## Command line

```
udt                      the program window
udt ports                serial ports and recognised adapters
udt adapters             adapter kinds, installed J2534 / RP1210 drivers
udt profiles             profiles and virtual vehicles
udt definitions          control unit descriptions
udt scan    [conn]       connect and list the units with identification
udt dtc     [conn] [-clear]
udt live    [conn] [-params a,b] [-interval 500ms] [-count N]
udt raw     [conn] "21 01" ...
udt bridge  [conn] [-listen 0.0.0.0:35000]
udt lookup  P0130 ...    describe codes offline
```

Connection flags: `-adapter` (`sim`, `kkl`, `elm327`, `elm327-wifi`, `j2534`,
`slcan`, `socketcan`, `pcan`, `kvaser`, `rp1210`, `doip`, `hsfz`), `-port`,
`-profile`, `-sim`, `-address`, `-init`, `-bitrate`, `-txid`, `-rxid`,
`-ecu`, `-baud`, `-lang`, `-v` (print the journal).

```bash
udt scan -adapter kkl -port COM3 -profile ru-auto     # VAZ/GAZ/UAZ with a KKL cable
udt dtc  -adapter elm327 -port COM5                   # any OBD-II car
udt live -adapter sim -sim gaz-mikas -params twat,freq,uacc
```

## When nothing answers

**Always:** ignition on; +12 V on pin 16 of the connector; the **Journal**
shows whether requests go out (`tx`) and answers come back (`rx`).

- **K-Line (KKL):** Latency Timer 1 ms for FTDI; an explicit profile instead
  of automatic; "adapter echo", DTR and RTS under Advanced; the cable's mode
  switch (K+DCAN) set to K-Line. Details:
  [GAZ-VOLGA.md](GAZ-VOLGA.md#eng).
- **ELM327:** Bluetooth — pair the adapter in the OS and choose the
  **outgoing** COM port; the program finds the port speed itself; cheap
  "v2.1" clones often lack commands.
- **J2534 / RP1210:** not listed — the maker's driver is not installed; a
  bitness error — run `udt-x86.exe`.
- **CAN:** the right bit rate (cars 500 kbit/s, trucks 250 or 500); on a
  bench without a car the bus needs 120 Ω terminators at both ends.
- **DoIP / ENET:** the cable must drive the activation line; give the network
  card an address in 169.254.x.x (or let it pick one automatically) and leave
  the car's address empty — the program finds it.

Still nothing — open an
[issue](https://github.com/AristarhUcolov/universal-diagnostic-tool/issues):
adapter, car, year, control unit and the journal file.

## Security

- By default the interface is reachable **only from this computer**
  (127.0.0.1); each run creates a new access key, and requests with a foreign
  host name are refused.
- With `-listen 0.0.0.0:…` anyone who has the link with the key can control
  the car. Use it only on your own network.
- Clearing codes, output tests and the terminal change the unit's state. Do
  them with the vehicle stationary.

## Support the project

The **"♥ Support"** section of the sidebar: links to Ko-fi, Buy Me a Coffee
and DonationAlerts, and ways to help without money — test the program on your
car, send a journal, check a control unit description. The program is free
and will stay free.
