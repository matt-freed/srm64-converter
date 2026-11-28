// Package srm reads and writes Mupen64Plus-Next .srm save containers.
// The layout is described in SRM_SPEC.md at the repository root.
package srm

import (
	_ "embed"
	"fmt"
)

// Region sizes and offsets, from libretro_memory.h (save_memory_data).
const (
	EEPSize = 0x800
	PakSize = 0x8000
	Paks    = 4
	SRASize = 0x8000
	FLASize = 0x20000

	EEPOffset = 0
	PakOffset = EEPOffset + EEPSize
	SRAOffset = PakOffset + PakSize*Paks
	FLAOffset = SRAOffset + SRASize

	// Size is the exact size of a valid .srm file (296,960 bytes).
	Size = FLAOffset + FLASize
)

// Kind is a native save type.
type Kind int

const (
	EEP Kind = iota
	MPK
	SRA
	FLA
)

// Ext returns the conventional file extension, without the dot.
func (k Kind) Ext() string {
	return [...]string{"eep", "mpk", "sra", "fla"}[k]
}

// Name returns a human-readable name.
func (k Kind) Name() string {
	return [...]string{"EEPROM", "Controller Pak", "SRAM", "FlashRAM"}[k]
}

// Region is one area of the container. Slot is 1-4 for paks and 0 otherwise.
type Region struct {
	Kind Kind
	Slot int
	Off  int
	Len  int
}

// Label is a short display name such as "EEPROM" or "Controller Pak 2".
func (r Region) Label() string {
	if r.Kind == MPK {
		return fmt.Sprintf("%s %d", r.Kind.Name(), r.Slot)
	}
	return r.Kind.Name()
}

// Regions lists every region in file order.
var Regions = []Region{
	{EEP, 0, EEPOffset, EEPSize},
	{MPK, 1, PakOffset + 0*PakSize, PakSize},
	{MPK, 2, PakOffset + 1*PakSize, PakSize},
	{MPK, 3, PakOffset + 2*PakSize, PakSize},
	{MPK, 4, PakOffset + 3*PakSize, PakSize},
	{SRA, 0, SRAOffset, SRASize},
	{FLA, 0, FLAOffset, FLASize},
}

// RegionFor returns the region for a kind and slot (slot is ignored unless kind is MPK).
func RegionFor(k Kind, slot int) (Region, error) {
	for _, r := range Regions {
		if r.Kind == k && (k != MPK || r.Slot == slot) {
			return r, nil
		}
	}
	return Region{}, fmt.Errorf("no %s region for slot %d", k.Name(), slot)
}

// blankPak is a formatted, empty Controller Pak as Mupen64Plus-Next writes it.
// It was copied from an unused slot of a real .srm; unused slots are
// byte-identical in practice, so it carries the same fixed serial.
//
//go:embed blankpak.mpk
var blankPak []byte

// Image is an in-memory .srm file.
type Image [Size]byte

// Parse validates b as an .srm file and returns a copy of it.
func Parse(b []byte) (*Image, error) {
	if len(b) != Size {
		return nil, fmt.Errorf("not a Mupen64Plus-Next .srm: size is %d bytes, want %d", len(b), Size)
	}
	img := new(Image)
	copy(img[:], b)
	return img, nil
}

// Blank returns the image the core creates when no save exists: 0xFF
// everywhere, with a formatted empty pak in each of the four slots.
func Blank() *Image {
	img := new(Image)
	for i := range img {
		img[i] = 0xFF
	}
	for _, r := range Regions {
		if r.Kind != MPK {
			continue
		}
		copy(img[r.Off:r.Off+r.Len], blankPak)
	}
	return img
}

// Bytes returns the slice of img covered by r. It aliases img.
func (img *Image) Bytes(r Region) []byte {
	return img[r.Off : r.Off+r.Len]
}

// IsUsed reports whether r holds save data. Cart regions are in use unless
// every byte is 0xFF or every byte is 0x00. A pak is in use if its note table
// has any entries.
func (img *Image) IsUsed(r Region) bool {
	return HasData(r.Kind, img.Bytes(r))
}

// HasData reports whether b, the contents of a region or a native file of
// kind k, holds save data. For cart types, a uniform fill of 0xFF (the
// current core's blank) or 0x00 (what some older cores wrote) counts as
// blank. For a 32 KB pak, data means any note-table entry.
func HasData(k Kind, b []byte) bool {
	if k == MPK {
		return len(Notes(b)) > 0
	}
	return !allBytes(b, 0xFF) && !allBytes(b, 0x00)
}

// allBytes reports whether every byte of b is c.
func allBytes(b []byte, c byte) bool {
	for _, x := range b {
		if x != c {
			return false
		}
	}
	return true
}

// CountNonFF returns the number of bytes in b that are not 0xFF.
func CountNonFF(b []byte) int {
	n := 0
	for _, c := range b {
		if c != 0xFF {
			n++
		}
	}
	return n
}
