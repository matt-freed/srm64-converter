package srm

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// Native file sizes.
const (
	EEP4KSize  = 512  // 4 Kbit EEPROM
	EEP16KSize = 2048 // 16 Kbit EEPROM
	MPK4Size   = PakSize * Paks
)

// ByteOrder is the byte order of SRAM and FlashRAM native files. EEPROM and
// Controller Pak files have a single order and are never swapped.
type ByteOrder int

const (
	// Hardware is the order the cartridge chip holds, which cart dumpers
	// read and write. It is the default.
	Hardware ByteOrder = iota
	// Mupen is the order stored in the .srm: every 32-bit word reversed,
	// because the core keeps SRAM and FlashRAM as host-order (little-endian)
	// words. Files in this order load only in Mupen64Plus-based emulators.
	Mupen
)

// Swapped reports whether native files of kind k in order o differ from the
// .srm bytes, which is true for SRAM and FlashRAM in Hardware order.
func (o ByteOrder) Swapped(k Kind) bool {
	return o == Hardware && (k == SRA || k == FLA)
}

// SwapMode is a byte-order transform applied to each 32-bit word, shown for
// the bytes A B C D. Every mode is its own inverse. Native file sizes are all
// multiples of 4.
type SwapMode int

const (
	// SwapFull is a full 32-bit endian reversal: A B C D -> D C B A. It is
	// the swap between cartridge order and the .srm (see ByteOrder).
	SwapFull SwapMode = iota
	// SwapBytes swaps the bytes of each 16-bit half: A B C D -> B A D C.
	SwapBytes
	// SwapWords swaps the two 16-bit halves: A B C D -> C D A B.
	SwapWords
)

// String returns the mode's name as used on the command line.
func (m SwapMode) String() string {
	return [...]string{"full", "byte", "word"}[m]
}

// Pattern shows what the mode does to the bytes A B C D.
func (m SwapMode) Pattern() string {
	return [...]string{"ABCD -> DCBA", "ABCD -> BADC", "ABCD -> CDAB"}[m]
}

// ParseSwapMode parses "full", "byte" or "word".
func ParseSwapMode(s string) (SwapMode, error) {
	for m := SwapFull; m <= SwapWords; m++ {
		if s == m.String() {
			return m, nil
		}
	}
	return 0, fmt.Errorf("unknown swap mode %q (want full, byte or word)", s)
}

// Swap applies m to b in place, one 32-bit word at a time. Trailing bytes
// that don't fill a word are left alone.
func Swap(b []byte, m SwapMode) {
	for i := 0; i+3 < len(b); i += 4 {
		w := b[i : i+4 : i+4]
		switch m {
		case SwapFull:
			w[0], w[1], w[2], w[3] = w[3], w[2], w[1], w[0]
		case SwapBytes:
			w[0], w[1], w[2], w[3] = w[1], w[0], w[3], w[2]
		case SwapWords:
			w[0], w[1], w[2], w[3] = w[2], w[3], w[0], w[1]
		}
	}
}

// Native returns a copy of r's contents as a native save file in byte order
// o. For EEPROM, eepSize picks the file size (512 or 2048); 0 guesses with
// GuessEEPSize.
func (img *Image) Native(r Region, eepSize int, o ByteOrder) []byte {
	b := img.Bytes(r)
	if r.Kind == EEP {
		if eepSize == 0 {
			eepSize = GuessEEPSize(b)
		}
		b = b[:eepSize]
	}
	b = bytes.Clone(b)
	if o.Swapped(r.Kind) {
		Swap(b, SwapFull)
	}
	return b
}

// NativeName returns the file name split uses for r: base.eep, base.sra,
// base.fla, base.mpk for pak 1 and base.N.mpk for paks 2-4. KindFromPath
// maps each name back to r.
func NativeName(base string, r Region) string {
	if r.Kind == MPK && r.Slot > 1 {
		return fmt.Sprintf("%s.%d.%s", base, r.Slot, r.Kind.Ext())
	}
	return base + "." + r.Kind.Ext()
}

// BaseName returns path without its extension, and without a pak slot suffix
// ("dir/Game.3.mpk" gives "dir/Game"). It inverts NativeName.
func BaseName(path string) string {
	base := strings.TrimSuffix(path, filepath.Ext(path))
	if k, slot, err := KindFromPath(path); err == nil && k == MPK && slot != 0 {
		if s := fmt.Sprintf(".%d", slot); strings.HasSuffix(base, s) {
			base = strings.TrimSuffix(base, s)
		}
	}
	return base
}

// ErrNoFreePak is returned by AssignPakSlots when a pak has no slot given and
// every slot is already in use or claimed.
var ErrNoFreePak = errors.New("no unused Controller Pak slot")

