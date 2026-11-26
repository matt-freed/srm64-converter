package main

import (
	"fmt"

	"github.com/matt-freed/srm64-converter/internal/srm"
	"github.com/spf13/cobra"
)

func insertCmd() *cobra.Command {
	var (
		out   string
		slot  int
		force bool
	)
	cmd := &cobra.Command{
		Use:   "insert FILE.srm NATIVE... [--slot N] [-o OUT.srm]",
		Short: "Add native save files to an existing .srm",
		Long: `Add native save files to an existing .srm, replacing the regions they
cover and leaving every other region as it was.

` + nativeHelp + `

FILE.srm is updated in place (written atomically). With -o, the result goes
to OUT.srm instead and FILE.srm is left untouched.`,
		Args: args(cobra.MinimumNArgs(2)),
		RunE: func(cmd *cobra.Command, a []string) error {
			img, err := loadSRM(a[0])
			if err != nil {
				return err
			}
			if err := placeFiles(img, a[1:], slot, cmd.OutOrStdout()); err != nil {
				return err
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
	addSlotFlag(f.IntVar, &slot)
	f.BoolVarP(&force, "force", "f", false, "overwrite OUT.srm if it exists")
	return cmd
}
