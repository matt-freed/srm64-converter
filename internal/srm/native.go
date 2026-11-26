package srm

import (
	"bytes"
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

// Native returns a copy of r's contents as a native save file. For EEPROM,
// eepSize picks the file size (512 or 2048); 0 guesses with GuessEEPSize.
func (img *Image) Native(r Region, eepSize int) []byte {
	b := img.Bytes(r)
	if r.Kind == EEP {
		if eepSize == 0 {
			eepSize = GuessEEPSize(b)
		}
		b = b[:eepSize]
	}
	return bytes.Clone(b)
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
	if k, slot, err := KindFromPath(path); err == nil && k == MPK {
		if s := fmt.Sprintf(".%d", slot); strings.HasSuffix(base, s) {
			base = strings.TrimSuffix(base, s)
		}
	}
	return base
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
// slot suffix, as in "game.2.mpk" (slots 1-4); without one the slot is 1.
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
		return MPK, 1, nil
	}
	return 0, 0, fmt.Errorf("%s: unknown save type %q (want .eep, .sra, .fla or .mpk)", path, ext)
}

// Place copies native data into img. EEPROM data may be 512 or 2048 bytes;
// the rest of the region is filled with 0xFF. A 128 KB .mpk fills all four
// pak slots and slot is ignored. It returns the regions written.
func (img *Image) Place(k Kind, slot int, data []byte) ([]Region, error) {
	switch k {
	case EEP:
		if len(data) != EEP4KSize && len(data) != EEP16KSize {
			return nil, sizeErr(k, len(data), EEP4KSize, EEP16KSize)
		}
	case SRA:
		if len(data) != SRASize {
			return nil, sizeErr(k, len(data), SRASize)
		}
	case FLA:
		if len(data) != FLASize {
			return nil, sizeErr(k, len(data), FLASize)
		}
	case MPK:
		if len(data) == MPK4Size {
			var rs []Region
			for i := range Paks {
				r, _ := RegionFor(MPK, i+1)
				copy(img.Bytes(r), data[i*PakSize:(i+1)*PakSize])
				rs = append(rs, r)
			}
			return rs, nil
		}
		if len(data) != PakSize {
			return nil, sizeErr(k, len(data), PakSize, MPK4Size)
		}
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

func sizeErr(k Kind, got int, want ...int) error {
	ws := make([]string, len(want))
	for i, w := range want {
		ws[i] = strconv.Itoa(w)
	}
	return fmt.Errorf("%s file is %d bytes, want %s", k.Name(), got, strings.Join(ws, " or "))
}
