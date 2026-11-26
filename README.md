# cpzos-apps

Applications and the application catalog for CPZ OS (Arch Linux ARM).
This repository is the source of the signed `[cpz-apps]` pacman repository;
it is not part of the base image.

```text
cpz-system-apps     Home, Apps Manager, Settings, Roms, Terminal
cpz-apps-catalog    data-only catalog of available apps (apps.json)
cpz-app-<name>      applications; each installs desktop entries
```

The base image trust root is the existing CPZ UI signing key. The UI image
bakes the current catalog at build time; Apps Manager refreshes it in the
background through a narrow helper.

## System apps

Home, Apps Manager, Settings and Roms are one binary on the shared
`internal/tui` kit; Terminal is a desktop entry that starts a login shell. Home
and the catalog list only the system apps and installed apps that come from
the catalog; unrelated desktop entries shipped by the session (foot, help) are
not apps. Home, Apps Manager, Settings and Roms follow the binding rules in
`cpz-arch/docs/TUI-CONTRACT.md`:

- plain `F` / `X` / `Z` / `C` navigation (no Fn), arrows, `Enter`, `Esc`;
- 40×10 minimum layout, one focus bar, no borders, no footer;
- first frame ≤ 100 ms, no network or sudo before it.

`cpz-app` is one multicall binary:

```sh
cpz-app home       # new tab: choose an app
cpz-app apps       # Apps Manager
cpz-app settings   # Settings: update system and About
cpz-app roms [--system S]  # ROM list for NES/SNES/Genesis/GB/GBA
cpz-app run FILE NAME  # internal: run an argv file and return to Home
cpz-app exec FILE      # internal: run an argv file (tmux-safe, no shell)
```

The `run` supervisor is what makes quitting an app land back on Home instead
of an empty tab. It has no input loop, so the app owns the terminal.

Home and Apps Manager show `Starting <name>…` while a graphical app (Freedoom,
for example) is loading and replace it with the failure reason if the child
exits non-zero, so a slow launch never looks like a dead key. A graphical
entry is single-instance across tabs: the same tab refuses a second launch
until the first exits, and another tab reports `already running`. Terminal
entries are tabs and stay multi-instance.

Terminal is `/usr/share/applications/cpz-terminal.desktop` (`Exec=bash -l`,
`Terminal=true`, `X-CPZ-System=true`); it needs no code in the binary.

## Headless

Every screen action is also a command:

```sh
cpz-app apps list [--installed|--available] [--category C] [--query Q] [--json]
cpz-app apps install <package|desktop-id> [--yes] [--json]
cpz-app apps remove  <package|desktop-id> [--yes] [--json]
cpz-app apps update  [--yes] [--json]
cpz-app settings about  [--json]
cpz-app settings update [--yes] [--json]
```

Exit codes: 0 ok, 1 failed, 2 usage, 3 needs `--yes`, 4 unknown app,
5 protected, 6 offline.

`settings update` and the Settings **Update system** row run a full
`pacman -Syu` through the helper, which upgrades the UI, the system apps and
the catalog. The helper ignores `linux-rpi` and `raspberrypi-bootloader`: the
boot files are generated into the FAT partition by the image build, so a new
kernel or bootloader needs a new image, not an in-place transaction.

`apps update` and the Apps Manager **Update list and apps** row run a full
`pacman -Syu` (the kernel and bootloader stay pinned), then force-refresh the
catalog. Installed apps and the system apps move to the versions in
`[cpz-apps]`, and new catalog entries appear. Only the automatic refresh on
entry is limited to one attempt per 10 minutes and updates the catalog alone.
Settings → **Update system** upgrades the system and apps but does not force
the catalog reinstall.

While a package operation runs, Apps Manager and Settings lock the screen and
show an ASCII spinner. All keys are ignored except `Esc` (or `Ctrl+C`), which
asks the helper to stop pacman. A transaction interrupted in the middle may
need to be retried; the next update repairs the package database.

## Build

```sh
make build        # host binary in dist/
make test
make package      # Arch environment: packages + catalog + dist/repo
```

`make package` needs `makepkg`, `repo-add`, `go` and `bsdtar`. To install on a
device for testing, point a `[cpz-apps]` repository at `dist/repo` and run
`pacman -Sy cpz-system-apps cpz-apps-catalog`.

The UI image does not read this working tree. It installs `cpz-apps-catalog`
from the configured `[cpz-apps]` repository, so adding a directory under
`apps/` does nothing until the repository is rebuilt. A local `file://` test
image also bakes a snapshot of that repository into `/opt/cpz/apps-repo`;
replace that copy or rebuild the image when `dist/repo` changes. App Manager's
**Update list and apps** installs newer packages from the repository, including
`cpz-apps-catalog`; it never generates a catalog from source.

## Catalog

`tools/build-catalog` reads the built packages with `bsdtar`, takes every
`/usr/share/applications/*.desktop`, and writes one schema-1 `apps.json`:

- name, comment, keywords and categories come from the desktop entry;
- the package is the install/remove unit;
- `X-CPZ-System=true` and the `cpz-system-apps` package are never removable;
- the catalog is data only (`depends=()`), which is why the background refresh
  can install it with `-S` without a full upgrade.

The catalog package version is the UTC build time
(`YYYY.MM.DD.HHMMSS`). A date-only version makes two rebuilds from the same day
compare equal, so pacman skips the second one and devices keep stale data.