// AssignPakSlots resolves the slot for each pak to be placed into img.
// requested[i] is the slot asked for (1-4), or 0 if none was given. Requested
// slots are kept as they are, even if that slot holds data (it is replaced).
// Each 0 gets the lowest-numbered slot that is unused in img (IsUsed is false)
// and not requested or already assigned to another pak, in order. If none is
// left, it returns ErrNoFreePak with the index of the pak that didn't fit.
func AssignPakSlots(img *Image, requested []int) ([]int, int, error) {
	slots := make([]int, len(requested))
	taken := map[int]bool{}
	for i, s := range requested {
		if s < 0 || s > Paks {
			return nil, i, fmt.Errorf("pak slot %d out of range (want 1-%d)", s, Paks)
		}
		slots[i] = s
		if s != 0 {
			taken[s] = true
		}
	}
	for i, s := range slots {
		if s != 0 {
			continue
		}
		for c := 1; c <= Paks && slots[i] == 0; c++ {
			r, _ := RegionFor(MPK, c)
			if !taken[c] && !img.IsUsed(r) {
				slots[i] = c
				taken[c] = true
			}
		}
		if slots[i] == 0 {
			return nil, i, ErrNoFreePak
		}
	}
	return slots, -1, nil
}

// GuessEEPSize returns 512 if everything past the first 512 bytes of the
// EEPROM region is 0xFF (a 4 Kbit cart), otherwise 2048. The .srm does not
// record the real size, so a 16 Kbit save that has not yet written past byte
// 512 is guessed as 4 Kbit.
func GuessEEPSize(eep []byte) int {
	if CountNonFF(eep[EEP4KSize:]) == 0 {
		return EEP4KSize
	}
	return EEP16KSize
}

// KindFromPath identifies a native file by its extension. A pak may carry a
// slot suffix, as in "game.2.mpk" (slots 1-4). For a pak without one the
// returned slot is 0, meaning "not specified" (see AssignPakSlots). For cart
// types the slot is always 0.
func KindFromPath(path string) (Kind, int, error) {
	base := filepath.Base(path)
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".eep":
		return EEP, 0, nil
	case ".sra":
		return SRA, 0, nil
	case ".fla":
		return FLA, 0, nil
	case ".mpk":
		stem := strings.TrimSuffix(base, filepath.Ext(base))
		if s := filepath.Ext(stem); len(s) == 2 {
			if n, err := strconv.Atoi(s[1:]); err == nil && n >= 1 && n <= Paks {
				return MPK, n, nil
			}
		}
		return MPK, 0, nil
	}
	return 0, 0, fmt.Errorf("%s: unknown save type %q (want .eep, .sra, .fla or .mpk)", path, ext)
}

// Place copies native data, in byte order o, into img. EEPROM data may be 512
// or 2048 bytes; the rest of the region is filled with 0xFF. A 128 KB .mpk
// fills all four pak slots and slot is ignored. data is not modified. It
// returns the regions written.
func (img *Image) Place(k Kind, slot int, data []byte, o ByteOrder) ([]Region, error) {
	if err := CheckNativeSize(k, len(data)); err != nil {
		return nil, err
	}
	if o.Swapped(k) {
		data = bytes.Clone(data)
		Swap(data, SwapFull)
	}
	if k == MPK && len(data) == MPK4Size {
		var rs []Region
		for i := range Paks {
			r, _ := RegionFor(MPK, i+1)
			copy(img.Bytes(r), data[i*PakSize:(i+1)*PakSize])
			rs = append(rs, r)
		}
		return rs, nil
	}
	r, err := RegionFor(k, slot)
	if err != nil {
		return nil, err
	}
	dst := img.Bytes(r)
	pad := dst[copy(dst, data):]
	for i := range pad {
		pad[i] = 0xFF
	}
	return []Region{r}, nil
}

// CheckNativeSize reports whether n bytes is a valid size for a native file of
// kind k: 512 or 2048 for EEPROM, 32 KB for SRAM, 128 KB for FlashRAM, and
// 32 KB (one pak) or 128 KB (four paks) for a Controller Pak.
func CheckNativeSize(k Kind, n int) error {
	switch k {
	case EEP:
		if n != EEP4KSize && n != EEP16KSize {
			return sizeErr(k, n, EEP4KSize, EEP16KSize)
		}
	case SRA:
		if n != SRASize {
			return sizeErr(k, n, SRASize)
		}
	case FLA:
		if n != FLASize {
			return sizeErr(k, n, FLASize)
		}
	case MPK:
		if n != PakSize && n != MPK4Size {
			return sizeErr(k, n, PakSize, MPK4Size)
		}
	}
	return nil
}

func sizeErr(k Kind, got int, want ...int) error {
	ws := make([]string, len(want))
	for i, w := range want {
		ws[i] = strconv.Itoa(w)
	}
	return fmt.Errorf("%s file is %d bytes, want %s", k.Name(), got, strings.Join(ws, " or "))
}
