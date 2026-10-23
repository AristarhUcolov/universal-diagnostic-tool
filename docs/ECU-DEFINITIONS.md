<a id="ru"></a>

**Русский** · [English](#eng)

# Описания блоков управления и база кодов

Программа знает о конкретном блоке ровно то, что написано в его описании:
куда подключаться, какие запросы слать, где в ответе лежит каждый параметр и
как его пересчитать. Описание — JSON-файл. Чтобы добавить блок, Go не нужен.

**Разделы:** [Где лежат файлы](#где-лежат-файлы) · [Пример](#пример) ·
[Поля](#поля) · [Параметры и формулы](#параметры-и-формулы) ·
[Микас и Январь-4](#микас-и-январь-4) · [UDS](#uds) ·
[Исполнительные механизмы](#исполнительные-механизмы) ·
[Статус и источники](#статус-и-источники) ·
[Как изучить новый блок](#как-изучить-новый-блок) ·
[База кодов](#база-кодов-неисправностей)

---

## Где лежат файлы

- Встроенные описания: [`internal/ecu/defs/`](../internal/ecu/defs/).
- Свои описания: папка `definitions` в папке данных — `udt-data\definitions`
  рядом с программой (или `%APPDATA%\udt\definitions` в Windows и
  `~/.config/udt/definitions` в Linux, если в папку программы нельзя
  записывать). Быстро открыть: **Настройки → Папки → Описания блоков**.
  Описание с тем же `id`, что у встроенного, заменяет встроенное.
- Описания читаются при запуске программы. Ошибки в файле показывает
  `udt definitions`; такое описание не загружается.

## Пример

Блок двигателя на KWP2000 по K-Line (вымышленный, для иллюстрации):

```json
{
  "id": "example-kwp",
  "name": "Пример блока KWP2000",
  "vendor": "Пример",
  "protocol": "kwp2000",
  "profile": "ru-kwp",
  "family": "ru-kwp",
  "status": "unverified",
  "vehicles": ["Пример: такой-то автомобиль с таким-то блоком"],
  "sources": ["Документ, по которому составлено описание, с версией и датой"],
  "notes": {"en": "What to know about this unit.", "ru": "Что нужно знать об этом блоке."},
  "link": {"init": "fast", "baud": 10400, "target": "0x10", "tester": "0xF1", "p3min_ms": 100},
  "match": {"ident": ["1411020"], "block_len": {"21 01": 36}},
  "keepalive": "3E 01",
  "ident": [
    {"request": "1A 80", "skip": 2, "fields": [
      {"key": "vin", "name": {"en": "VIN", "ru": "VIN"}, "len": 19}
    ]}
  ],
  "dtc": {"read": "18 00 00 00", "format": "kwp", "clear": ["14 00 00"]},
  "blocks": [
    {"id": "01", "name": {"en": "Main data", "ru": "Основные данные"},
     "request": "21 01", "skip": 2, "min_len": 34,
     "params": [
       {"id": "twat", "name": {"en": "Coolant temperature", "ru": "Температура охлаждающей жидкости"},
        "offset": 8, "expr": "x-40", "unit": "°C", "min": -40, "max": 130,
        "group": "temperature", "obd": "05"},
       {"id": "rpm", "name": {"en": "Engine speed", "ru": "Частота вращения коленвала"},
        "offset": 11, "expr": "x*40", "unit": "rpm", "min": 0, "max": 7000,
        "group": "engine", "obd": "0C"}
     ]}
  ]
}
```

Описание может унаследовать другое: `"extends": "vaz-m154n-euro2"` берёт
всё из базового, а поля из файла заменяют унаследованные **целиком**: свой
список `blocks` или `actuators` полностью заменяет базовый, параметры и
тесты не смешиваются по порядку. Правила опознания `match` не наследуются.
Так сделаны `vaz-j72` и `vaz-m154-r83`.

## Поля

| Поле | Что это |
|---|---|
| `id` | уникальный идентификатор: латиница, цифры, дефис |
| `extends` | `id` описания, на котором основано это |
| `name`, `vendor` | название блока и производитель |
| `protocol` | `kwp2000`, `legacy` (Микас, Январь-4) или `uds` |
| `profile` | профиль подключения, к которому относится блок (`ru-kwp`, `ru-legacy`, `uds`…) |
| `family` | набор описаний кодов: `ru-kwp`, `ru-r83`, `gaz-mikas`, `vag`; без него — общий SAE |
| `status` | `verified`, `partial` или `unverified` — см. [ниже](#статус-и-источники) |
| `vehicles` | машины и двигатели, на которых стоит блок |
| `sources` | откуда взяты данные — **обязательно** |
| `notes` | пояснение на двух языках: `{"en": …, "ru": …}` |
| `link` | связь: `init` (`fast`, `slow`, `none`), `baud`, `target` и `tester` (адреса K-Line), `p3min_ms`; для CAN — `tx_id`, `rx_id`, `bitrate` |
| `match` | как узнать блок: `ident` (подстроки идентификации), `version` (байт версии Микас, hex), `block_len` (запрос → длина ответа), `fallback` |
| `session`, `keepalive` | запрос после подключения и запрос, удерживающий сеанс |
| `ident` | чтение идентификации: `request`, `skip`, `fields` (`key`, `name`, `len`, `type`: `ascii`, `hex`, `cp866`, `u8`) |
| `dtc` | ошибки: `read` (запрос), `format` (`kwp`, `uds`, `legacy`), `clear` (один или несколько запросов) |
| `blocks` | записи данных: `id`, `name`, `request`, `skip`, `min_len`, `params` |
| `actuators` | исполнительные механизмы — см. [ниже](#исполнительные-механизмы) |

Все запросы пишутся в hex через пробел: `"21 01"`.

## Параметры и формулы

| Поле | Что это |
|---|---|
| `id` | идентификатор параметра, уникальный в описании |
| `name` | `{"en": …, "ru": …}` |
| `offset` | позиция в ответе после `skip` байт |
| `type` | `u8` (по умолчанию), `s8`, `u16le`, `u16be`, `s16le`, `s16be`, `u32le`, `u32be`, `bit`, `flags`, `ascii` |
| `bit` | номер бита для `bit` |
| `expr` | формула пересчёта; `x` — сырое значение |
| `unit`, `min`, `max`, `digits` | единица, диапазон шкалы, знаков после запятой |
| `group` | группа в списке: `temperature`, `engine`, `air`, `fuel`, `ignition`, `electrical`, `idle`, `o2`, `modes`, `sensors`, `status`… |
| `obd` | номер PID OBD-II, которому соответствует параметр (для плиток обзора и моста ELM327) |
| `enum` | подписи значений: `{"0": {"en": "off", "ru": "выкл"}, "1": …}` |
| `flags` | подписи битов 0–7 для `flags` |
| `verified` | `false`, если формула не сверена с реальным блоком (параметр получит «?») |
| `note` | пояснение к параметру на двух языках |

**Формулы:** числа (десятичные или `0x…`), `+ - * / %`, `& | ^ << >>`,
скобки, унарный минус и функции `abs`, `min`, `max`, `round`, `floor`,
`ceil`, `sqrt`. Примеры: `x-40`, `x*40`, `5.2+x*0.05`, `(x-128)/256`,
`(x>>4)&0x0F`.

## Микас и Январь-4

В раннем протоколе нет «записей данных» — параметры запрашиваются по кодам
(команда `61`). У блока `blocks` без `request`, а у каждого параметра вместо
`offset` — `code`:

```json
{"id": "twat", "name": {"en": "Coolant temperature", "ru": "Температура охлаждающей жидкости"},
 "code": "1A", "expr": "x-40", "unit": "°C", "min": -40, "max": 130, "group": "temperature", "obd": "05"}
```

Блок узнаётся по байту версии: `"match": {"version": ["09", "0A"]}`. Пример —
[`gaz-mikas71.json`](../internal/ecu/defs/gaz-mikas71.json).

Длина ответа на `61` — сумма длин запрошенных параметров, поэтому один
неверный `type` сдвинул бы все значения после него. Если длина не сходится,
программа опрашивает параметры по одному, запоминает настоящую длину,
пишет предупреждение в журнал, а вместо значения такого параметра
показывает сырые байты — ни одно соседнее значение не будет неверным.
Так нашлось, что настоящий Микас-5.47 передаёт «установку расхода воздуха»
(`59`) одним байтом, хотя в описании протокола — два.

## UDS

Для `"protocol": "uds"` записи данных — это идентификаторы DID:
`"request": "22 F4 0C"`, `"skip": 3`. Ошибки по умолчанию читаются запросом
`19 02 FF`. Описание UDS-блока пока выбирается вручную:
**Подключение → Дополнительные настройки → Описание блока** или
`udt scan -profile uds -ecu <id>`.

## Исполнительные механизмы

```json
{"id": "fan", "name": {"en": "Cooling fan", "ru": "Вентилятор охлаждения"},
 "kind": "toggle", "on": "30 0A 06 01", "off": "30 0A 00",
 "warning": {"en": "Keep hands away from the fan.", "ru": "Уберите руки от вентилятора."}}
```

| `kind` | Что делает |
|---|---|
| `toggle` | кнопки «вкл» (`on`) и «выкл» (`off`) |
| `pulse` | `on`, пауза `max` миллисекунд (не больше 1000, по умолчанию 300), затем `off` |
| `value` | поле ввода; `set` — шаблон запроса, `min` и `max` — допустимый диапазон, `unit` — единица |

В шаблоне `set` значение подставляется в фигурных скобках: `{v}` — один
байт, `{v/10}` — с формулой, `{v:u16be}` — два байта. При отключении
программа сама выключает всё, что включила.

| Поле | Что это |
|---|---|
| `condition` | `engine_off` — тест выполняется только на заглушенном двигателе, `engine_running` — только на работающем. Программа проверяет обороты (параметр с `"obd": "0C"`) и без них тест не запустит. У `pulse` (форсунки, катушки) `engine_off` обязателен — это проверяет CI |
| `timeout_s` | через сколько секунд включённый выход вернётся блоку сам (по умолчанию 60, не больше 600); бензонасосу хватит 30 |

## Статус и источники

- **`verified`** — данные взяты из официальной документации производителя или
  сверены с реальным блоком на нескольких значениях (например, температура
  с термометром, обороты с тахометром).
- **`partial`** — основа проверена, отдельные параметры нет: у них
  `"verified": false`.
- **`unverified`** — описание составлено по косвенным данным. Программа
  показывает это пользователю.

Поле `sources` обязательно: документ, его версия и дата, или «сверено с
блоком такой-то, прошивка такая-то, на машине такой-то». Неверная формула
может заставить человека поменять исправную деталь, поэтому описание без
источника не принимается (это проверяет CI).

## Как изучить новый блок

1. Подключитесь с профилем «KWP2000 — свой адрес» или «UDS» и нужным адресом
   или идентификаторами.
2. **Терминал:** отправьте `21 01` (KWP2000) или `22 F1 90` (UDS) и
   посмотрите ответ.
3. **Параметры → Сырые записи:** запись запрашивается по кругу, а байты,
   которые меняются, подсвечиваются. Прогрейте двигатель, покрутите обороты,
   нажмите педаль — и смотрите, какой байт за что отвечает.
4. Найденное запишите в описание со статусом `unverified` и сверьте с
   приборами. Журнал сеанса приложите к pull request — по нему можно
   перепроверить.

## База кодов неисправностей

Файлы [`internal/dtc/data/*.dtc`](../internal/dtc/data/), по строке на код:

```
# комментарий
P0130 | O2 sensor circuit (bank 1, sensor 1) | Цепь датчика кислорода (банк 1, датчик 1)
```

| Файл | Коды |
|---|---|
| `sae.dtc` | общие коды SAE J2012 (`P0130`) |
| `ru-kwp.dtc` | ВАЗ/ГАЗ/УАЗ на KWP2000 — только коды, отличающиеся от SAE |
| `ru-r83.dtc` | Bosch M1.5.4 / Январь-5 «Россия-83» |
| `gaz-mikas.dtc` | трёхзначные коды Микас-5.4/7.x (`053`) |
| `vag.dtc` | пятизначные коды VAG (`00532`) |

Обязательно: три поля через `|`, английский и русский текст, формат кода
своего набора, без повторов. Всё это проверяет CI, так что исправление можно
прислать прямо через веб-интерфейс GitHub.

---
---

<a id="eng"></a>

[Русский](#ru) · **English**

# Control unit descriptions and the code database

The program knows exactly what a control unit's description says about it:
how to connect, which requests to send, where each value sits in the answer
and how to convert it. A description is a JSON file; adding a control unit
needs no Go.

## Where the files are

- Built-in descriptions: [`internal/ecu/defs/`](../internal/ecu/defs/).
- Your own: the `definitions` folder in the data folder —
  `udt-data/definitions` next to the program (or `%APPDATA%\udt\definitions`
  on Windows and `~/.config/udt/definitions` on Linux when the program's
  folder cannot be written). **Settings → Folders → ECU definitions** opens
  it. A description with the same `id` as a built-in one replaces it.
- Descriptions are read at start-up. `udt definitions` shows errors; a file
  with errors is not loaded.

## Example and fields

See the [Russian section](#пример) for a complete example — JSON keys are the
same in both languages; `name`, `notes` and `note` carry `{"en": …, "ru": …}`
text. The fields:

| Field | Meaning |
|---|---|
| `id` | unique identifier: Latin letters, digits, hyphens |
| `extends` | `id` of the description this one builds on; each field in the file replaces the inherited one **as a whole** (a `blocks` or `actuators` list of its own replaces the base's list, entries are never mixed by position); `match` is not inherited |
| `name`, `vendor` | control unit name and maker |
| `protocol` | `kwp2000`, `legacy` (Микас, Январь-4) or `uds` |
| `profile` | connection profile the unit belongs to (`ru-kwp`, `ru-legacy`, `uds`…) |
| `family` | code description set: `ru-kwp`, `ru-r83`, `gaz-mikas`, `vag`; generic SAE without it |
| `status` | `verified`, `partial` or `unverified` |
| `vehicles` | vehicles and engines using the unit |
| `sources` | where the data comes from — **required** |
| `link` | `init` (`fast`, `slow`, `none`), `baud`, `target` and `tester` (K-Line addresses), `p3min_ms`; for CAN `tx_id`, `rx_id`, `bitrate` |
| `match` | recognising the unit: `ident` (identification substrings), `version` (Микас version byte, hex), `block_len` (request → answer length), `fallback` |
| `session`, `keepalive` | request sent after connecting; request that keeps the session open |
| `ident` | identification: `request`, `skip`, `fields` (`key`, `name`, `len`, `type`: `ascii`, `hex`, `cp866`, `u8`) |
| `dtc` | `read` (request), `format` (`kwp`, `uds`, `legacy`), `clear` (one or more requests) |
| `blocks` | data records: `id`, `name`, `request`, `skip`, `min_len`, `params` |
| `actuators` | output tests |

Parameters: `id`, `name`, `offset` (after `skip` bytes), `type` (`u8`
default, `s8`, `u16le`, `u16be`, `s16le`, `s16be`, `u32le`, `u32be`, `bit`,
`flags`, `ascii`), `bit`, `expr`, `unit`, `min`, `max`, `digits`, `group`,
`obd` (the matching OBD-II PID, used by overview tiles and the ELM327
bridge), `enum`, `flags`, `verified` (`false` puts a "?" on the value),
`note`.

**Formulas:** numbers (decimal or `0x…`), `+ - * / %`, `& | ^ << >>`,
parentheses, unary minus and `abs`, `min`, `max`, `round`, `floor`, `ceil`,
`sqrt`; `x` is the raw value. Examples: `x-40`, `x*40`, `5.2+x*0.05`,
`(x-128)/256`, `(x>>4)&0x0F`.

**Микас and Январь-4:** no data records — values are requested by code
(command `61`). Use `blocks` without `request` and give each parameter a
`code` instead of an `offset`; recognise the unit with
`"match": {"version": ["09", "0A"]}`. See
[`gaz-mikas71.json`](../internal/ecu/defs/gaz-mikas71.json). The answer to
`61` is the sizes of the requested values added up, so one wrong `type`
would shift every value after it. When the length does not add up, the
program asks for the values one by one, remembers the real size, writes a
warning to the journal and shows that value as raw bytes. This is how a
real Микас-5.47 turned out to send the air flow set point (`59`) as one
byte, where the protocol description says two.

**UDS:** data records are DIDs (`"request": "22 F4 0C"`, `"skip": 3`); codes
are read with `19 02 FF` by default. A UDS description is chosen by hand for
now: **Connect → Advanced → Control unit description** or
`udt scan -profile uds -ecu <id>`.

**Output tests:** `toggle` (`on` / `off` requests), `pulse` (`on`, a pause of
`max` milliseconds — at most 1000, default 300 — then `off`), `value` (`set`
template with `min`, `max`, `unit`). In templates `{v}` is one byte, `{v/10}`
applies a formula, `{v:u16be}` writes two bytes. On disconnect the program
switches off everything it switched on. `condition`: `engine_off` or
`engine_running` — checked against the engine speed (the value with
`"obd": "0C"`) before the test starts; when the speed cannot be read, the
test is not started. A `pulse` (injector, coil) must have `engine_off`,
which CI checks. `timeout_s`: an output left on is handed back
to the control unit after this many seconds (default 60, at most 600).

## Status and sources

- **`verified`** — from the maker's official documentation, or checked
  against a real unit on several values (temperature against a thermometer,
  engine speed against a tachometer).
- **`partial`** — the base is checked, some values are not: those carry
  `"verified": false`.
- **`unverified`** — built from indirect evidence; the program tells the user.

`sources` is required: the document with version and date, or "checked
against unit X, software Y, in car Z". A wrong formula can make someone
replace a good part, so descriptions without a source are not accepted (CI
checks this).

## Exploring a new control unit

1. Connect with the "KWP2000 — custom address" or "UDS" profile and the
   address or identifiers.
2. **Terminal:** send `21 01` (KWP2000) or `22 F1 90` (UDS) and look at the
   answer.
3. **Live data → Raw records:** the record is polled in a loop and changing
   bytes are highlighted. Warm the engine up, rev it, press the pedal — and
   watch which byte follows what.
4. Put what you found into a description with status `unverified` and check
   it against instruments. Attach the session journal to the pull request so
   others can re-check.

## Trouble code database

Files [`internal/dtc/data/*.dtc`](../internal/dtc/data/), one line per code:
`CODE | English | Russian`; lines starting with `#` are comments. `sae.dtc`
— generic SAE J2012 codes; `ru-kwp.dtc` — VAZ/GAZ/UAZ KWP2000 codes that
differ from SAE; `ru-r83.dtc` — Bosch M1.5.4 / Январь-5 "Russia-83";
`gaz-mikas.dtc` — three-digit Микас codes; `vag.dtc` — five-digit VAG codes.
Three fields, both languages, the right code format for the file and no
duplicates are required — CI checks all of it, so a fix can be sent straight
from GitHub's web interface.
