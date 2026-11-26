package srm

import (
	"bytes"
	"math/rand/v2"
	"testing"
)

// randBytes returns n deterministic pseudo-random bytes. The data is never
// all 0xFF, so a region holding it counts as used.
func randBytes(seed uint64, n int) []byte {
	r := rand.New(rand.NewPCG(seed, 0x5352_4D36_34))
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(r.Uint32())
	}
	return b
}

// pakWithNotes returns a formatted pak whose note table has n entries, with
// pseudo-random data in the note blocks (pages 5 onward).
func pakWithNotes(seed uint64, n int) []byte {
	pak := bytes.Clone(blankPak)
	copy(pak[0x500:], randBytes(seed, PakSize-0x500))
	for i := range n {
		e := pak[noteTableOff+i*noteSize:]
		copy(e[0:6], "NTST01")
		copy(e[0x10:0x20], randBytes(seed+uint64(i), 16))
	}
	return pak
}

func TestLayout(t *testing.T) {
	// Fixed values from libretro_memory.h; see SRM_SPEC.md.
	want := map[string][2]int{
		"EEPROM":           {0x00000, 0x00800},
		"Controller Pak 1": {0x00800, 0x08000},
		"Controller Pak 2": {0x08800, 0x08000},
		"Controller Pak 3": {0x10800, 0x08000},
		"Controller Pak 4": {0x18800, 0x08000},
		"SRAM":             {0x20800, 0x08000},
		"FlashRAM":         {0x28800, 0x20000},
	}
	if Size != 296960 {
		t.Fatalf("Size = %d, want 296960", Size)
	}
	end := 0
	for _, r := range Regions {
		if w := want[r.Label()]; r.Off != w[0] || r.Len != w[1] {
			t.Errorf("%s = %#x+%#x, want %#x+%#x", r.Label(), r.Off, r.Len, w[0], w[1])
		}
		if r.Off != end {
			t.Errorf("%s starts at %#x, want %#x (gap or overlap)", r.Label(), r.Off, end)
		}
		end += r.Len
	}
	if end != Size || len(Regions) != len(want) {
		t.Errorf("regions cover %#x in %d entries, want %#x in %d", end, len(Regions), Size, len(want))
	}
}

func TestBlank(t *testing.T) {
	img := Blank()
	for _, r := range Regions {
		if img.IsUsed(r) {
			t.Errorf("%s is used in a blank image", r.Label())
		}
		b := img.Bytes(r)
		if r.Kind == MPK {
			if !bytes.Equal(b, blankPak) {
				t.Errorf("%s is not the blank pak", r.Label())
			}
		} else if CountNonFF(b) != 0 {
			t.Errorf("%s is not all 0xFF", r.Label())
		}
	}
}

// split extracts every used region the way the split command does, keyed by
// the file name it would write.
func split(img *Image) map[string][]byte {
	files := map[string][]byte{}
	for _, r := range Regions {
		if img.IsUsed(r) {
			files[NativeName("game", r)] = img.Native(r, 0)
		}
	}
	return files
}

