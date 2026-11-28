package main

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
)

// Tests never have a terminal unless runIn asks for one, so prompts don't
// depend on how go test was started.
func init() { stdinIsTerminal = func() bool { return false } }

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	out, _, err := runIn(t, "", false, args...)
	return out, err
}

// runIn runs the CLI with stdin set to input, as if from a terminal when tty
// is true. It returns stdout and stderr.
func runIn(t *testing.T, input string, tty bool, args ...string) (string, string, error) {
	t.Helper()
	old := stdinIsTerminal
	stdinIsTerminal = func() bool { return tty }
	defer func() { stdinIsTerminal = old }()
	var out, errb bytes.Buffer
	root := newRoot(&out)
	root.SetErr(&errb)
	root.SetIn(bytes.NewBufferString(input))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errb.String(), err
}

func randFile(t *testing.T, path string, seed uint64, n int) []byte {
	t.Helper()
	r := rand.New(rand.NewPCG(seed, 1))
	b := make([]byte, n)
	for i := range b {
		b[i] = byte(r.Uint32())
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return b
}

// TestCLIRoundTrip packs generated files with the CLI, splits the result into
// a new directory, and packs that again. Files and .srm must match exactly.
func TestCLIRoundTrip(t *testing.T) {
	in, outDir := t.TempDir(), t.TempDir()

	// A pak counts as used once its note table (at 0x300) has an entry.
	pak := randFile(t, filepath.Join(in, "Game.3.mpk"), 3, 0x8000)
	copy(pak[0x300:], "NTST01")
	if err := os.WriteFile(filepath.Join(in, "Game.3.mpk"), pak, 0o644); err != nil {
		t.Fatal(err)
	}
	want := map[string][]byte{
		"Game.eep":   randFile(t, filepath.Join(in, "Game.eep"), 1, 512),
		"Game.fla":   randFile(t, filepath.Join(in, "Game.fla"), 2, 0x20000),
		"Game.3.mpk": pak,
	}

	srm1 := filepath.Join(in, "Game.srm")
	if _, err := run(t, "pack", "-o", srm1,
		filepath.Join(in, "Game.eep"), filepath.Join(in, "Game.fla"), filepath.Join(in, "Game.3.mpk")); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "split", "-o", outDir, srm1); err != nil {
		t.Fatal(err)
	}

	entries, _ := os.ReadDir(outDir)
	if len(entries) != len(want) {
		t.Errorf("split wrote %d files, want %d", len(entries), len(want))
	}
	var natives []string
	for name, data := range want {
		p := filepath.Join(outDir, name)
		got, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("split did not write %s: %v", name, err)
			continue
		}
		if !bytes.Equal(got, data) {
			t.Errorf("%s differs from the packed input", name)
		}
		natives = append(natives, p)
	}

	srm2 := filepath.Join(outDir, "again.srm")
	if _, err := run(t, append([]string{"pack", "-o", srm2}, natives...)...); err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(srm1)
	c, _ := os.ReadFile(srm2)
	if !bytes.Equal(a, c) {
		t.Error("re-packed .srm differs from the original")
	}

	out, err := run(t, "inspect", srm2)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(out), []byte("looks 4 Kbit")) || !bytes.Contains([]byte(out), []byte("NTST")) {
		t.Errorf("inspect output missing EEPROM size or pak note:\n%s", out)
	}
}

// TestPackInfersOutput checks that pack without -o names the .srm after the
// first file, dropping a pak slot suffix, and writes it beside that file.
func TestPackInfersOutput(t *testing.T) {
	dir := t.TempDir()
	pak := filepath.Join(dir, "My Game.2.mpk")
	randFile(t, pak, 4, 0x8000)
	eep := filepath.Join(dir, "My Game.eep")
	randFile(t, eep, 5, 512)

	if _, err := run(t, "pack", pak, eep); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(filepath.Join(dir, "My Game.srm")); err != nil || fi.Size() != 296960 {
		t.Fatalf("inferred output not written correctly: %v", err)
	}
	// The inferred name obeys the overwrite guard too.
	if _, err := run(t, "pack", eep); err == nil {
		t.Error("pack overwrote the inferred .srm without -f")
	}
}