`tools/check-catalog` runs after the catalog is generated. It fails the build
if any desktop entry under `apps/` is missing from `apps.json`, so a release
cannot be signed or published with an app left out.

## Published repository

The signed `[cpz-apps]` repository is the `apps-stable` release:

```text
https://github.com/jmas/cpzos-apps/releases/download/apps-stable
```

The catalog JSON is `usr/share/cpz/catalog/apps.json` inside the
`cpz-apps-catalog-<version>-1-any.pkg.tar.xz` asset; on a device the installed
copy is `/usr/share/cpz/catalog/apps.json`. CI builds and signs every `v*` tag
with `tools/sign-repo.sh`; it never publishes a tag without the signing key.

Publishing rules:

- a push to `main` does not publish; only a `v*` tag runs the package job;
- the job fails if any `apps/*/*.desktop` entry is missing from the catalog;
- a `file://` test image is a snapshot, not the release: App Manager cannot see
  a newer release until the image is rebuilt or `/opt/cpz/apps-repo` is
  replaced;
- Apps Manager shows available apps in the catalog; Home shows only installed
  apps. A newly published app appears under Games as available and must be
  installed.

## Example application

`apps/cpz-app-mc` wraps the Arch `mc` package: it installs only a desktop
entry and depends on `mc`. The catalog lists it as available; installing it
through Apps Manager pulls `mc` from the Arch repositories. Removing it removes
the entry and, if nothing else needs it, `mc`.

## Games

Every game is its own package, so Apps Manager installs and removes one game
at a time. The Freedoom entries use the `doomretro` engine and ship only free
Freedoom data:

- `Freedoom: Phase 1` (`freedoom1.wad`) and `Freedoom: Phase 2`
  (`freedoom2.wad`) are the Doom- and Doom II-compatible games;
- `FreeDM` (`freedm.wad`) is the deathmatch WAD for Doom II;
- `NetHack`, `NBSDGAMES!`, `GNU Chess`, `GNU Go` and
  `Colossal Cave Adventure` are terminal games that fit the panel;
- `The Dreamhold` and `Bronze` are freeware Z-code interactive fiction,
  installed as separate story packages with `frotz-ncurses` as the interpreter.

The interactive-fiction entries are named after the stories, not the
interpreter. They are freeware rather than open source; the author, source URL
and redistribution note are in `/usr/share/doc/<package>/NOTICE`.

`NetHack` depends on the patched `nethack` package in this repository. The
Arch package enables GDB panic tracing and points `GDBPATH` at `/usr/bin/gdb`;
the device does not ship gdb, and NetHack treats the missing path as a fatal
sysconf error. The package keeps the game and disables only the GDB trace.

`GNU Go` starts a 9x9 game because a 19x19 board is wider than the panel.
Games that need a much larger text grid are deliberately not packaged:
Dungeon Crawl Stone Soup and Cataclysm: Dark Days Ahead do not fit 45x12, and
`tty-solitaire` refuses to start below 57x28. The panel is the limit; do not
add an entry that cannot be played on it.

The catalog name rule is in `CONTRIBUTING.md`: the entry is named after the
game that actually starts, not its engine. The Freedoom packages are why
`cpz-app-freedoom-phase1` replaces the old `cpz-app-doom` package.

## Roms

`cpz-app roms` lists ROMs under:

```text
~/roms/nes/
~/roms/snes/
~/roms/genesis/
~/roms/gb/
~/roms/gba/
```

It also scans `/run/media/$USER/*/roms/<system>` and
`/media/$USER/*/roms/<system>` for USB sticks. The **Roms** system entry shows
every system; the catalog entries **NES**, **SNES**, **Sega Genesis**,
**Game Boy** and **Game Boy Advance** install the matching emulator and open
the list filtered to that system.

Mednafen handles NES, SNES and Genesis; `cpz-mgba` is a minimal mGBA build for
Game Boy, Game Boy Color and Game Boy Advance. No ROMs or BIOS files are
shipped. Mednafen uses `F`/`X`/`Z`/`C`, `J`/`K`/`L`, `U`/`I`,
`Enter`/`Backspace`; mGBA gets the same d-pad through explicit `-C` bindings.
Both avoid the Fn layer.

## Media and Open with

The base image mounts USB filesystems with `cpz-automount` under
`/media/<label>` and records them in `/run/cpz-automount`. Home lists those
disks above the apps and updates when a disk is plugged or unplugged.

Selecting a disk opens a menu:

- **Open** uses the default handler for the path;
- **Open with…** chooses another installed handler;
- **Unmount** calls `/usr/lib/cpz/media-umount`, which forwards to
  `cpz-automount` as root.

`internal/openwith` is the single entry point for opening a path. It reads
`/etc/cpz/open-with.conf` and `~/.config/cpz/open-with.conf` (keys are
`directory` or a MIME type, values are desktop entry ids) and otherwise uses
installed `MimeType` handlers. Directories fall back to Terminal; installing
Midnight Commander adds it to **Open with…**. The same function is available
headlessly:

```sh
cpz-app open <path> [--with desktop-id] [--list]
```

## Curation

Pull requests only. One application per PR, with a desktop entry, a PKGBUILD
and a working install/remove path. No services, no install scripts, no GUI
toolkits, no telemetry. Names follow the upstream product name; see the
**Names** section of `CONTRIBUTING.md`.

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
