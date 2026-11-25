# cpzos-apps

Applications and the application catalog for CPZ OS (Arch Linux ARM).
This repository is the source of the signed `[cpz-apps]` pacman repository;
it is not part of the base image.

```text
cpz-system-apps     Home, Apps Manager, Settings, Terminal
cpz-apps-catalog    data-only catalog of available apps (apps.json)
cpz-app-<name>      applications; each installs desktop entries
```

The base image trust root is the existing CPZ UI signing key. The UI image
bakes the current catalog at build time; Apps Manager refreshes it in the
background through a narrow helper.

## System apps

Home, Apps Manager and Settings are one binary on the shared `internal/tui`
kit; Terminal is a desktop entry that starts a login shell. Home and the
catalog list only the system apps and installed apps that come from the
catalog; unrelated desktop entries shipped by the session (foot, help) are not
apps. All four follow the binding rules in `cpz-arch/docs/TUI-CONTRACT.md`:

- plain `F` / `X` / `Z` / `C` navigation (no Fn), arrows, `Enter`, `Esc`;
- 40×10 minimum layout, one focus bar, no borders, no footer;
- first frame ≤ 100 ms, no network or sudo before it.

`cpz-app` is one multicall binary:

```sh
cpz-app home       # new tab: choose an app
cpz-app apps       # Apps Manager
cpz-app settings   # Settings, currently About system
cpz-app run FILE NAME  # internal: run an argv file and return to Home
cpz-app exec FILE      # internal: run an argv file (tmux-safe, no shell)
```

The `run` supervisor is what makes quitting an app land back on Home instead
of an empty tab. It has no input loop, so the app owns the terminal.

Terminal is `/usr/share/applications/cpz-terminal.desktop` (`Exec=bash -l`,
`Terminal=true`, `X-CPZ-System=true`); it needs no code in the binary.

## Headless

Every screen action is also a command:

```sh
cpz-app apps list [--installed|--available] [--category C] [--query Q] [--json]
cpz-app apps install <package|desktop-id> [--yes] [--json]
cpz-app apps remove  <package|desktop-id> [--yes] [--json]
cpz-app apps update  [--yes] [--json]
cpz-app settings about [--json]
```

Exit codes: 0 ok, 1 failed, 2 usage, 3 needs `--yes`, 4 unknown app,
5 protected, 6 offline.

## Build

```sh
make build        # host binary in dist/
make test
make package      # Arch environment: packages + catalog + dist/repo
```

`make package` needs `makepkg`, `repo-add`, `go` and `bsdtar`. To install on a
device for testing, point a `[cpz-apps]` repository at `dist/repo` and run
`pacman -Sy cpz-system-apps cpz-apps-catalog`.

## Catalog

`tools/build-catalog` reads the built packages with `bsdtar`, takes every
`/usr/share/applications/*.desktop`, and writes one schema-1 `apps.json`:

- name, comment, keywords and categories come from the desktop entry;
- the package is the install/remove unit;
- `X-CPZ-System=true` and the `cpz-system-apps` package are never removable;
- the catalog is data only (`depends=()`), which is why the background refresh
  can install it with `-S` without a full upgrade.

## Example application

`apps/cpz-app-mc` wraps the Arch `mc` package: it installs only a desktop
entry and depends on `mc`. The catalog lists it as available; installing it
through Apps Manager pulls `mc` from the Arch repositories. Removing it removes
the entry and, if nothing else needs it, `mc`.

## Curation

Pull requests only. One application per PR, with a desktop entry, a PKGBUILD
and a working install/remove path. No services, no install scripts, no GUI
toolkits, no telemetry. See `CONTRIBUTING.md`.

## Layout

```text
cmd/cpz-app/            multicall entry point and headless commands
internal/tui/           theme, keymap, list, screen, input, confirm
internal/i18n/          system language resolution and catalogs
internal/catalog/       apps.json schema and loader
internal/apps/          XDG discovery, desktop parsing, merge
internal/pacman/        read-only queries and the helper client
internal/launch/        argv expansion, tmux tab, window replacement
internal/home/          new-tab launcher
internal/store/         Apps Manager
internal/settings/      Settings and About
internal/about/         system facts for About
system/                 desktop entries, helper, sudoers, protected list
packaging/              PKGBUILDs for cpz-system-apps and cpz-apps-catalog
tools/                  catalog generator and local repository builder
```