// TestInsert checks that insert changes only the regions it is given, updates
// the .srm in place by default, and leaves it untouched with -o.
func TestInsert(t *testing.T) {
	dir := t.TempDir()
	eep := filepath.Join(dir, "a.eep")
	randFile(t, eep, 6, 512)
	srmPath := filepath.Join(dir, "g.srm")
	if _, err := run(t, "pack", "-o", srmPath, eep); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(srmPath)

	pak := randFile(t, filepath.Join(dir, "b.2.mpk"), 7, 0x8000)
	copy(pak[0x300:], "NTST01")
	os.WriteFile(filepath.Join(dir, "b.2.mpk"), pak, 0o644)
	eep2 := randFile(t, filepath.Join(dir, "c.eep"), 8, 512)

	// -o writes a new file and leaves the input alone.
	other := filepath.Join(dir, "other.srm")
	if _, err := run(t, "insert", srmPath, filepath.Join(dir, "b.2.mpk"), "-o", other); err != nil {
		t.Fatal(err)
	}
	if now, _ := os.ReadFile(srmPath); !bytes.Equal(now, before) {
		t.Error("insert -o modified the input .srm")
	}
	if _, err := run(t, "insert", srmPath, filepath.Join(dir, "b.2.mpk"), "-o", other); err == nil {
		t.Error("insert -o overwrote an existing file without -f")
	}

	// In place: pak 2 and EEPROM change, every other byte stays.
	out, err := run(t, "insert", "-f", srmPath, filepath.Join(dir, "b.2.mpk"), filepath.Join(dir, "c.eep"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(out), []byte("EEPROM (replaces existing data)")) {
		t.Errorf("insert did not note the replaced EEPROM:\n%s", out)
	}
	if bytes.Contains([]byte(out), []byte("Controller Pak 2 (replaces")) {
		t.Errorf("insert called a blank pak replaced:\n%s", out)
	}
	after, _ := os.ReadFile(srmPath)
	want := bytes.Clone(before)
	copy(want[0:512], eep2)
	copy(want[0x8800:0x10800], pak)
	if !bytes.Equal(after, want) {
		t.Error("in-place insert changed bytes outside the inserted regions, or missed some")
	}
	if b, _ := os.ReadFile(other); !bytes.Equal(b[0x8800:0x10800], pak) {
		t.Error("insert -o output is missing the pak")
	}
}

// TestInsertPakSlots walks the slot rules in order: --slot, then a .N.mpk
// name, then the first unused slot, then an error asking for --slot.
func TestInsertPakSlots(t *testing.T) {
	dir := t.TempDir()
	mkPak := func(name string, seed uint64) string {
		p := filepath.Join(dir, name)
		b := randFile(t, p, seed, 0x8000)
		copy(b[0x300:], "NTST01")
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	slotOf := func(srmPath, pak string) int {
		t.Helper()
		img, _ := os.ReadFile(srmPath)
		want, _ := os.ReadFile(pak)
		for s := 1; s <= 4; s++ {
			off := 0x800 + (s-1)*0x8000
			if bytes.Equal(img[off:off+0x8000], want) {
				return s
			}
		}
		return 0
	}

	srmPath := filepath.Join(dir, "g.srm")
	p1 := mkPak("first.mpk", 10)
	if _, err := run(t, "pack", "-o", srmPath, p1); err != nil {
		t.Fatal(err)
	}
	if s := slotOf(srmPath, p1); s != 1 {
		t.Fatalf("pack put an unnumbered pak in slot %d, want 1", s)
	}

	// No slot given: first unused slot, skipping used pak 1.
	p2 := mkPak("second.mpk", 11)
	if _, err := run(t, "insert", srmPath, p2); err != nil {
		t.Fatal(err)
	}
	if s := slotOf(srmPath, p2); s != 2 {
		t.Errorf("auto slot = %d, want 2", s)
	}

	// .N.mpk name wins over auto.
	p4 := mkPak("third.4.mpk", 12)
	if _, err := run(t, "insert", srmPath, p4); err != nil {
		t.Fatal(err)
	}
	if s := slotOf(srmPath, p4); s != 4 {
		t.Errorf(".4.mpk slot = %d, want 4", s)
	}

	// --slot wins over the name, and may replace a used slot.
	p3 := mkPak("fourth.4.mpk", 13)
	out, err := run(t, "insert", "-f", "--slot", "1", srmPath, p3)
	if err != nil {
		t.Fatal(err)
	}
	if s := slotOf(srmPath, p3); s != 1 {
		t.Errorf("--slot 1 put pak in slot %d", s)
	}
	if !bytes.Contains([]byte(out), []byte("Controller Pak 1 (replaces existing data)")) {
		t.Errorf("--slot over a used slot not reported as replaced:\n%s", out)
	}

	// Fill slot 3, then an unnumbered pak has nowhere to go.
	if _, err := run(t, "insert", srmPath, mkPak("fifth.mpk", 14)); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(srmPath)
	_, err = run(t, "insert", srmPath, mkPak("sixth.mpk", 15))
	var ue usageError
	if !errors.As(err, &ue) || !bytes.Contains([]byte(err.Error()), []byte("--slot")) {
		t.Errorf("full .srm: err = %v, want a usage error mentioning --slot", err)
	}
	if after, _ := os.ReadFile(srmPath); !bytes.Equal(before, after) {
		t.Error("failed insert modified the .srm")
	}

	// --slot misuse.
	randFile(t, filepath.Join(dir, "a.eep"), 16, 512)
	for _, a := range [][]string{
		{"insert", "--slot", "5", srmPath, p2},
		{"insert", "--slot", "2", srmPath, p2, p3},
		{"insert", "--slot", "2", srmPath, filepath.Join(dir, "a.eep")},
	} {
		if _, err := run(t, a...); !errors.As(err, &ue) {
			t.Errorf("%v: err = %v, want a usage error", a[1:3], err)
		}
	}
}

// TestInsertConfirm checks the prompt before replacing data: yes proceeds,
// anything else aborts, no terminal refuses, and -f skips the question. A
// fresh region never prompts.
func TestInsertConfirm(t *testing.T) {
	dir := t.TempDir()
	eep1 := filepath.Join(dir, "one.eep")
	randFile(t, eep1, 20, 512)
	eep2 := filepath.Join(dir, "two.eep")
	want := randFile(t, eep2, 21, 512)
	pak := filepath.Join(dir, "p.mpk")
	b := randFile(t, pak, 22, 0x8000)
	copy(b[0x300:], "NTST01")
	os.WriteFile(pak, b, 0o644)

	srmPath := filepath.Join(dir, "g.srm")
	reset := func() []byte {
		t.Helper()
		if _, err := run(t, "pack", "-f", "-o", srmPath, eep1, pak); err != nil {
			t.Fatal(err)
		}
		before, _ := os.ReadFile(srmPath)
		return before
	}
	unchanged := func(name string, before []byte) {
		t.Helper()
		if now, _ := os.ReadFile(srmPath); !bytes.Equal(now, before) {
			t.Errorf("%s: .srm was modified", name)
		}
	}
	replaced := func(name string) {
		t.Helper()
		if now, _ := os.ReadFile(srmPath); !bytes.Equal(now[:512], want) {
			t.Errorf("%s: EEPROM was not replaced", name)
		}
	}

	for _, c := range []struct {
		name, input string
		tty, ok     bool
	}{
		{"yes", "y\n", true, true},
		{"YES", "YES\n", true, true},
		{"no", "n\n", true, false},
		{"empty answer", "\n", true, false},
		{"EOF", "", true, false},
		{"no terminal", "y\n", false, false},
	} {
		before := reset()
		_, stderr, err := runIn(t, c.input, c.tty, "insert", srmPath, eep2)
		if !bytes.Contains([]byte(stderr), []byte("already hold data and will be replaced: EEPROM")) {
			t.Errorf("%s: no warning naming EEPROM:\n%s", c.name, stderr)
		}
		if c.ok {
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
			}
			replaced(c.name)
		} else {
			if err == nil {
				t.Errorf("%s: insert went ahead", c.name)
			}
			unchanged(c.name, before)
		}
		if c.tty != bytes.Contains([]byte(stderr), []byte("Replace? [y/N]")) {
			t.Errorf("%s: prompt shown = %v, want %v", c.name, !c.tty, c.tty)
		}
	}

	// A pak slot counts too: --slot 1 over the used pak 1 asks.
	before := reset()
	_, stderr, err := runIn(t, "n\n", true, "insert", "--slot", "1", srmPath, pak)
	if err == nil || !bytes.Contains([]byte(stderr), []byte("Controller Pak 1")) {
		t.Errorf("pak replace: err = %v, stderr:\n%s", err, stderr)
	}
	unchanged("pak replace declined", before)

	// -f never asks.
	reset()
	_, stderr, err = runIn(t, "", true, "insert", "-f", srmPath, eep2)
	if err != nil || bytes.Contains([]byte(stderr), []byte("Replace?")) {
		t.Errorf("-f: err = %v, stderr:\n%s", err, stderr)
	}
	replaced("-f")

	// Nothing to replace: no warning, no prompt.
	reset()
	pak3 := filepath.Join(dir, "p.3.mpk")
	os.WriteFile(pak3, b, 0o644)
	_, stderr, err = runIn(t, "", true, "insert", srmPath, pak3)
	if err != nil || bytes.Contains([]byte(stderr), []byte("warning")) {
		t.Errorf("fresh slot: err = %v, stderr:\n%s", err, stderr)
	}
}

func TestInspectNative(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	pak := make([]byte, 0x8000) // empty note entries are zero, as on a formatted pak
	copy(pak[0x300:], "NTST01")

	cases := []struct {
		name string
		path string
		want []string // substrings of the output
	}{
		{"eep 4K", filepath.Join(dir, "a.eep"), []string{"EEPROM file (512 bytes)", "EEPROM  512", "used", "4 Kbit"}},
		{"eep 16K", filepath.Join(dir, "b.eep"), []string{"EEPROM file (2048 bytes)", "16 Kbit"}},
		{"blank sra", write("c.sra", bytes.Repeat([]byte{0xFF}, 0x8000)), []string{"SRAM file (32768 bytes)", "blank", "  0  "}},
		{"fla", filepath.Join(dir, "d.fla"), []string{"FlashRAM file (131072 bytes)", "used"}},
		{"pak", write("e.mpk", pak), []string{"Controller Pak  32768", "1 note(s)", "Controller Pak notes:", "NTST"}},
		{"4 paks", write("f.mpk", bytes.Repeat(pak, 4)), []string{"Controller Pak 4  32768", "Controller Pak 4 notes:"}},
	}
	randFile(t, filepath.Join(dir, "a.eep"), 30, 512)
	randFile(t, filepath.Join(dir, "b.eep"), 31, 2048)
	randFile(t, filepath.Join(dir, "d.fla"), 32, 0x20000)
	for _, c := range cases {
		out, err := run(t, "inspect", c.path)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		for _, w := range c.want {
			if !bytes.Contains([]byte(out), []byte(w)) {
				t.Errorf("%s: output missing %q:\n%s", c.name, w, out)
			}
		}
	}

	// A wrong size is a runtime error, not a usage error.
	_, err := run(t, "inspect", write("bad.eep", make([]byte, 1000)))
	var ue usageError
	if err == nil || errors.As(err, &ue) || !bytes.Contains([]byte(err.Error()), []byte("want 512 or 2048")) {
		t.Errorf("bad size: err = %v", err)
	}
}

// TestCLIByteSwap checks the bytes on disk: .sra/.fla are word-swapped into
// and out of the .srm by default, and copied as stored with --skip-byte-swap,
// for pack, insert and split. .eep is never swapped.
func TestCLIByteSwap(t *testing.T) {
	dir := t.TempDir()
	sra := filepath.Join(dir, "g.sra")
	data := randFile(t, sra, 40, 0x8000)
	swapped := bytes.Clone(data)
	for i := 0; i < len(swapped); i += 4 {
		swapped[i], swapped[i+1], swapped[i+2], swapped[i+3] = swapped[i+3], swapped[i+2], swapped[i+1], swapped[i]
	}
	eep := filepath.Join(dir, "g.eep")
	eepData := randFile(t, eep, 41, 512)
	const sraOff = 0x20800
	region := func(p string) []byte {
		t.Helper()
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return b[sraOff : sraOff+0x8000]
	}

	hw, mu := filepath.Join(dir, "hw.srm"), filepath.Join(dir, "mu.srm")
	if _, err := run(t, "pack", "-o", hw, sra, eep); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "pack", "--skip-byte-swap", "-o", mu, sra); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(region(hw), swapped) {
		t.Error("pack did not word-swap the .sra")
	}
	if !bytes.Equal(region(mu), data) {
		t.Error("pack --skip-byte-swap changed the .sra")
	}
	if b, _ := os.ReadFile(hw); !bytes.Equal(b[:512], eepData) {
		t.Error("pack swapped the .eep")
	}

	// insert follows the same rule.
	in := filepath.Join(dir, "in.srm")
	if _, err := run(t, "pack", "-o", in, eep); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "insert", in, sra); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(region(in), swapped) {
		t.Error("insert did not word-swap the .sra")
	}

	// split reverses it, so the default round trip gives back the input.
	for _, c := range []struct {
		name string
		args []string
		want []byte
	}{
		{"default", nil, data},
		{"skip", []string{"--skip-byte-swap"}, swapped},
	} {
		out := filepath.Join(dir, "split-"+c.name)
		os.Mkdir(out, 0o755)
		if _, err := run(t, append(append([]string{"split", "-o", out}, c.args...), hw)...); err != nil {
			t.Fatal(err)
		}
		got, _ := os.ReadFile(filepath.Join(out, "hw.sra"))
		if !bytes.Equal(got, c.want) {
			t.Errorf("split %s: wrong .sra bytes", c.name)
		}
		if e, _ := os.ReadFile(filepath.Join(out, "hw.eep")); !bytes.Equal(e, eepData) {
			t.Errorf("split %s: .eep changed", c.name)
		}
	}
}

