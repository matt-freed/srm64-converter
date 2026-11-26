package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/matt-freed/srm64-converter/internal/srm"
	"github.com/spf13/cobra"
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
GAME.N.mpk name replaces whatever that slot holds.`

func packCmd() *cobra.Command {
	var (
		out   string
		slot  int
		force bool
	)
	cmd := &cobra.Command{
		Use:   "pack [-o OUT.srm] NATIVE...",
		Short: "Build a new .srm from native save files",
		Long: `Build a new .srm from native save files.

` + nativeHelp + `

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
			if err := placeFiles(img, a, slot, cmd.OutOrStdout()); err != nil {
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
	addSlotFlag(f.IntVar, &slot)
	f.BoolVarP(&force, "force", "f", false, "overwrite OUT.srm if it exists")
	return cmd
}

func addSlotFlag(intVar func(*int, string, int, string), p *int) {
	intVar(p, "slot", 0, "Controller Pak slot (1-4) for a single .mpk file")
}

type native struct {
	path string
	kind srm.Kind
	slot int
	data []byte
}

// placeFiles reads each native file, resolves pak slots (see nativeHelp), and
// places the files into img in argument order, printing one line per region
// to out. slotFlag is the --slot value, or 0 if not given.
func placeFiles(img *srm.Image, paths []string, slotFlag int, out io.Writer) error {
	var files []native
	var paks []int // indexes into files of single-slot .mpk files
	nMPK := 0
	for _, path := range paths {
		k, slot, err := srm.KindFromPath(path)
		if err != nil {
			return usageError{err}
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
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
			return usagef("--slot must be 1-%d, not %d", srm.Paks, slotFlag)
		case nMPK != 1:
			return usagef("--slot needs exactly one .mpk file, got %d", nMPK)
		case len(paks) != 1:
			return usagef("--slot can't be used with a 128 KB .mpk, which fills all four slots")
		}
		files[paks[0]].slot = slotFlag
	}

	requested := make([]int, len(paks))
	for i, fi := range paks {
		requested[i] = files[fi].slot
	}
	slots, bad, err := srm.AssignPakSlots(img, requested)
	if errors.Is(err, srm.ErrNoFreePak) {
		return usagef("%s: every Controller Pak slot already holds data; "+
			"use --slot N (1-%d) to choose one to replace", files[paks[bad]].path, srm.Paks)
	} else if err != nil {
		return usageError{err}
	}
	for i, fi := range paks {
		files[fi].slot = slots[i]
	}

	written := map[srm.Region]string{}
	for _, f := range files {
		var had []bool
		for _, r := range srm.Regions {
			had = append(had, img.IsUsed(r))
		}
		regions, err := img.Place(f.kind, f.slot, f.data)
		if err != nil {
			return fmt.Errorf("%s: %w", f.path, err)
		}
		for _, r := range regions {
			if prev, ok := written[r]; ok {
				return usagef("%s and %s both go in %s", prev, f.path, r.Label())
			}
			written[r] = f.path
			note := ""
			if had[regionIndex(r)] {
				note = " (replaced existing data)"
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
	return nil
}

func regionIndex(r srm.Region) int {
	for i, x := range srm.Regions {
		if x == r {
			return i
		}
	}
	return -1
}
