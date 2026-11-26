package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/matt-freed/srm64-converter/internal/srm"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// nativeHelp describes how native files are matched to regions; shared by
// pack and insert.
const nativeHelp = `Each file's type comes from its extension: .eep (512 or 2048 B), .sra
(32 KB), .fla (128 KB) or .mpk (32 KB). A 128 KB .mpk fills all four pak
slots. Otherwise each .mpk goes in the first slot that applies:

  1. --slot N, if given (only with a single .mpk file)
  2. N from a GAME.N.mpk file name (N = 1-4)
  3. the lowest-numbered slot with no notes that no other file claims

If no slot is free, the command fails and asks for --slot. --slot or a
GAME.N.mpk name may pick a slot that already holds data.`

func packCmd() *cobra.Command {
	var (
		out      string
		slot     int
		skipSwap bool
		force    bool
	)
	cmd := &cobra.Command{
		Use:   "pack [-o OUT.srm] [-f] NATIVE...",
		Short: "Build a new .srm from native save files",
		Long: `Build a new .srm from native save files.

` + nativeHelp + `

` + byteOrderHelp + `

Regions without a file are blank, as the emulator would create them. To
add files to an existing .srm, use insert.

Without -o, the output is named after the first file: GAME.eep or
GAME.2.mpk gives GAME.srm in the same directory.`,
		Args: args(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, a []string) error {
			if out == "" {
				out = srm.BaseName(a[0]) + ".srm"
			}
			img := srm.Blank()
			if _, err := placeFiles(img, a, slot, byteOrder(skipSwap), cmd.OutOrStdout()); err != nil {
				return err
			}
			if err := writeFile(out, img[:], force); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (%d bytes)\n", out, srm.Size)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVarP(&out, "output", "o", "", "the .srm file to write (default: first file's name with .srm)")
	addSlotFlag(f, &slot)
	addByteOrderFlag(f, &skipSwap)
	f.BoolVarP(&force, "force", "f", false, "overwrite OUT.srm if it exists")
	return cmd
}

func addSlotFlag(f *pflag.FlagSet, p *int) {
	f.IntVar(p, "slot", 0, "Controller Pak slot (1-4) for a single .mpk file")
}

// byteOrderHelp explains the SRAM/FlashRAM byte order; shared by split, pack
// and insert.
const byteOrderHelp = `.sra and .fla files are read and written in cartridge (hardware) order, the
order cart dumpers use. The .srm stores them with every 4-byte word
reversed, so they are swapped on the way in and out. --skip-byte-swap
copies them as stored, for .sra/.fla files already in Mupen64Plus order
(e.g. from standalone Mupen64Plus or a tool that doesn't swap). .eep and
.mpk files are never swapped.`

func addByteOrderFlag(f *pflag.FlagSet, p *bool) {
	f.BoolVar(p, "skip-byte-swap", false, "copy .sra/.fla data as stored in the .srm (Mupen64Plus order)")
}

func byteOrder(skipSwap bool) srm.ByteOrder {
	if skipSwap {
		return srm.Mupen
	}
	return srm.Hardware
}

type native struct {
	path string
	kind srm.Kind
	slot int
	data []byte
}

// placeFiles reads each native file, resolves pak slots (see nativeHelp), and
// places the files into img in argument order, printing one line per region
// to out. slotFlag is the --slot value, or 0 if not given, and order is the
// byte order of .sra/.fla files. It returns the
// regions that held data before and were overwritten. img is only modified in
// memory; on error the caller must not write it.
func placeFiles(img *srm.Image, paths []string, slotFlag int, order srm.ByteOrder, out io.Writer) ([]srm.Region, error) {
	var files []native
	var paks []int // indexes into files of single-slot .mpk files
	nMPK := 0
	for _, path := range paths {
		k, slot, err := srm.KindFromPath(path)
		if err != nil {
			return nil, usageError{err}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if k == srm.MPK {
			nMPK++
			if len(data) != srm.MPK4Size {
				paks = append(paks, len(files))
			}
		}
		files = append(files, native{path, k, slot, data})
	}

	if slotFlag != 0 {
		switch {
		case slotFlag < 1 || slotFlag > srm.Paks:
			return nil, usagef("--slot must be 1-%d, not %d", srm.Paks, slotFlag)
		case nMPK != 1:
			return nil, usagef("--slot needs exactly one .mpk file, got %d", nMPK)
		case len(paks) != 1:
			return nil, usagef("--slot can't be used with a 128 KB .mpk, which fills all four slots")
		}
		files[paks[0]].slot = slotFlag
	}

	requested := make([]int, len(paks))
	for i, fi := range paks {
		requested[i] = files[fi].slot
	}
	slots, bad, err := srm.AssignPakSlots(img, requested)
	if errors.Is(err, srm.ErrNoFreePak) {
		return nil, usagef("%s: every Controller Pak slot already holds data; "+
			"use --slot N (1-%d) to choose one to replace", files[paks[bad]].path, srm.Paks)
	} else if err != nil {
		return nil, usageError{err}
	}
	for i, fi := range paks {
		files[fi].slot = slots[i]
	}

	written := map[srm.Region]string{}
	var replaced []srm.Region
	for _, f := range files {
		var had []bool
		for _, r := range srm.Regions {
			had = append(had, img.IsUsed(r))
		}
		regions, err := img.Place(f.kind, f.slot, f.data, order)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.path, err)
		}
		for _, r := range regions {
			if prev, ok := written[r]; ok {
				return nil, usagef("%s and %s both go in %s", prev, f.path, r.Label())
			}
			written[r] = f.path
			note := ""
			if had[regionIndex(r)] {
				note = " (replaces existing data)"
				replaced = append(replaced, r)
			}
			fmt.Fprintf(out, "%s -> %s%s\n", f.path, r.Label(), note)
		}
	}

	carts := 0
	for _, r := range srm.Regions {
		if r.Kind != srm.MPK && img.IsUsed(r) {
			carts++
		}
	}
	if carts > 1 {
		fmt.Fprintln(os.Stderr, "srm64: warning: the .srm holds more than one cartridge save type; a game uses only one")
	}
	return replaced, nil
}

func regionIndex(r srm.Region) int {
	for i, x := range srm.Regions {
		if x == r {
			return i
		}
	}
	return -1
}