// TestInspectText checks the readable-text line: shown for .sra/.fla/.eep in the
// file's own order, with the byte-order hint only for .sra/.fla, and never for .mpk.
func TestInspectText(t *testing.T) {
	dir := t.TempDir()
	fla := bytes.Repeat([]byte{0xFF}, 0x20000)
	copy(fla, "Mario Story 006\x00")
	flaPath := filepath.Join(dir, "g.fla")
	os.WriteFile(flaPath, fla, 0o644)
	sraPath := filepath.Join(dir, "blank.sra")
	os.WriteFile(sraPath, bytes.Repeat([]byte{0xFF}, 0x8000), 0o644)
	eepPath := filepath.Join(dir, "g.eep")
	os.WriteFile(eepPath, append([]byte("ABCDEFGH"), bytes.Repeat([]byte{0xFF}, 504)...), 0o644)

	pak := make([]byte, 0x8000)
	copy(pak, "ABCDEFGH")
	pakPath := filepath.Join(dir, "g.mpk")
	os.WriteFile(pakPath, pak, 0o644)

	for _, c := range []struct {
		path, want string
		absent     bool
	}{
		{flaPath, "First found text at 0x00000: Mario Story 006.", false},
		{flaPath, "try byte swapping", false},
		{sraPath, "First found text: none", false},
		{eepPath, "First found text at 0x00000: ABCDEFGH", false},
		{eepPath, "try byte swapping", true},
		{pakPath, "First found text", true},
	} {
		out, err := run(t, "inspect", c.path)
		if err != nil {
			t.Fatal(err)
		}
		if got := bytes.Contains([]byte(out), []byte(c.want)); got == c.absent {
			t.Errorf("%s: contains %q = %v, want %v:\n%s", filepath.Base(c.path), c.want, got, !c.absent, out)
		}
	}
}

