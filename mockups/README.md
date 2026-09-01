# OmaStack QML design mock-ups

These files are fixture-only design-gate artifacts. They deliberately contain
no process supervision, privileged operations, DNS changes or certificate
installation.

The main review board is `qml/OmaStackMockup.qml`. It uses the active Omarchy
font alias, square default geometry, compact panel spacing, Nerd Font glyphs
and bounded 60-sample CPU/memory sparklines. The Tokyo Night and Catppuccin
Latte wrappers override only the theme; layout and fixture data stay identical.

Headless render command:

```sh
QT_QPA_PLATFORM=offscreen QT_QPA_PLATFORMTHEME= QT_QUICK_BACKEND=software \
  /usr/lib/qt6/bin/qmlscene mockups/qml/OmaStackMockup.qml
```

Run the equivalent command with `RenderTokyoNight.qml` or
`RenderCatppuccinLatte.qml` for the other review images. The empty
`QT_QPA_PLATFORMTHEME` is intentional: it prevents the host's GTK Qt platform
theme from requiring a live display during headless rendering.
