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

## Build

```sh
go build -o srm64 ./cmd/srm64
go vet ./... && go test ./...
```

## Usage

```sh
srm64 inspect "Game.srm"                    # which regions hold data, pak notes
srm64 split "Game.srm" [-o DIR] [-f]        # writes Game.eep / .sra / .fla / .mpk
srm64 pack Game.eep [Game.mpk ...] [-o OUT.srm]  # writes Game.srm
srm64 pack -o "Game.srm" --base old.srm Game.mpk   # merge into an existing .srm
```

- **Split** writes only the regions that hold data (`--all` writes every region). Pak 1
  becomes `GAME.mpk`, and paks 2–4 become `GAME.N.mpk`.
- **EEPROM size:** the `.srm` doesn't record it. `split` writes 512 B (4 Kbit) when
  everything after byte 512 is `0xFF`, and 2048 B (16 Kbit) otherwise. Use
  `--eep-size 512|2048` to choose the size yourself.
- **Pack** reads each file's type from its extension. A file named `GAME.N.mpk` goes in
  pak slot N, and a 128 KB `.mpk` fills all four slots. Regions you don't supply stay blank,
  the same as a new save the emulator creates. Without `-o`, the output is named after
  the first file and written beside it: `Game.eep` or `Game.2.mpk` gives `Game.srm`.
- Neither command overwrites an existing file unless you pass `-f`.
- Exit codes: 0 for success, 1 for an error, 2 for a bad command line.

## License

MIT. See [LICENSE](LICENSE).
