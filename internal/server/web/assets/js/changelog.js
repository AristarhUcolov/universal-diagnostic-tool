// What changed in each release, shown by "What's new". The newest comes
// first; a release without a date is still being prepared. Add the entry
// of a new release here before tagging it.
export const releases = [
  {
    version: '0.4.0', date: '2026-10-23',
    ru: [
      'Новый вид: значок программы, тёмная тема в его цвет, значки разделов; окно запоминает размер и положение.',
      'Палитра команд (Ctrl+K): раздел, действие, параметр или код неисправности — по нескольким буквам.',
      '«Параметры»: отмеченные и снятые галочки применяются сразу, без «Стоп» и «Старт»; опрос начинается сам, а в верхней строке видно, сколько значений опрашивается и как часто.',
      '«Обзор» показывает основные значения сразу после подключения.',
      'Проверка обновлений: программа сообщает о новой версии и ничего не скачивает сама; выключается в настройках.',
      'Попробовать без машины: кнопка «Попробовать на симуляторе» на каждой странице, где нужно подключение.',
    ],
    en: [
      'A new look: the program icon, a dark theme in its colours, icons in the menu; the window remembers its size and place.',
      'Command palette (Ctrl+K): any page, action, value or trouble code from a few letters.',
      'Live data: ticking and unticking values takes effect at once, without Stop and Start; polling starts by itself, and the top bar shows how many values are polled and how often.',
      'The overview shows the main values right after connecting.',
      'Update check: the program tells about a new version and never downloads anything by itself; it can be switched off in the settings.',
      'Try without a car: a "Try the simulator" button on every page that needs a connection.',
    ],
  },
  {
    version: '0.3.0', date: '2026-10-23',
    ru: [
      'Всё хранится в папке udt-data рядом с программой — её можно носить на флешке.',
      '«Приборы»: стрелочные приборы, режим HUD, бортовой компьютер и замер разгона 0–100.',
      '«Мониторы»: готовность мониторов OBD-II и самопроверки блока (режим 06) с допусками.',
      'Предупреждения о перегреве, напряжении и перекруте на любой странице.',
      'Обороты холостого хода и РДВ меняются в реальном времени.',
      'CAN FD для UDS и адаптеры ELM327 Bluetooth LE.',
    ],
    en: [
      'Everything is kept in the udt-data folder next to the program — carry it on a flash drive.',
      'Gauges: dials, a HUD mode, a trip computer and 0–100 timing.',
      'Monitors: OBD-II readiness and the controller’s own tests (service 06) with limits.',
      'Alerts about overheating, voltage and over-revving on any page.',
      'Idle speed and the idle air valve set in real time.',
      'CAN FD for UDS and ELM327 Bluetooth LE adapters.',
    ],
  },
  {
    version: '0.2.0', date: '2026-10-22',
    ru: [
      'Сервис: перезагрузка блока, тесты механизмов с условиями по оборотам, регулировки холостого хода.',
      'Пошаговые проверки: аккумулятор и генератор, термостат, холостой ход, датчик кислорода, дроссель.',
      '«Как исправить» у каждого кода неисправности: признаки, причины, проверки и срочность.',
      'Микас-5.4/7.1 проверен на настоящей ГАЗ-3110: исправлены список ошибок и расход воздуха.',
    ],
    en: [
      'Service: control unit restart, output tests with engine-speed conditions, idle adjustments.',
      'Guided checks: battery and alternator, thermostat, idle, oxygen sensor, throttle.',
      '"How to fix" for every trouble code: symptoms, causes, checks and urgency.',
      'Микас-5.4/7.1 checked on a real GAZ-3110: the fault list and air flow fixed.',
    ],
  },
  {
    version: '0.1.0', date: '2026-10-22',
    ru: [
      'Первый выпуск: ошибки с расшифровкой, параметры в реальном времени, графики и запись в CSV.',
      'Адаптеры: KKL, ELM327, J2534, CANable, PCAN, Kvaser, SocketCAN, RP1210, DoIP и BMW ENET.',
      'Протоколы: OBD-II, KWP2000, ISO 9141-2, UDS, J1939, KW1281, TP 2.0, Микас и Январь-4.',
      'Мост ELM327, доступ с телефона и симулятор с десятью машинами.',
    ],
    en: [
      'First release: trouble codes with descriptions, live data, charts and CSV recording.',
      'Adapters: KKL, ELM327, J2534, CANable, PCAN, Kvaser, SocketCAN, RP1210, DoIP and BMW ENET.',
      'Protocols: OBD-II, KWP2000, ISO 9141-2, UDS, J1939, KW1281, TP 2.0, Микас and Январь-4.',
      'ELM327 bridge, access from a phone and a simulator with ten cars.',
    ],
  },
];
