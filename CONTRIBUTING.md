<a id="ru"></a>

**Русский** · [English](#eng)

# Как помочь проекту

Спасибо, что заглянули. Цель проекта простая: чтобы человек с ноутбуком и
недорогим кабелем мог сам узнать, что с его машиной, и не зависеть от дилера
и платных программ.

## Самое ценное: проверка на настоящих машинах

Программа написана и проверена на встроенном симуляторе. Чего ей не хватает —
**проверки на реальных адаптерах и автомобилях**. Если у вас есть адаптер и
машина:

1. Подключитесь, прочитайте ошибки, запустите параметры.
2. Сохраните журнал: **Журнал → Экспорт CSV** или файл `.jsonl` из папки
   журналов (**Настройки → Папки → Журналы**).
3. Откройте [issue](https://github.com/AristarhUcolov/universal-diagnostic-tool/issues)
   и напишите:
   - адаптер: модель, микросхема (видна в списке портов), драйвер;
   - машина: модель, год, двигатель, надпись на этикетке блока;
   - что получилось и что нет;
   - приложите журнал.

В журнале может оказаться VIN машины — если не хотите его публиковать,
замените его на `XXXX` перед отправкой.

Проверенные адаптеры и блоки отметим в документации — это поможет следующему
владельцу такой же машины.

## Описания блоков и коды неисправностей

Правила и формат: **[docs/ECU-DEFINITIONS.md](docs/ECU-DEFINITIONS.md)**.

Коротко:

- у каждого описания обязательно поле `sources` — документ или «сверено с
  блоком такой-то на машине такой-то»;
- не уверены в формуле — ставьте `"verified": false` или статус
  `unverified`: программа покажет предупреждение, и это честно;
- отсутствующий параметр лучше выдуманного;
- весь текст — на английском и русском; тест проверит, что второй язык не
  забыт;
- советы по ремонту лежат в
  [`internal/repair/hints.json`](internal/repair/hints.json): что делает
  узел, признаки, причины от простых к дорогим, как проверить и срочность
  (`stop`, `soon`, `info`). Пишите только то, что можно проверить
  мультиметром, манометром или в разделе «Параметры», без названий
  платных программ;
- проверили тест механизма или регулировку на своей машине — напишите,
  какой блок и что произошло, и пришлите журнал.

CI проверит формат за вас. Go устанавливать не обязательно — можно прислать
pull request прямо через веб-интерфейс GitHub.

## Ошибки и неточности

Если программа показала не то, что показывает исправный прибор (термометр,
тахометр, другой сканер), — **это важный баг, откройте issue**. Приложите
журнал, значение в программе, показание прибора, блок и машину.

## Код

```bash
go test ./...          # должно быть зелено
go test -race ./...    # гонки потоков
gofmt -w .             # перед коммитом
go vet ./...
```

Что стоит знать об устройстве:

- Слои: адаптер → канал (`diag.Channel`) → протокол → обработчик блока
  (`ecu.Handler`) → интерфейс. Новый адаптер — это реализация `kline.Line`,
  `can.Bus` или `diag.Channel`; всё, что выше, заработает с ним само.
- У каждого протокола есть двойник в симуляторе (`internal/sim`), и тесты
  гоняют настоящий код против него. Новый протокол — вместе с узлом
  симулятора и тестом.
- Без cgo: драйверы адаптеров (J2534, RP1210, PCAN, Kvaser) подгружаются при
  запуске, чтобы программа оставалась одним файлом.
- Интерфейс — обычные JS-модули без сборки в `internal/server/web`; весь
  текст в `i18n.js` на двух языках.
- Интерфейс доступен только с этого компьютера и защищён ключом сеанса —
  пусть так и остаётся.

## Поддержать деньгами

Программа бесплатная и останется такой. Если она вам помогла и хочется
отблагодарить деньгами — это оплачивает время на разработку, адаптеры для
проверки и испытания на настоящих машинах:

| | |
|---|---|
| **Ko-fi** — разово или ежемесячно, картой из любой страны | [ko-fi.com/AristarhUcolov](https://ko-fi.com/AristarhUcolov) |
| **Buy Me a Coffee** — быстрый платёж картой | [buymeacoffee.com/aristarh.ucolov](https://buymeacoffee.com/aristarh.ucolov) |
| **DonationAlerts** — удобно из России и стран СНГ | [donationalerts.com/c/aristarh_ucolov](https://www.donationalerts.com/c/aristarh_ucolov) |

Те же ссылки — в программе (раздел **«♥ Поддержать»**) и за кнопкой
**Sponsor** на странице репозитория.

## Чего проект не примет

Программирование ключей и иммобилайзеров, обход противоугонных систем,
изменение пробега. И данные без источника.

---
---

<a id="eng"></a>

[Русский](#ru) · **English**

# How to help

Thanks for stopping by. The goal is simple: a person with a laptop and a
cheap cable should be able to find out what is wrong with their car without
depending on a dealer or paid software.

## Most valuable: testing on real vehicles

The program was written and tested against the built-in simulator. What it
lacks is **testing with real adapters and vehicles**. If you have an adapter
and a car:

1. Connect, read the codes, start live data.
2. Save the journal: **Journal → Export CSV** or the `.jsonl` file from the
   journal folder (**Settings → Folders → Journals**).
3. Open an [issue](https://github.com/AristarhUcolov/universal-diagnostic-tool/issues)
   with:
   - adapter: model, chip (shown in the port list), driver;
   - car: model, year, engine, the label on the control unit;
   - what worked and what did not;
   - the journal attached.

The journal may contain the car's VIN — replace it with `XXXX` before
sending if you do not want to publish it.

Tested adapters and control units will be marked in the documentation — that
helps the next owner of the same car.

## Control unit descriptions and trouble codes

Rules and format: **[docs/ECU-DEFINITIONS.md](docs/ECU-DEFINITIONS.md)**.

In short:

- every description needs `sources` — a document, or "checked against unit X
  in car Y";
- not sure about a formula — use `"verified": false` or status `unverified`:
  the program shows a warning, and that is honest;
- a missing value is better than an invented one;
- all text is in English and Russian; a test makes sure neither is
  forgotten;
- repair advice lives in
  [`internal/repair/hints.json`](internal/repair/hints.json): what the part
  does, symptoms, causes from cheap to expensive, how to check, and urgency
  (`stop`, `soon`, `info`). Write only what can be checked with a
  multimeter, a pressure gauge or on the Live data page, and name no paid
  software;
- tried an output test or an idle adjustment on your car — tell us which
  unit and what happened, and send the journal.

CI checks the format for you. You do not need Go — a pull request can be sent
straight from GitHub's web interface.

## Bugs and inaccuracies

If the program shows something different from a known-good instrument
(thermometer, tachometer, another scanner), **that is an important bug —
open an issue**. Attach the journal, the program's value, the instrument's
reading, the control unit and the car.

## Code

```bash
go test ./...          # must be green
go test -race ./...    # data races
gofmt -w .             # before committing
go vet ./...
```

Worth knowing about the structure:

- Layers: adapter → channel (`diag.Channel`) → protocol → control unit
  handler (`ecu.Handler`) → interface. A new adapter implements `kline.Line`,
  `can.Bus` or `diag.Channel`; everything above works with it by itself.
- Every protocol has a twin in the simulator (`internal/sim`), and the tests
  run the real code against it. A new protocol comes with a simulator node and
  a test.
- No cgo: adapter drivers (J2534, RP1210, PCAN, Kvaser) are loaded at run
  time so the program stays a single file.
- The interface is plain JS modules without a build step in
  `internal/server/web`; all text lives in `i18n.js` in both languages.
- The interface is reachable only from this computer and protected by a
  session key — keep it that way.

## Support with money

The program is free and will stay free. If it helped you and you want to say
thanks with money, it pays for development time, adapters for testing and
trials on real cars:

| | |
|---|---|
| **Ko-fi** — one-off or monthly, by card from any country | [ko-fi.com/AristarhUcolov](https://ko-fi.com/AristarhUcolov) |
| **Buy Me a Coffee** — a quick card payment | [buymeacoffee.com/aristarh.ucolov](https://buymeacoffee.com/aristarh.ucolov) |
| **DonationAlerts** — convenient from Russia and the CIS | [donationalerts.com/c/aristarh_ucolov](https://www.donationalerts.com/c/aristarh_ucolov) |

The same links are in the program (the **"♥ Support"** section) and behind
the **Sponsor** button on the repository page.

## What the project will not accept

Key and immobiliser programming, bypassing anti-theft systems, changing the
odometer. And data without a source.