func TestCLIErrors(t *testing.T) {
	dir := t.TempDir()
	eep := filepath.Join(dir, "input.eep") // split of g.srm writes g.eep beside it
	junk := filepath.Join(dir, "junk.bin")
	randFile(t, junk, 9, 100)
	randFile(t, eep, 3, 512)
	srmPath := filepath.Join(dir, "g.srm")
	if _, err := run(t, "pack", "-o", srmPath, eep); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		args  []string
		usage bool
	}{
		{"pack overwrite", []string{"pack", "-o", srmPath, eep}, false},
		{"split overwrite", []string{"split", srmPath}, false},
		{"pack duplicate", []string{"pack", "-o", filepath.Join(dir, "x.srm"), eep, eep}, true},
		{"pack unknown ext", []string{"pack", "-o", filepath.Join(dir, "x.srm"), srmPath}, true},
		{"bad eep size", []string{"split", "--eep-size", "1k", srmPath}, true},
		{"inspect not srm", []string{"inspect", junk}, false},
		{"inspect no args", []string{"inspect"}, true},
		{"pack --base removed", []string{"pack", "--base", srmPath, "-o", filepath.Join(dir, "x.srm"), eep}, true},
		{"insert no natives", []string{"insert", srmPath}, true},
		{"insert into non-srm", []string{"insert", eep, eep}, false},
	}
	// Split once so the overwrite case has something to collide with.
	if _, err := run(t, "split", srmPath); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		_, err := run(t, c.args...)
		if err == nil {
			t.Errorf("%s: no error", c.name)
			continue
		}
		var ue usageError
		if errors.As(err, &ue) != c.usage {
			t.Errorf("%s: usage error = %v, want %v (%v)", c.name, !c.usage, c.usage, err)
		}
	}
	if _, err := run(t, "pack", "-f", "-o", srmPath, eep); err != nil {
		t.Errorf("pack -f: %v", err)
	}
}
