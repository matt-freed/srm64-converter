# srm64-converter

Go CLI (`srm64`) that converts Mupen64Plus-Next `.srm` save containers to and from
native N64 save files (`.eep`, `.sra`, `.fla`, `.mpk`). `SRM_SPEC.md` describes the
format, and `README.md` covers usage.

## Commands

```sh
go build -o srm64 ./cmd/srm64
go vet ./... && go test ./...
gofmt -l .                      # must print nothing
```

## Hard rules

- **`SRM_SPEC.md` is the source of truth for the format.** When behaviour changes
  (sizes, offsets, EEPROM guess, naming, blank fills), update the spec in the same
  change. Region offsets and sizes come from libretro's `save_memory_data` struct and
  are fixed. `TestLayout` pins them.
- **No real save files in the repo or the tests.** Tests use
  generated data (seeded `math/rand/v2`) and must never rely on committed save files. With 
  samples present, a manual check is fine: split each `.srm`, `cmp` against the
  sample natives, re-pack, then `cmp` against the original.
- **Packing must reproduce the emulator's output byte-for-byte.** Cart regions
  default to `0xFF`, and each pak slot defaults to `internal/srm/blankpak.mpk`, which
  was copied from an unused slot of a real `.srm`. Don't regenerate or edit that
  file. Every unused slot Mupen64Plus-Next writes is identical to it, including the
  serial.
- **No byte-swapping.** Regions are stored exactly as the native files. Other
  emulators' word-swapped `.sra`/`.fla` (e.g. Project64) aren't supported. Adding a
  swap would be a new, opt-in flag.
- **Mempak support is deliberately minimal:** note-table parsing only (game code,
  publisher, raw name bytes), with no font decoding or filesystem logic. 
- **CLI conventions**:
  - Exit codes are 0 ok, 1 runtime error, 2 usage error. Return `usagef(...)` /
    `usageError{err}` for bad command lines and wrap cobra arg checks with `args(...)`.
  - Commands print to `cmd.OutOrStdout()`, which is the buffered `stdout` flushed in
    `main`. Don't write to `os.Stdout` directly. Warnings go to stderr.
  - Files are written through `writeFile`, which is atomic and refuses to overwrite
    unless `-f` is given. The one exception is `insert` updating its own input `.srm`,
    which it always does. `-f` only matters for `insert -o`.
  - `pack` always builds a new `.srm` from blank, and `insert` edits an existing one.
    Both share `placeFiles` in `pack.go`.
  - `insert` asks before replacing any region that already holds data
    (`confirmReplace` in `confirm.go`), and refuses when stdin isn't a terminal. `-f`
    skips the prompt. `placeFiles` only changes the in-memory image and returns the
    replaced regions, so nothing touches disk until after confirmation. Keep it that
    way. Terminal detection uses `golang.org/x/term`. Don't go back to a
    `ModeCharDevice` check, because `/dev/null` passes it. Tests stub
    `stdinIsTerminal` (`runIn` in `main_test.go`).
  - Pak slot rules (`--slot`, then `.N.mpk`, then the first unused slot, then an
    error) are specified in `SRM_SPEC.md` under *Pak slot selection*.
    `srm.AssignPakSlots` implements them. `KindFromPath` returns slot 0 for "no slot
    in the name", so don't treat 0 as slot 1 anywhere.

## Layout

- `internal/srm`: pure data, with no file I/O. `Regions` is the single table of the
  layout, and the rest of the code loops over it. `Place` (native → image) and
  `Native` / `NativeName` (image → native file and name) are inverses, and
  `KindFromPath` maps a `NativeName` back to its region. The round-trip tests depend
  on that.
- `cmd/srm64`: cobra commands. `newRoot(out)` builds the command tree so tests can
  run the real CLI (`main_test.go`) in a temp dir.

## Known limitation

The `.srm` doesn't record EEPROM size. `split` guesses 512 B when bytes
`0x200`–`0x7FF` are all `0xFF`, which is wrong for a 16 Kbit game that hasn't
written past byte 512. `--eep-size` overrides the guess. Keep the guess, the
override, and the spec's description consistent.

## CI and releases

`.github/workflows/build.yml` is modelled on `mempak-tools`:
- **Pull requests and pushes to `main`:** vet, test and a CLI smoke test on Linux, macOS
  and Windows (`gofmt` is checked on Linux), then packaging.
- **Pushes to `main` only:** publishes a GitHub release tagged `v<major>.<minor>.<run number>`.
  `VERSION` holds `major.minor`. The version is stamped in with `-X main.version`, so a
  local build reports `dev`.

Package names (`srm64-macos-universal.tar.gz`, `srm64-linux-x86_64.tar.gz`,
`srm64-linux-arm64.tar.gz`, `srm64-windows-x86_64.zip`) and the `srm64-<platform>/srm64`
layout inside each tarball are what `install.sh` downloads and unpacks, so change them
together. Release builds use `CGO_ENABLED=0`, so keep the code cgo-free. Test
`install.sh` locally with `SRM64_URL=file:///path/to/pkg.tar.gz PREFIX=/tmp/x sh install.sh`.
