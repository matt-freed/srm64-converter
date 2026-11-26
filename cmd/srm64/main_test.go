package main

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	root := newRoot(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
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

func TestCLIErrors(t *testing.T) {
	dir := t.TempDir()
	eep := filepath.Join(dir, "input.eep") // split of g.srm writes g.eep beside it
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
		{"inspect not srm", []string{"inspect", eep}, false},
		{"inspect no args", []string{"inspect"}, true},
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
