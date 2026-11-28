package srm

// TextLineSize is the width of the line FirstTextLine returns, as in a hex dump.
const TextLineSize = 16

// FirstTextLine finds the first 16-byte line of b, at a 16-byte-aligned offset,
// that holds readable text: a run of at least 4 printable ASCII characters of
// which at least 3 are letters. It returns the line's offset and the line with
// non-printable bytes shown as '.'.
//
// It is meant for checking SRAM/FlashRAM byte order by eye. Text in cartridge
// order reads normally, and in Mupen64Plus order it is reversed in 4-byte
// groups ("Mario Story" becomes "iraMtS o yro"). Lines are 16-byte aligned, so
// the swap only moves bytes within a line, and the same line is found in either
// order. Saves with no text can still match on random data, so a result isn't
// proof that the file holds text.
func FirstTextLine(b []byte) (offset int, line string, ok bool) {
	for off := 0; off < len(b); off += TextLineSize {
		l := b[off:min(off+TextLineSize, len(b))]
		if hasTextRun(l) {
			return off, printableLine(l), true
		}
	}
	return 0, "", false
}

func hasTextRun(l []byte) bool {
	run, letters := 0, 0
	for _, c := range l {
		if c < 0x20 || c > 0x7E {
			run, letters = 0, 0
			continue
		}
		run++
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			letters++
		}
		if run >= 4 && letters >= 3 {
			return true
		}
	}
	return false
}

func printableLine(l []byte) string {
	s := make([]byte, len(l))
	for i, c := range l {
		if c < 0x20 || c > 0x7E {
			c = '.'
		}
		s[i] = c
	}
	return string(s)
}
