# Contributing an application

One application per pull request. The app must be a CLI/TUI program that is
useful on a 320×170 screen with a matrix keyboard.

## Requirements

- A desktop entry in `/usr/share/applications` with `Name`, `Name[uk]`,
  `Comment`, `Comment[uk]`, `Exec`, `Terminal=true` and a main freedesktop
  category.
- `X-CPZ-Contract=1` if the app relies on the CPZ launch or tab semantics.
- A normal pacman package under `apps/<name>/PKGBUILD`. The package is the
  install/remove unit.
- No daemon, no install script, no root at runtime, no telemetry, no GUI
  toolkit, no network on startup.
- `desktop-file-validate` passes.
- Packaged under `apps/<name>/` with the sources or a pinned upstream release.

## Review

- The app works with plain `F`/`X`/`Z`/`C`, arrows, `Enter`, `Esc` and `Tab`;
  no Fn-only key is the only way to do something.
- It looks right at 40×10 and at the panel grid; labels do not clip.
- Strings come from a catalog (`internal/i18n` or the same precedence).
- The first frame is under 100 ms; no network or subprocess before it.
- Install and removal both work; removal leaves no files outside `/home`.
- The license allows redistribution.

## Checks

```sh
make test
make fmtcheck
make package       # builds the catalog and the local repository
```
