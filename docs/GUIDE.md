<a id="ru"></a>

**Русский** · [English](#eng)

# Руководство

**Разделы:** [Запуск](#запуск) · [Интерфейс](#интерфейс) · [Подключение](#подключение) ·
[Обзор](#обзор) · [Ошибки](#ошибки) · [Мониторы](#мониторы) ·
[Параметры](#параметры) · [Приборы](#приборы) ·
[Предупреждения](#предупреждения) ·
[Блок управления](#блок-управления) · [Сервис](#сервис) ·
[Проверки](#проверки) · [Терминал](#терминал) · [Монитор CAN](#монитор-can) · [Журнал](#журнал) ·
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

Настройки, журналы, записи и отчёты лежат в папке `udt-data` рядом с
программой: перенесите программу вместе с этой папкой — на флешку или другой
компьютер, — и всё переедет с ней. Если в папку программы нельзя записывать
(например, Program Files), данные хранятся в профиле пользователя:
`%APPDATA%\udt` (Windows) или `~/.config/udt` (Linux). Ключ `-data папка`
или переменная окружения `UDT_DATA` задают любую папку. При первом запуске
настройки и журналы прежних версий из `%APPDATA%\udt` копируются в
`udt-data` сами. Где лежат данные, видно в **Настройках → Папки**.

## Интерфейс

- **Окно** при первом запуске открывается во весь экран, а потом — там и
  такого размера, как вы его оставили (файл `window.json` в `udt-data`). В
  заголовке окна — открытый раздел.
- **Меню разделов** слева; на узком экране (телефон, планшет) оно прячется
  за кнопкой ☰.
- **Верхняя строка:** подключение (нажмите — откроется страница
  подключения); **опрос** — сколько значений опрашивается и сколько раз в
  секунду, нажатие открывает «Параметры», крестик останавливает опрос;
  **REC** — идёт запись CSV; красная метка предупреждения; напряжение
  бортсети; **Команды**; язык и тема.
- **Палитра команд — Ctrl+K** (или кнопка «Команды»): наберите несколько
  букв — раздел («ошиб»), действие («запись», «тёмная тема»,
  «отключиться»), параметр подключённого блока («темп») или код
  неисправности («P0301», «035»). Стрелки — выбор, Enter — выполнить, Esc —
  закрыть.
- **Попробовать на симуляторе** — кнопка на каждой странице, которой нужно
  подключение: подключает виртуальную современную машину и не меняет ваше
  «как в прошлый раз».
- **Что нового** — внизу меню, у номера версии. После обновления окно со
  списком изменений открывается само один раз.
- **Новая версия.** Если на GitHub вышла новая версия, внизу меню появится
  «Вышла версия …»: там ссылка на страницу загрузки, «Позже» и «Пропустить
  эту версию». Программа сама ничего не скачивает.

## Подключение

1. Вставьте адаптер в разъём машины и включите зажигание (двигатель можно
   завести).
2. **Адаптер.** Карточки неустановленных драйверов (J2534, RP1210, PCAN,
   Kvaser) помечены «не обнаружен».
3. **Порт.** Для COM-адаптеров программа показывает найденные порты и
   подписывает микросхему: FTDI, CH340, CP210x, PL2303, Bluetooth. Для Wi-Fi
   ELM327 укажите адрес (обычно `192.168.0.10:35000`), для J2534 — драйвер,
   для DoIP — IP машины (или оставьте пустым — программа поищет сама). Для
   **ELM327 Bluetooth LE** нажмите **«Найти адаптеры»** и выберите адаптер
   из списка (OBD-адаптеры помечены и стоят первыми): такие адаптеры не
   появляются как COM-порт.
4. **Автомобиль / блок управления** — профиль. Не уверены — «Автоматически»;
   для ВАЗ/ГАЗ/УАЗ — «ВАЗ / ГАЗ / УАЗ — автоматически».
5. **Подключиться.** «Ход подключения» показывает каждую попытку.

В следующий раз вверху страницы будет карточка **«Как в прошлый раз»**: тот
же адаптер, порт и профиль одной кнопкой. При самом первом запуске на её месте
— приветствие с кнопкой «Попробовать на симуляторе».

**Дополнительные настройки:** адрес блока (hex; `10` — двигатель KWP2000,
`01` — двигатель VW), инициализация K-Line (быстрая или 5 бод), скорость CAN,
идентификаторы UDS (по умолчанию `7E0` / `7E8`), **CAN FD** для UDS (машины
примерно с 2020 года; адаптеры PCAN-USB FD, CANable 2.0, SocketCAN с FD) и
скорость данных CAN FD, принудительное описание блока, эхо адаптера, способ
быстрой инициализации, уровни DTR и RTS, скорость порта адаптера.

## Обзор

Блок, протокол, статус описания и адаптер; число ошибок с первыми кодами;
основные значения плитками с мини-графиком и шкалой (жёлтый и красный цвет —
выход за пороги, например перегрев или низкое напряжение). Значения и ошибки
появляются сами сразу после подключения; ошибки перечитывает кнопка
«Прочитать ошибки». Если вы остановили опрос вручную, плитки ждут кнопки
«Показать основные параметры». Для OBD-II — готовность мониторов и лампа MIL.
Для симулятора — кнопки управления двигателем и «добавить неисправность».

## Ошибки

- **Прочитать** — коды с расшифровкой на языке интерфейса, статусом (активна,
  подтверждена, ожидает, постоянная, лампа MIL) и блоком-источником. Для
  OBD-II ниже показывается стоп-кадр — значения в момент записи ошибки.
- **Как исправить ▾** — под каждым кодом: что делает узел, признаки,
  возможные причины (от простых и дешёвых к дорогим) и как проверить. Метка
  у кода — насколько срочно: «Опасно ехать», «Устранить скоро», «Можно
  отложить»; сверху — сводка по всем кодам. Это общие рекомендации:
  окончательное слово — за руководством по ремонту вашей машины.
- **Стереть** — с подтверждением. Если неисправность осталась, код вернётся.
- **Отчёт для печати** — откроется страница с машиной, блоками, ошибками,
  советами по ремонту и параметрами; сохраните её в PDF через печать
  браузера.
- **Поиск кода** — по номеру (`P0130`, `053`, `00532`) или слову; в
  разделе «Справочник» он работает и без подключения. Там же —
  **«Справочник по ремонту»**: все советы по узлам без подключения к машине.

## Мониторы

Для машин с OBD-II (EOBD).

- **Готовность мониторов** — какие самопроверки (катализатор, датчики
  кислорода, система улавливания паров…) блок уже завершил и горит ли лампа
  Check Engine. После стирания ошибок или отключения аккумулятора мониторы
  незавершены, пока машина не поездит в нужных им условиях.
- **Пропуски зажигания по цилиндрам** — счётчики за текущую или последнюю
  поездку (полоса) и среднее за 10 поездок (отметка). Цилиндр с пропусками
  видно раньше, чем блок запишет код.
- **Результаты самопроверок (режим 06)** — **Прочитать результаты**: каждое
  значение, его допуск и итог «в норме / вне допуска», с отметкой положения
  в допуске. Значение у края допуска — повод проверить деталь, пока она не
  отказала. Тесты с номерами 80–FF — тесты производителя: у них нет
  стандартного названия.

Результаты в стандартных единицах есть у машин с CAN (обычно с 2008 года).
У старых протоколов (K-Line, J1850) режим 06 выдаёт значения в единицах
производителя, поэтому программа их числами не показывает.

## Параметры

Отметьте параметры слева (по группам, с фильтром) или нажмите готовый
набор — «Основные», «Холостой ход», «Смесь и топливо». Опрос начинается сам
при открытии страницы — с тем набором, что вы выбирали для этого блока в
прошлый раз (в первый раз — основные параметры). Галочки, отмеченные или
снятые во время опроса, применяются сразу: новые значения появляются через
мгновение, без «Стоп» и «Старт». Так же сразу меняется частота: максимально
быстро, 4 раза в секунду или раз в секунду. **Стоп** останавливает опрос, и
другие страницы не запускают его сами, пока вы не нажмёте **Старт** (кроме
«Приборов»: они открываются ради живых значений).

- **Таблица** — значение, единица, минимум и максимум за сеанс.
- **Графики** — последняя минута; у каждого параметра своя ось. Наведите
  мышь — увидите значение в любой момент.
- **Сырые записи** — для блоков KWP2000: байты записи по кругу, меняющиеся
  подсвечены. Помогает найти и проверить параметры нового блока.
- **Запись CSV** — всё, что опрашивается, пишется в файл (папка «Записи»);
  открывается в Excel или LibreOffice.

Знак «?» у параметра — формула не сверена с реальным блоком.

## Приборы

Стрелочные приборы для выбранных значений, бортовой компьютер и замер
разгона. Программа сама добавляет нужные значения к опросу.

- **Выбрать значения** — до 12 приборов; выбор запоминается для каждого
  блока. Зелёная и красная зоны на шкале — те же пороги, что на «Обзоре».
- **HUD** — крупные цифры скорости и оборотов на чёрном фоне; кнопка
  «Зеркально» отражает их для проекции на лобовое стекло.
- **Во весь экран** — приборы без меню.
- **Бортовой компьютер** — мгновенный и средний расход, израсходованное
  топливо, пробег, время в пути, средняя и максимальная скорость. Расход
  берётся из часового расхода топлива, а без него оценивается по расходу
  воздуха (для бензинового двигателя при λ = 1). Считает с первого открытия
  раздела до выхода из программы; «Сбросить» начинает заново.
- **Замер разгона** — 0–60, 0–100, 60–100, 80–120 км/ч и 402 м (с
  имперскими единицами — 0–30, 0–60, 30–70 миль/ч и ¼ мили). Выберите
  замер, нажмите **Начать**, остановитесь (или сбросьте скорость) и
  трогайтесь. На время замера программа опрашивает только скорость и
  обороты как можно чаще; время между двумя замерами скорости
  интерполируется. Лучший результат отмечен звёздочкой. Замеряйте только на
  закрытой площадке.

## Предупреждения

Пока идёт опрос параметров, программа следит за опасными значениями на любой
странице: перегрев (от 115 °C), низкое напряжение бортсети (до 11,5 В) или
слишком высокое (от 15,2 В), обороты выше 6500. Появляются сообщение с
советом, красная отметка в верхней строке (нажмите — откроются «Приборы») и
короткий звук. Предупреждение снимается, когда значение вернётся в норму.
Выключаются они в **Настройках**.

## Блок управления

Идентификация (номера блока и программы, VIN, если блок его хранит), кнопка
копирования, описание блока: статус, машины, возможности, источники. Внизу —
все блоки сеанса.

## Сервис

Сервисные функции блока. Вверху — живая полоска: обороты, температура,
напряжение и значения холостого хода, чтобы сразу видеть, как отрабатывает
двигатель.

- **Перезагрузка блока управления** — как поворот ключа: временные
  регулировки сбрасываются, связь программа восстанавливает сама. Для блоков
  KWP2000 и UDS (команда ecuReset).
- **Импульсные тесты** — форсунки и катушки зажигания: короткий импульс,
  слушайте щелчок. Только на заглушенном двигателе с включённым зажиганием.
- **Реле и клапаны** — бензонасос, вентилятор, кондиционер, лампа
  неисправности, клапан адсорбера. Включённое вернётся блоку само — через
  60 секунд, бензонасос через 30; обратный отсчёт виден на карточке.
- **Регулировки холостого хода** — в реальном времени: ползунок, кнопки
  шага (±10 и ±50 об/мин) или число. С галочкой **«Применять сразу»**
  значение уходит в блок, пока вы двигаете ползунок; график под ним
  показывает заданное значение и фактическое. «Вернуть ЭБУ» отдаёт
  управление блоку.

У каждого теста указано условие — «двигатель заглушен» или «двигатель
работает». Программа проверяет его по оборотам и не запустит тест, если
условие не выполнено или обороты прочитать не удалось. Набор тестов зависит
от описания блока: сейчас они есть для ВАЗ с Bosch M1.5.4 / M1.5.4N,
Январь-5.1, VS-5.1 и Январь-7.2 (KWP2000). Для Микас-5.4 / 7.1 тесты не
предлагаются: их команды не опубликованы.

## Проверки

Пошаговые проверки с оценкой «норма» или «не норма» по живым показаниям
блока. Программа подсказывает, что сделать (заглушить или завести
двигатель, включить нагрузку), измеряет и объясняет результат.

- **Аккумулятор и генератор** — заряд без нагрузки, зарядка на работающем
  двигателе, напряжение под нагрузкой.
- **Прогрев и термостат** — как растёт температура охлаждающей жидкости.
- **Стабильность холостого хода** — насколько ровно держатся обороты
  прогретого двигателя.
- **Датчик кислорода** — переключается ли передний датчик между «богато» и
  «бедно».
- **Датчик положения дросселя** — на заглушенном двигателе нажмите педаль
  до упора и отпустите: показания должны пройти весь ход от 0 до 100 %.

Если блок не сообщает нужных значений, проверка помечена «Недоступно».

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
журнала в файл, открытие в окне, предупреждения об опасных значениях и их
звук, проверка новой версии при запуске; мост ELM327; доступ из сети;
**папки** — где лежат данные (рядом с программой или в профиле), кнопки
открыть журналы, записи, отчёты и описания блоков; **о программе** —
«Что нового», «Проверить обновления», версия, база кодов и лицензия.

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
udt reset   [связь]      перезагрузить блок управления и восстановить связь
udt bridge  [связь] [-listen 0.0.0.0:35000]
udt lookup  P0130 ...    расшифровать коды без подключения
```

`udt dtc` печатает под каждым кодом, какой раздел справочника по ремонту к
нему относится.

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
  переключатель режима на кабеле (K+DCAN) — в K-Line. Сообщение «кабель
  перестал слышать собственные байты» значит, что у кабеля пропало питание
  +12 В (выключено зажигание, отошёл разъём). Подробно —
  [GAZ-VOLGA.md](GAZ-VOLGA.md#7-если-не-связывается).
- **ELM327:** Bluetooth — сопрягите адаптер в системе и выберите
  **исходящий** COM-порт; скорость порта программа подбирает сама; дешёвые
  клоны «v2.1» часто не умеют часть команд.
- **ELM327 Bluetooth LE:** Bluetooth компьютера включён; адаптер вставлен в
  машину, зажигание включено, а к адаптеру не подключён телефон (BLE-адаптер
  работает только с одним устройством). Сопрягать в системе не нужно.
- **CAN FD:** нужен адаптер с FD — PCAN-USB FD, CANable 2.0 или интерфейс
  SocketCAN, включённый командой `ip link set can0 type can bitrate 500000
  dbitrate 2000000 fd on`. У CANable 2.0 скорость данных — 2 или 5 Мбит/с.
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
- Стирание ошибок, сервисные функции и терминал меняют состояние блока.
  Делайте это на неподвижной машине. Тесты с условием по двигателю
  программа без проверки оборотов не запускает, а включённые выходы сама
  возвращает блоку по таймеру и при отключении.

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

Settings, journals, recordings and reports live in the `udt-data` folder
next to the program: move the program together with that folder — to a flash
drive or another computer — and everything moves with it. Where the
program's folder cannot be written (Program Files, for example), the data is
kept in the user profile: `%APPDATA%\udt` (Windows) or `~/.config/udt`
(Linux). The `-data folder` flag or the `UDT_DATA` environment variable
choose any folder. On the first start the settings and journals of earlier
versions in `%APPDATA%\udt` are copied into `udt-data`. **Settings →
Folders** shows where the data is.

## The interface

- **The window** opens full screen the first time, and after that where and
  as large as you left it (the `window.json` file in `udt-data`). The title
  bar names the open page.
- **The menu** of pages is on the left; on a narrow screen (a phone, a
  tablet) it hides behind the ☰ button.
- **The top bar:** the connection (click it for the connection page);
  **polling** — how many values are polled and how many times a second, a
  click opens Live data and the × stops polling; **REC** — a CSV recording
  runs; a red alert mark; the supply voltage; **Commands**; language and
  theme.
- **The command palette — Ctrl+K** (or the "Commands" button): type a few
  letters — a page ("trouble"), an action ("record", "dark theme",
  "disconnect"), a value of the connected unit ("temp") or a trouble code
  ("P0301", "035"). Arrows choose, Enter runs, Esc closes.
- **Try the simulator** — a button on every page that needs a connection: it
  connects a virtual modern car and leaves your "like last time" as it is.
- **What's new** — at the bottom of the menu, next to the version. After an
  update the list of changes opens by itself once.
- **A new version.** When a new version is out on GitHub, "Version … is out"
  appears at the bottom of the menu, with a link to the download page,
  "Later" and "Skip this version". The program never downloads anything by
  itself.

## Connecting

1. Plug the adapter into the car and switch the ignition on (the engine may
   run).
2. **Adapter.** Cards whose driver is not installed (J2534, RP1210, PCAN,
   Kvaser) say "not detected".
3. **Port.** For serial adapters the program lists the ports it found and
   names the chip: FTDI, CH340, CP210x, PL2303, Bluetooth. For a Wi-Fi ELM327
   enter the address (usually `192.168.0.10:35000`), for J2534 the driver,
   for DoIP the car's IP (or leave it empty — the program searches). For an
   **ELM327 Bluetooth LE** press **"Find adapters"** and pick the adapter
   from the list (OBD adapters are marked and listed first): such adapters
   never show up as a serial port.
4. **Vehicle / control unit** — the profile. Not sure — "Automatic"; for
   VAZ/GAZ/UAZ — "VAZ / GAZ / UAZ — automatic".
5. **Connect.** The progress list shows every attempt.

Next time a **"Connect like last time"** card at the top of the page offers
the same adapter, port and profile with one button. On the very first start a
welcome with a "Try the simulator" button stands in its place.

**Advanced:** control unit address (hex; `10` — KWP2000 engine, `01` — VW
engine), K-Line initialisation (fast or 5 baud), CAN bit rate, UDS
identifiers (`7E0` / `7E8` by default), **CAN FD** for UDS (many cars from
about 2020; PCAN-USB FD, CANable 2.0 or a SocketCAN FD interface) and the CAN
FD data rate, a forced control unit description, adapter echo, fast-init
method, DTR and RTS levels, adapter port speed.

## Overview

Control unit, protocol, description status and adapter; the number of codes
with the first few; key values as tiles with a sparkline and a gauge (yellow
and red mean beyond thresholds — overheating, low voltage). The values and
the codes appear by themselves right after connecting; "Read codes" reads
the codes again. After you stopped polling by hand, the tiles wait for "Show
key values". For OBD-II: monitor readiness and the MIL. For the simulator:
engine controls and "inject a fault".

## Trouble codes

- **Read** — codes with descriptions in the interface language, status
  (active, confirmed, pending, permanent, MIL) and the source unit. For
  OBD-II the freeze frame is shown below — values at the moment the code was
  stored.
- **How to fix ▾** — under each code: what the part does, symptoms, possible
  causes (cheap and simple first) and how to check them. The badge says how
  urgent it is: "Driving on is risky", "Repair soon", "Can wait"; a summary
  of all codes is on top. This is general advice: the repair manual of your
  car has the final word.
- **Clear** — after a confirmation. If the fault remains, the code returns.
- **Printable report** — a page with the vehicle, units, codes, repair
  advice and values; save it as PDF from the browser's print dialog.
- **Code search** — by number (`P0130`, `053`, `00532`) or by word; in the
  "Reference" section it works without a connection too, next to the
  **repair advice** for every part.

## Monitors

For OBD-II (EOBD) vehicles.

- **Monitor readiness** — which self-tests (catalyst, oxygen sensors,
  evaporative system…) the controller has completed, and whether the
  check-engine lamp is on. After clearing codes or disconnecting the battery
  the monitors stay incomplete until the car has run in the conditions they
  need.
- **Misfires per cylinder** — the counts of the current or last drive (bar)
  and the average of the last 10 drives (mark). A misfiring cylinder shows
  up before the controller stores a code.
- **Test results (service 06)** — **Read test results**: every value, its
  limits and a "within / out of limits" verdict, with a mark showing where
  it lies between them. A value near a limit is a reason to check the part
  before it fails. Tests 80–FF belong to the manufacturer and have no
  standard name.

Results in standard units exist on CAN vehicles (usually from 2008). Older
protocols (K-Line, J1850) report service 06 in manufacturer units, so the
program does not show them as numbers.

## Live data

Tick parameters on the left (grouped, with a filter) or pick a ready set —
"Key values", "Idle", "Mixture and fuel". Polling starts by itself when the
page opens, with the set you chose for this control unit last time (the key
values the first time). Values ticked or unticked while polling take effect
at once: new values show up a moment later, without Stop and Start. The rate
changes at once too: as fast as possible, 4 times a second or once a second.
**Stop** stops polling, and other pages do not start it by themselves until
you press **Start** (except Gauges, which are opened for live values).

- **Table** — value, unit, session minimum and maximum.
- **Charts** — the last minute; one axis per parameter. Hover to read any
  moment.
- **Raw records** — for KWP2000 units: the record's bytes in a loop, with
  changing bytes highlighted. Helps to find and check values of a new unit.
- **Record CSV** — everything polled goes to a file (the "Recordings" folder)
  for Excel or LibreOffice.

A "?" next to a value means its formula has not been checked against a real
unit.

## Gauges

Dials for the values you choose, a trip computer and acceleration timing.
The program adds the values it needs to the polling by itself.

- **Choose values** — up to 12 dials, remembered for each control unit. The
  yellow and red zones are the thresholds of the Overview.
- **HUD** — big digits of speed and engine speed on black; "Mirror" flips
  them for a reflection in the windscreen.
- **Full screen** — the dials without the menu.
- **Trip computer** — current and average consumption, fuel used,
  distance, time, average and top speed. Consumption comes from the fuel
  rate or, without it, is estimated from the air flow (petrol engine at
  λ = 1). It counts from the first visit of the page until the program is
  closed; "Reset" starts again.
- **Acceleration timing** — 0–60, 0–100, 60–100, 80–120 km/h and 402 m (with
  imperial units 0–30, 0–60, 30–70 mph and ¼ mile). Choose a run, press
  **Start**, stop (or slow down) and go. During the run the program polls
  only the speed and engine speed, as fast as the line allows, and
  interpolates between two speed readings. The best time is starred. Only
  on a closed track.

## Alerts

While values are polled, the program watches for dangerous ones on any page:
overheating (from 115 °C), low supply voltage (11.5 V and below) or too high
(from 15.2 V), engine speed above 6500 rpm. A message with advice, a red mark
in the top bar (click it to open the Gauges) and a short sound appear; the
alert ends when the value is back to normal. **Settings** switch alerts off.

## Control unit

Identification (unit and software numbers, the VIN if the unit stores it), a
copy button, the description: status, vehicles, capabilities, sources. All
units of the session are listed at the bottom.

## Service

The control unit's service functions. A live strip on top shows engine
speed, temperature, voltage and the idle values, so you see at once how the
engine responds.

- **Restart the control unit** — like turning the key: temporary
  adjustments are dropped and the program restores communication by itself.
  For KWP2000 and UDS units (ecuReset).
- **Pulse tests** — injectors and ignition coils: a short pulse, listen for
  the click. Engine stopped, ignition on.
- **Relays and valves** — fuel pump, fan, A/C, check-engine lamp, canister
  purge. Anything left on returns to the control unit by itself after 60
  seconds (the fuel pump after 30); the card counts down.
- **Idle adjustments** — in real time: a slider, step buttons (±10 and ±50
  rpm) or a number. With **"Apply at once"** the value goes to the unit
  while you move the slider; the chart below shows the target and the
  actual value. "Hand back" returns control to the unit.

Each test states its condition — "engine stopped" or "engine running". The
program checks it against the engine speed and does not start the test when
the condition is not met or the speed cannot be read. The tests depend on
the description: they exist for VAZ with Bosch M1.5.4 / M1.5.4N,
Январь-5.1, VS-5.1 and Январь-7.2 (KWP2000). Микас-5.4 / 7.1 get none: the
commands are not published.

## Checks

Step-by-step checks with a pass or fail verdict from the live values. The
program says what to do (stop or start the engine, switch a load on),
measures and explains the result.

- **Battery and charging** — charge at rest, charging with the engine
  running, voltage under load.
- **Warm-up and thermostat** — how the coolant temperature rises.
- **Idle stability** — how steady the idle speed of a warm engine is.
- **Oxygen sensor** — whether the front sensor switches between rich and
  lean.
- **Throttle sensor** — engine stopped, press the pedal to the floor and
  release it: the reading must cover the whole range from 0 to 100 %.

A check is marked "Not available" when the unit does not report the values
it needs.

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
window mode, alerts about dangerous values and their sound, the check for a
new version at start; the ELM327 bridge; network access; **folders** — where
the data is (next to the program or in the profile) and buttons that open
journals, recordings, reports and control unit descriptions; **about** —
"What's new", "Check for updates", version, code database and licence.

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
udt reset   [conn]       restart the control unit and restore communication
udt bridge  [conn] [-listen 0.0.0.0:35000]
udt lookup  P0130 ...    describe codes offline
```

`udt dtc` prints under each code the repair advice section it belongs to.

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
  switch (K+DCAN) set to K-Line. "The cable stopped hearing its own bytes"
  means the cable lost its +12 V (ignition off, plug loose). Details:
  [GAZ-VOLGA.md](GAZ-VOLGA.md#eng).
- **ELM327:** Bluetooth — pair the adapter in the OS and choose the
  **outgoing** COM port; the program finds the port speed itself; cheap
  "v2.1" clones often lack commands.
- **ELM327 Bluetooth LE:** the computer's Bluetooth is on; the adapter is in
  the car with the ignition on, and no phone is connected to it (a BLE
  adapter talks to one device at a time). No pairing in the OS is needed.
- **CAN FD:** it needs an FD adapter — PCAN-USB FD, CANable 2.0, or a
  SocketCAN interface set up with `ip link set can0 type can bitrate 500000
  dbitrate 2000000 fd on`. A CANable 2.0 runs the data phase at 2 or 5
  Mbit/s.
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
- Clearing codes, service functions and the terminal change the unit's
  state. Do them with the vehicle stationary. Tests with an engine condition
  are not started without checking the engine speed, and outputs left on
  are handed back to the unit by a timer and on disconnect.

## Support the project

The **"♥ Support"** section of the sidebar: links to Ko-fi, Buy Me a Coffee
and DonationAlerts, and ways to help without money — test the program on your
car, send a journal, check a control unit description. The program is free
and will stay free.
