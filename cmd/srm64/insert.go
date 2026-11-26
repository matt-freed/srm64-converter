package main

import (
	"fmt"

	"github.com/matt-freed/srm64-converter/internal/srm"
	"github.com/spf13/cobra"
)

func insertCmd() *cobra.Command {
	var (
		out      string
		slot     int
		skipSwap bool
		force    bool
	)
	cmd := &cobra.Command{
		Use:   "insert FILE.srm NATIVE... [--slot N] [-o OUT.srm] [-f]",
		Short: "Add native save files to an existing .srm",
		Long: `Add native save files to an existing .srm, replacing the regions they
cover and leaving every other region as it was.

` + nativeHelp + `

` + byteOrderHelp + `

If any region (cart save or pak slot) already holds data, insert lists
them and asks before replacing them; without a terminal to ask, it stops.
-f replaces without asking.

FILE.srm is updated in place (written atomically). With -o, the result goes
to OUT.srm instead and FILE.srm is left untouched.`,
		Args: args(cobra.MinimumNArgs(2)),
		RunE: func(cmd *cobra.Command, a []string) error {
			img, err := loadSRM(a[0])
			if err != nil {
				return err
			}
			replaced, err := placeFiles(img, a[1:], slot, byteOrder(skipSwap), cmd.OutOrStdout())
			if err != nil {
				return err
			}
			if !force {
				if err := confirmReplace(cmd, replaced); err != nil {
					return err
				}
			}
			dst, overwrite := a[0], true
			if out != "" {
				dst, overwrite = out, force
			}
			if err := writeFile(dst, img[:], overwrite); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s (%d bytes)\n", dst, srm.Size)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVarP(&out, "output", "o", "", "write to OUT.srm instead of updating FILE.srm")
	addSlotFlag(f, &slot)
	addByteOrderFlag(f, &skipSwap)
	f.BoolVarP(&force, "force", "f", false, "replace existing data, and overwrite OUT.srm, without asking")
	return cmd
}
