<a id="ru"></a>

**Русский** · [English](#eng)

# Ресурсы Windows

Значок, манифест и сведения о версии (на русском и английском) для
`udt.exe` и `udt-x86.exe`. Уже собраны в `../rsrc_windows_amd64.syso` и
`../rsrc_windows_386.syso`, которые `go build` подхватывает сам — для обычной
сборки здесь ничего делать не нужно.

Пересобрать после изменений (нужны Python с Pillow и `windres` из MinGW):

```sh
cd cmd/udt/winres
python make_icon.py
windres -c 65001 -O coff -F pe-x86-64 -i app.rc -o ../rsrc_windows_amd64.syso
windres -c 65001 -O coff -F pe-i386 -i app.rc -o ../rsrc_windows_386.syso
```

Значок — диагностическая колодка OBD-II с линией пульса на тёмном скруглённом
квадрате, как у программы «Сход-развал». Для 16–24 пикселей рисуется
упрощённый вариант без контактов, чтобы значок в заголовке окна оставался
чётким. Тот же рисунок — `internal/server/web/assets/icon.svg` в интерфейсе.

Манифест объявляет поддержку высокого DPI (PerMonitorV2): без него окно на
экранах с масштабом 125–200 % выглядит размытым.

При выпуске новой версии поменяйте номер версии в `app.rc` (все шесть мест) и
в `app.manifest`, затем пересоберите оба файла `.syso`.

---

<a id="eng"></a>

[Русский](#ru) · **English**

# Windows resources

The icon, manifest and version information (in Russian and English) for
`udt.exe` and `udt-x86.exe`. They are already compiled into
`../rsrc_windows_amd64.syso` and `../rsrc_windows_386.syso`, which `go build`
picks up by itself — nothing to do here for an ordinary build.

To rebuild after changes (needs Python with Pillow and `windres` from MinGW):

```sh
cd cmd/udt/winres
python make_icon.py
windres -c 65001 -O coff -F pe-x86-64 -i app.rc -o ../rsrc_windows_amd64.syso
windres -c 65001 -O coff -F pe-i386 -i app.rc -o ../rsrc_windows_386.syso
```

The icon is an OBD-II diagnostic socket with a pulse line on a dark rounded
square, like the wheel-alignment program's. The 16–24 pixel sizes are a
simpler drawing without the contacts, so the icon in the window title bar
stays sharp. The same picture is `internal/server/web/assets/icon.svg` in the
interface.

The manifest declares high-DPI support (PerMonitorV2): without it the window
looks blurry on screens scaled to 125–200 %.

For a new release, change the version number in `app.rc` (all six places) and
in `app.manifest`, then rebuild both `.syso` files.
