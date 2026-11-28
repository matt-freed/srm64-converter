package main

import (
	"fmt"
	"os"

	"github.com/matt-freed/srm64-converter/internal/srm"
	"github.com/spf13/cobra"
)

func swapCmd() *cobra.Command {
	var (
		mode  string
		out   string
		force bool
	)
	cmd := &cobra.Command{
		Use:   "swap NATIVE... [--mode full|byte|word] [-o OUT] [-f]",
		Short: "Change the byte order of native save files",
		Long: `Change the byte order of native save files (.eep, .sra, .fla, .mpk),
for files another tool wrote in a different order. Each 32-bit word is
transformed; shown for the bytes A B C D:

  full  A B C D -> D C B A   full 32-bit endian reversal (default)
  byte  A B C D -> B A D C   swap the bytes of each 16-bit half
  word  A B C D -> C D A B   swap the two 16-bit halves

full is the same swap split and pack apply to .sra/.fla, and equals byte
and word together. Every mode is its own inverse: running the same command
again restores the file.

Files are updated in place. With -o, the result for a single file goes to
OUT instead. .srm files are rejected: swapping a whole .srm would scramble
its EEPROM and pak data too.`,
		Args: args(cobra.MinimumNArgs(1)),
		RunE: func(cmd *cobra.Command, a []string) error {
			m, err := srm.ParseSwapMode(mode)
			if err != nil {
				return usagef("--mode: %v", err)
			}
			if out != "" && len(a) != 1 {
				return usagef("-o needs exactly one file, got %d", len(a))
			}

			// Read and check every file before writing any.
			data := make([][]byte, len(a))
			for i, path := range a {
				k, _, err := srm.KindFromPath(path)
				if err != nil {
					return usagef("%v (swap works on native files only)", err)
				}
				if data[i], err = os.ReadFile(path); err != nil {
					return err
				}
				if err := srm.CheckNativeSize(k, len(data[i])); err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
			}

			for i, path := range a {
				srm.Swap(data[i], m)
				dst, overwrite := path, true
				if out != "" {
					dst, overwrite = out, force
				}
				if err := writeFile(dst, data[i], overwrite); err != nil {
					return err
				}
				target := path
				if dst != path {
					target = path + " -> " + dst
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s: %s swap (%s)\n", target, m, m.Pattern())
			}
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&mode, "mode", "full", "swap to apply: full, byte or word")
	f.StringVarP(&out, "output", "o", "", "write a single file's result to OUT instead of updating it")
	f.BoolVarP(&force, "force", "f", false, "overwrite OUT if it exists")
	return cmd
}
