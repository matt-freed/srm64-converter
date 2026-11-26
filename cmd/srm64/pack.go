package main

import (
	"fmt"
	"os"

	"github.com/matt-freed/srm64-converter/internal/srm"
	"github.com/spf13/cobra"
)

func packCmd() *cobra.Command {
	var (
		out   string
		base  string
		force bool
	)
	cmd := &cobra.Command{
		Use:   "pack [-o OUT.srm] NATIVE...",
		Short: "Build an .srm from native save files",
		Long: `Build an .srm from native save files.

Each file's type comes from its extension: .eep (512 or 2048 B), .sra
(32 KB), .fla (128 KB) or .mpk (32 KB). A pak goes in slot 1 unless named
GAME.N.mpk (N = 1-4); a 128 KB .mpk fills all four slots.

Regions without a file are blank, as the emulator would create them. With
--base, the files are merged into an existing .srm instead.

Without -o, the output is named after the first file: GAME.eep or
GAME.2.mpk gives GAME.srm in the same directory.`,
		Args: args(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, a []string) error {
			if out == "" {
				out = srm.BaseName(a[0]) + ".srm"
			}
			img := srm.Blank()
			if base != "" {
				var err error
				if img, err = loadSRM(base); err != nil {
					return err
				}
			}

			written := map[srm.Region]string{}
			carts := map[srm.Kind]bool{}
			for _, path := range a {
				k, slot, err := srm.KindFromPath(path)
				if err != nil {
					return usageError{err}
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				regions, err := img.Place(k, slot, data)
				if err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				for _, r := range regions {
					if prev, ok := written[r]; ok {
						return usagef("%s and %s both go in %s", prev, path, r.Label())
					}
					written[r] = path
					fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s\n", path, r.Label())
				}
				if k != srm.MPK {
					carts[k] = true
				}
			}
			if len(carts) > 1 {
				fmt.Fprintln(os.Stderr, "srm64: warning: more than one cartridge save type given; a game uses only one")
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
	f.StringVar(&base, "base", "", "existing .srm to merge the files into")
	f.BoolVarP(&force, "force", "f", false, "overwrite OUT.srm if it exists")
	return cmd
}