// pack builds an image from files the way the pack command does.
func pack(t *testing.T, files map[string][]byte) *Image {
	t.Helper()
	img := Blank()
	for name, data := range files {
		k, slot, err := KindFromPath(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := img.Place(k, slot, data); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	return img
}

// TestRoundTrip packs generated native files into an .srm, splits it, and
// packs the result again. Each cycle must reproduce its input exactly, and
// only the expected regions may be used.
func TestRoundTrip(t *testing.T) {
	eep16 := randBytes(2, EEP16KSize)
	eep16[EEP16KSize-1] = 0x00 // data in the last byte keeps the guess at 16 Kbit

	cases := []struct {
		name  string
		files map[string][]byte
	}{
		{"eeprom 4 Kbit", map[string][]byte{"game.eep": randBytes(1, EEP4KSize)}},
		{"eeprom 16 Kbit", map[string][]byte{"game.eep": eep16}},
		{"sram", map[string][]byte{"game.sra": randBytes(3, SRASize)}},
		{"flashram", map[string][]byte{"game.fla": randBytes(4, FLASize)}},
		{"pak 1", map[string][]byte{"game.mpk": pakWithNotes(5, 3)}},
		{"pak 4", map[string][]byte{"game.4.mpk": pakWithNotes(6, 16)}},
		{"cart + paks", map[string][]byte{
			"game.sra":   randBytes(7, SRASize),
			"game.mpk":   pakWithNotes(8, 1),
			"game.3.mpk": pakWithNotes(9, 2),
		}},
		{"every region", map[string][]byte{
			"game.eep":   randBytes(10, EEP4KSize),
			"game.sra":   randBytes(11, SRASize),
			"game.fla":   randBytes(12, FLASize),
			"game.mpk":   pakWithNotes(13, 1),
			"game.2.mpk": pakWithNotes(14, 1),
			"game.3.mpk": pakWithNotes(15, 1),
			"game.4.mpk": pakWithNotes(16, 1),
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			img := pack(t, c.files)
			got := split(img)
			if len(got) != len(c.files) {
				t.Errorf("split wrote %d files, want %d", len(got), len(c.files))
			}
			for name, want := range c.files {
				if !bytes.Equal(got[name], want) {
					t.Errorf("%s: split output differs from input (%d vs %d bytes)", name, len(got[name]), len(want))
				}
			}
			if again := pack(t, got); *again != *img {
				t.Error("re-packing the split files gave a different .srm")
			}
		})
	}
}

func TestPlaceIntoBase(t *testing.T) {
	// Packing onto an existing image changes only the regions supplied.
	base := pack(t, map[string][]byte{"game.eep": randBytes(20, EEP4KSize)})
	pak := pakWithNotes(21, 2)
	img := *base
	if _, err := img.Place(MPK, 2, pak); err != nil {
		t.Fatal(err)
	}
	for _, r := range Regions {
		changed := !bytes.Equal(img.Bytes(r), base.Bytes(r))
		if want := r.Kind == MPK && r.Slot == 2; changed != want {
			t.Errorf("%s changed = %v, want %v", r.Label(), changed, want)
		}
	}
}

func TestEEPSizeGuess(t *testing.T) {
	// A 16 Kbit save whose upper 1.5 KB is still 0xFF can't be told apart from
	// a 4 Kbit one: auto gives 512 B and an explicit size recovers all 2048.
	eep := bytes.Repeat([]byte{0xFF}, EEP16KSize)
	copy(eep, randBytes(30, EEP4KSize))
	img := pack(t, map[string][]byte{"game.eep": eep})
	r := Regions[0]
	if n := len(img.Native(r, 0)); n != EEP4KSize {
		t.Errorf("auto size = %d, want %d", n, EEP4KSize)
	}
	if !bytes.Equal(img.Native(r, EEP16KSize), eep) {
		t.Error("explicit 2048 did not return the full EEPROM")
	}
	eep[0x200] = 0x00
	if GuessEEPSize(eep) != EEP16KSize {
		t.Error("data at 0x200 should guess 16 Kbit")
	}
}

func TestNativeIsCopy(t *testing.T) {
	img := pack(t, map[string][]byte{"game.sra": randBytes(40, SRASize)})
	r, _ := RegionFor(SRA, 0)
	b := img.Native(r, 0)
	b[0] ^= 0xFF
	if img.Bytes(r)[0] == b[0] {
		t.Error("Native aliases the image")
	}
}

func TestNotes(t *testing.T) {
	notes := Notes(pakWithNotes(50, 3))
	if len(notes) != 3 {
		t.Fatalf("got %d notes, want 3", len(notes))
	}
	if string(notes[0].GameCode[:]) != "NTST" || string(notes[0].Publisher[:]) != "01" {
		t.Errorf("note 0 = %q/%q, want NTST/01", notes[0].GameCode, notes[0].Publisher)
	}
	if len(Notes(blankPak)) != 0 {
		t.Error("blank pak has notes")
	}
}

func TestParseBadSize(t *testing.T) {
	for _, n := range []int{0, Size - 1, Size + 1} {
		if _, err := Parse(make([]byte, n)); err == nil {
			t.Errorf("Parse accepted %d bytes", n)
		}
	}
}

func TestPlaceSizes(t *testing.T) {
	cases := []struct {
		k    Kind
		n    int
		okay bool
	}{
		{EEP, 512, true}, {EEP, 2048, true}, {EEP, 1024, false},
		{SRA, SRASize, true}, {SRA, 98304, false},
		{FLA, FLASize, true}, {FLA, FLASize - 1, false},
		{MPK, PakSize, true}, {MPK, MPK4Size, true}, {MPK, 36928, false},
	}
	for _, c := range cases {
		_, err := Blank().Place(c.k, 1, make([]byte, c.n))
		if (err == nil) != c.okay {
			t.Errorf("Place(%s, %d bytes): err = %v, want ok=%v", c.k.Name(), c.n, err, c.okay)
		}
	}
}

func TestPlaceEEPPads(t *testing.T) {
	img := new(Image) // all zero, so padding is visible
	if _, err := img.Place(EEP, 0, make([]byte, 512)); err != nil {
		t.Fatal(err)
	}
	eep := img.Bytes(Regions[0])
	if CountNonFF(eep[512:]) != 0 || CountNonFF(eep[:512]) != 512 {
		t.Error("EEPROM not padded with 0xFF after 512 bytes")
	}
}

func TestPlaceMPK4(t *testing.T) {
	data := make([]byte, MPK4Size)
	for i := range Paks {
		data[i*PakSize] = byte(i + 1)
	}
	img := Blank()
	rs, err := img.Place(MPK, 1, data)
	if err != nil || len(rs) != Paks {
		t.Fatalf("Place 128 KB mpk: %v, %d regions", err, len(rs))
	}
	for i := range Paks {
		r, _ := RegionFor(MPK, i+1)
		if img.Bytes(r)[0] != byte(i+1) {
			t.Errorf("slot %d has wrong data", i+1)
		}
	}
}

func TestKindFromPath(t *testing.T) {
	cases := []struct {
		path string
		k    Kind
		slot int
		bad  bool
	}{
		{"a/Game.eep", EEP, 0, false},
		{"Game.SRA", SRA, 0, false},
		{"Game.fla", FLA, 0, false},
		{"Game.mpk", MPK, 1, false},
		{"Game.3.mpk", MPK, 3, false},
		{"Game.5.mpk", MPK, 1, false},
		{"Mario 64.mpk", MPK, 1, false},
		{"Game.srm", 0, 0, true},
	}
	for _, c := range cases {
		k, slot, err := KindFromPath(c.path)
		if (err != nil) != c.bad || (!c.bad && (k != c.k || slot != c.slot)) {
			t.Errorf("KindFromPath(%q) = %v, %d, %v", c.path, k, slot, err)
		}
	}
}

func TestNativeNameInvertsKindFromPath(t *testing.T) {
	for _, r := range Regions {
		name := NativeName("Some Game (USA)", r)
		k, slot, err := KindFromPath(name)
		if err != nil {
			t.Fatal(err)
		}
		got, err := RegionFor(k, slot)
		if err != nil || got != r {
			t.Errorf("%s -> %q -> %v, want %s", r.Label(), name, got.Label(), r.Label())
		}
	}
}
