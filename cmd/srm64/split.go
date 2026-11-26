package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/matt-freed/srm64-converter/internal/srm"
	"github.com/spf13/cobra"
)

func splitCmd() *cobra.Command {
	var (
		outDir  string
		eepSize string
		all     bool
		force   bool
	)
	cmd := &cobra.Command{
		Use:   "split FILE.srm",
		Short: "Extract native save files (.eep/.sra/.fla/.mpk) from an .srm",
		Long: `Extract native save files from an .srm.

Only regions that hold data are written, unless --all is given. Files are
named after the .srm: GAME.eep, GAME.sra, GAME.fla, GAME.mpk for pak 1 and
GAME.N.mpk for paks 2-4.

The .srm does not record EEPROM size. By default a save whose data stops
within the first 512 bytes is written as 4 Kbit (512 B), otherwise 16 Kbit
(2048 B). Use --eep-size to override.`,
		Args: args(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, a []string) error {
			var eepN int
			switch eepSize {
			case "auto":
			case "512":
				eepN = srm.EEP4KSize
			case "2048":
				eepN = srm.EEP16KSize
			default:
				return usagef("--eep-size must be auto, 512 or 2048, not %q", eepSize)
			}

			img, err := loadSRM(a[0])
			if err != nil {
				return err
			}
			dir := outDir
			if dir == "" {
				dir = filepath.Dir(a[0])
			}
			base := strings.TrimSuffix(filepath.Base(a[0]), filepath.Ext(a[0]))

			wrote := 0
			for _, r := range srm.Regions {
				if !all && !img.IsUsed(r) {
					continue
				}
				data := img.Native(r, eepN)
				path := filepath.Join(dir, srm.NativeName(base, r))
				if err := writeFile(path, data, force); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s -> %s (%d bytes)\n", r.Label(), path, len(data))
				wrote++
			}
			if wrote == 0 {
				return errors.New("no save data found in " + a[0] + " (use --all to write blank regions)")
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVarP(&outDir, "output-dir", "o", "", "directory to write to (default: next to the .srm)")
	f.StringVar(&eepSize, "eep-size", "auto", "EEPROM file size: auto, 512 or 2048")
	f.BoolVar(&all, "all", false, "also write regions that hold no data")
	f.BoolVarP(&force, "force", "f", false, "overwrite existing files")
	return cmd
}
