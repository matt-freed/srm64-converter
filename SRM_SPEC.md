# Mupen64Plus-Next `.srm` Format

Mupen64Plus-Next (libretro) saves N64 cartridge and controller-pak data as one
fixed-size `.srm` file. The file is a raw dump of the C struct `save_memory_data`,
which libretro returns from `retro_get_memory_data(RETRO_MEMORY_SAVE_RAM)`.

### Why one file

The layout comes from the [libretro API](https://github.com/libretro/RetroArch/blob/master/libretro-common/include/libretro.h).
A core does not write its own save files. It exposes one block of save memory through
`retro_get_memory_data(RETRO_MEMORY_SAVE_RAM)` and `retro_get_memory_size(RETRO_MEMORY_SAVE_RAM)`.
The frontend (e.g. RetroArch) loads that block from disk, saves it back, and names the
file after the game: `<game>.srm`. The frontend treats the block as opaque bytes and knows
nothing about N64 save types.

The N64 has four save media (EEPROM, SRAM, FlashRAM and Controller Paks), and a game's
type is only known after its ROM is loaded. A single buffer also has to cover the paks
plugged into each of four controllers. Mupen64Plus-Next therefore exposes one fixed-size
struct with room for the largest form of every type. Every game gets an `.srm` of the same
size, and the emulated cartridge and paks read and write their own part of it. The core's
own file hook, `save_storage_file_libretro()` in `main.c`, is empty: the frontend does all
the saving.

`libretro.h` lets a core that needs more than one buffer write its own files to the
save directory instead. Mupen64Plus-Next does not do that for these save types, so
converting to native files needs a tool like this one.

Sources:
- [`libretro/libretro_memory.h`](https://github.com/libretro/mupen64plus-libretro-nx/blob/develop/libretro/libretro_memory.h): struct layout
- [`mupen64plus-core/src/main/main.c`](https://github.com/libretro/mupen64plus-libretro-nx/blob/develop/mupen64plus-core/src/main/main.c): `open_{eep,mpk,sra,fla}_file` point each save type at its region
- [`libretro/libretro.c`](https://github.com/libretro/mupen64plus-libretro-nx/blob/develop/libretro/libretro.c): `format_saved_memory()` sets the initial contents

```c
#define EEPROM_MAX_SIZE 0x800
enum { MEMPAK_SIZE = 0x8000 };    // mempak.h
enum { SRAM_SIZE = 0x8000 };      // sram.h
enum { FLASHRAM_SIZE = 0x20000 }; // flashram.h

typedef struct _save_memory_data {
   uint8_t eeprom[EEPROM_MAX_SIZE];
   uint8_t mempack[MEMPAK_SIZE * 4];
   uint8_t sram[SRAM_SIZE];
   uint8_t flashram[FLASHRAM_SIZE];
} save_memory_data;
```

## Layout

The file has no header, magic number, version or checksum. Every field size is a
multiple of 4, so the struct has no padding.

| Region          | Offset    | Size (hex) | Size (bytes) | Native ext |
|-----------------|-----------|------------|--------------|------------|
| EEPROM          | `0x00000` | `0x00800`  | 2,048        | `.eep`     |
| Controller Pak 1| `0x00800` | `0x08000`  | 32,768       | `.mpk`     |
| Controller Pak 2| `0x08800` | `0x08000`  | 32,768       | `.mpk`     |
| Controller Pak 3| `0x10800` | `0x08000`  | 32,768       | `.mpk`     |
| Controller Pak 4| `0x18800` | `0x08000`  | 32,768       | `.mpk`     |
| SRAM            | `0x20800` | `0x08000`  | 32,768       | `.sra`     |
| FlashRAM        | `0x28800` | `0x20000`  | 131,072      | `.fla`     |
| **Total**       |           | `0x48800`  | **296,960**  |            |

A valid `.srm` is exactly 296,960 bytes. Reject any other size.

## Byte order

Each region is stored byte-for-byte, with no 16-bit or 32-bit swapping. This is the
same byte order that cartridge dumpers produce, so a native file matches its SRM
region exactly.

(Not verified: some emulators, notably Project64, are known to store `.sra`/`.fla`
with each 32-bit word swapped. Future work could add an optional byte swap flag later. It is
not part of this format.)

## Blank / initial contents

When no save exists, `format_saved_memory()` fills the regions as follows:

- **EEPROM, SRAM, FlashRAM**: filled with `0xFF`.
- **Controller Paks 1–4**: each is formatted by `format_mempak()` (ID blocks,
  index/inode table, empty note table), so a blank pak is not all `0xFF`. Most of a
  blank pak is `0xFF`, with about 376 non-zero formatting bytes. The source seeds a
  random serial, but in practice unused slots are byte-identical, both within a
  file and across files.

A game uses only one cartridge save type, so in practice exactly one cart region
holds data and the others stay `0xFF`. Paks can hold data alongside any cart type.

## Detecting what is in use

- **EEPROM / SRAM / FlashRAM**: the region is in use if any byte is not `0xFF`.
- **Controller Pak N**: read the note table at pak offset `0x300`–`0x4FF`
  (16 entries × 32 bytes). An entry is used if its 4-byte game code (bytes 0–3)
  is non-zero. A pak with no used entries is blank or formatted.
  Entry fields: `[0:4]` game code, `[4:6]` publisher code, `[0x10:0x20]` note name
  in N64 font encoding.

## EEPROM size (4 Kbit vs 16 Kbit)

The EEPROM region is always 2,048 bytes, but real carts are either:

- **4 Kbit** = 512 bytes (e.g. Bomberman 64)
- **16 Kbit** = 2,048 bytes (e.g. Banjo-Tooie)

A 4 Kbit game uses only the first 512 bytes, and the rest stays `0xFF`. The SRM
does not record the size, so splitting has to guess it:

- If bytes `0x200`–`0x7FF` are all `0xFF`, write 512 bytes. Otherwise write 2,048.
- Provide an override (`--eep-size 512|2048`). The guess fails for a 16 Kbit game
  that has not written past byte 512 yet.

## Native sizes

| Ext    | Type                    | Size                 | SRM region          |
|--------|-------------------------|----------------------|---------------------|
| `.eep` | EEPROM 4 Kbit / 16 Kbit | 512 / 2,048          | EEPROM, prefix      |
| `.sra` | SRAM 256 Kbit           | 32,768               | SRAM                |
| `.fla` | FlashRAM 1 Mbit         | 131,072              | FlashRAM            |
| `.mpk` | Controller Pak          | 32,768 (one pak)     | one pak slot        |

Unsupported: 768 Kbit (96 KB) SRAM carts, such as Dezaemon 3D, do not fit in the
32 KB SRAM region. Reject them when packing.

Some tools write a 4-pak `.mpk` of 131,072 bytes. Accept that size when packing
and map it to slots 1–4.

## Conversions

### Split (SRM → native)
1. Validate size == 296,960.
2. For each cart region in use, write `<name>.eep|.sra|.fla` (EEPROM trimmed as
   described above).
3. For each pak slot in use, write `<name>.mpk` for slot 1 and `<name>.<N>.mpk`
   for slots 2–4. Copy the full 32 KB slot.

### Pack (native → new SRM)
1. Start from a blank image (`0xFF` for the cart regions, plus 4 formatted blank paks).
2. Copy each native file to its region's offset:
   - `.eep` (512 or 2,048 B) goes to offset 0. Pad the rest of the 2 KB with `0xFF`.
   - `.sra` (32,768 B) goes to `0x20800`.
   - `.fla` (131,072 B) goes to `0x28800`.
   - `.mpk` (32,768 B) for pak N goes to `0x800 + (N-1)*0x8000`. A 131,072 B file
     goes to `0x800` and fills all four paks.
3. Reject native files whose size does not match.

### Insert (native → existing SRM)
The same as Pack, but step 1 starts from an existing `.srm`. Only the regions the
native files cover change, and every other byte is kept.

### Pak slot selection (Pack and Insert)
A 32 KB `.mpk` goes in the first slot that applies, by these rules in order:

1. **`--slot N`** (1–4), if given. Only allowed when exactly one `.mpk` file is given,
   and not with a 128 KB file.
2. **The file name**, if it ends `.N.mpk` with N from 1 to 4 (e.g. `Game.3.mpk`).
   Any other number (`Game.5.mpk`) is part of the name, not a slot.
3. **The first unused slot:** the lowest-numbered slot that has no notes (see
   *Detecting what is in use*) and that no other file in the same command claims.
4. **No unused slot:** fail without writing anything, and ask the user to pass `--slot N`.

Rules 1 and 2 may pick a slot that already holds data. Insert confirms before
replacing data in any region (see the README). All files claiming a slot through
rules 1 and 2 are resolved before any rule-3 file is placed, so a rule-3 file never
takes a slot that a later file names. Rule-3 files fill slots in argument order. Two
files that end up in the same slot are an error. A 128 KB `.mpk` always fills all
four slots.

For Pack the base image is blank, so rule 3 gives slots 1, 2, 3, 4 in order. Split
writes pak 1 without a number (`Game.mpk`), so re-packing a split works. Inserting
that `Game.mpk` back into an `.srm` whose pak 1 is in use puts it in the next free
slot instead. To replace pak 1, use `--slot 1` or name the file `Game.1.mpk`.

Blank pak: either port `format_mempak()` to Go, or embed a 32 KB blank-pak template
taken from an unused slot of a real `.srm`. This tool embeds a template.

### Inspect
Report the file size and validity, then for each region its offset, whether it is in
use, the non-`0xFF` byte count, and the guessed EEPROM size. For each pak, list its
used notes (game code, publisher, name).
