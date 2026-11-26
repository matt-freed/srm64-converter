# srm64-converter

`srm64` converts [Mupen64Plus-Next](https://github.com/libretro/mupen64plus-libretro-nx)
(libretro/RetroArch) `.srm` save files to and from native N64 save files. The native files
are the ones cart dumpers and other emulators use: `.eep`, `.sra`, `.fla` and `.mpk`.
[`SRM_SPEC.md`](SRM_SPEC.md) describes the `.srm` format.

## Native save types

N64 games save to one of three kinds of chip on the cartridge, or to a Controller Pak
plugged into the controller. Each game uses at most one cartridge save type, and some
games also save to a pak. To find which type a game uses, see the
[micro-64 game save database](http://micro-64.com/database/gamesave.shtml).

| Type           | Ext    | Size             | Where it lives        |
|----------------|--------|------------------|-----------------------|
| EEPROM         | `.eep` | 512 B or 2 KB    | Cartridge (4/16 Kbit) |
| SRAM           | `.sra` | 32 KB            | Cartridge (256 Kbit)  |
| FlashRAM       | `.fla` | 128 KB           | Cartridge (1 Mbit)    |
| Controller Pak | `.mpk` | 32 KB per pak    | Controller accessory  |

An `.srm` holds every type at once: one EEPROM, four Controller Paks (one per
controller), one SRAM and one FlashRAM. Usually only one cartridge region holds data.

## Install

Prebuilt binaries are on the [Releases page](https://github.com/matt-freed/srm64-converter/releases).
A new versioned release is published automatically after every successful build of `main`.

On macOS or Linux (x86-64 or arm64), you can install or update to the latest release with:

```sh
curl -fsSL https://raw.githubusercontent.com/matt-freed/srm64-converter/main/install.sh | sh
```

This installs to `~/.local/bin`; set `PREFIX` to change it, or `SRM64_VERSION=v0.1.N` to
pick a release. It also skips the macOS quarantine step below.

| File | Platform |
|---|---|
| `srm64-macos-universal.tar.gz` | macOS (Apple Silicon and Intel) |
| `srm64-linux-x86_64.tar.gz` | Linux x86-64 |
| `srm64-linux-arm64.tar.gz` | Linux arm64 (e.g. Raspberry Pi 4/5 with a 64-bit OS) |
| `srm64-windows-x86_64.zip` | Windows x64 |

On macOS the binary isn't signed by a registered Apple developer, so if you download it in
a browser, macOS blocks it the first time you run it. To allow it, run:

```sh
xattr -d com.apple.quarantine srm64
```

You can also allow it under System Settings → Privacy & Security. This isn't needed if you
build from source.

Release versions are numbered `<major>.<minor>.<build>`. The major and minor numbers come
from the `VERSION` file, and the build number is the CI run number. To start a new series
(for example 0.2.x), change the version there. `srm64 --version` shows the version of a
binary.

## Build

The only requirement is [Go](https://go.dev/dl/) (see `go.mod` for the version).

```sh
go build -o srm64 ./cmd/srm64
go vet ./... && go test ./...
```

## Usage

```sh
srm64 inspect "Game.srm"                                       # which regions hold data, pak notes
srm64 inspect Game.eep                                         # size, status and notes of a native file
srm64 split "Game.srm" [-o DIR] [--skip-byte-swap] [-f]              # writes Game.eep / .sra / .fla / .mpk
srm64 pack Game.sra [Game.mpk ...] [-o OUT.srm] [--skip-byte-swap] [-f]  # writes Game.srm
srm64 insert "Game.srm" Game.mpk [--slot N] [-o OUT.srm] [--skip-byte-swap] [-f]  # add files to an existing .srm
```

- **Split** writes only the regions that hold data (`--all` writes every region). Pak 1
  becomes `GAME.mpk`, and paks 2–4 become `GAME.N.mpk`.
- **EEPROM size:** the `.srm` doesn't record it. `split` writes 512 B (4 Kbit) when
  everything after byte 512 is `0xFF`, and 2048 B (16 Kbit) otherwise. Use
  `--eep-size 512|2048` to choose the size yourself.
- **Pack** reads each file's type from its extension (pak slots are covered below).
  Regions you don't supply stay blank, the same as a new save the emulator creates.
  Without `-o`, the output is named after the first file and written beside it:
  `Game.eep` or `Game.2.mpk` gives `Game.srm`.
- **Insert** reads files the same way, but places them into an existing `.srm`. Only the
  regions those files cover change. By default the `.srm` is updated in place; with `-o`
  the result is written to a new file and the original is left alone.
- **Replacing data:** if any region `insert` would write, cart save or pak slot,
  already holds data, it lists those regions and asks `Replace? [y/N]` before writing
  anything. When stdin isn't a terminal (a script or pipe), it doesn't ask; it stops
  with an error instead. `-f` replaces without asking.
- **Byte order (`.sra` / `.fla`):** Mupen64Plus-Next stores SRAM and FlashRAM with every
  4-byte word reversed. `split`, `pack` and `insert` swap them, so `.sra` and `.fla` files
  are in the cartridge's own order, which is what cart dumpers and flashers read and
  write. Pass `--skip-byte-swap` for files that are already in Mupen64Plus order, e.g.
  from standalone Mupen64Plus or a converter that copies the data without swapping.
  `.eep` and `.mpk` files are never swapped. To check a file's order, look for readable
  text from the game: in the cartridge's order it reads normally.
- **Pak slots** (`pack` and `insert`): each `.mpk` goes in the first slot that applies:
  1. `--slot N` (1–4), if given. This only works with a single `.mpk` file.
  2. N from a `GAME.N.mpk` file name.
  3. The lowest-numbered slot that has no notes and that no other file claims.
  4. If there is no such slot, the command fails without writing and asks for `--slot N`.

  `--slot` and `GAME.N.mpk` can pick a slot that already holds data, which `insert`
  confirms first, as above. A 128 KB `.mpk` fills all four slots. Because `split` writes
  pak 1 as `GAME.mpk`, inserting that file into an `.srm` that already has a pak 1 puts
  it in the next free slot. To replace pak 1, use `--slot 1`.
- Commands never overwrite an existing file unless you pass `-f`. The exception is
  `insert` updating its own input `.srm`, which is the point of the command; replacing
  data inside it is confirmed as described above.
- Exit codes: 0 for success, 1 for an error, 2 for a bad command line.

## License

MIT. See [LICENSE](LICENSE).
