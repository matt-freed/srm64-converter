package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/matt-freed/srm64-converter/internal/srm"
	"github.com/spf13/cobra"
)

func inspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect FILE.srm",
		Short: "Show which save regions of an .srm hold data",
		Args:  args(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, a []string) error {
			img, err := loadSRM(a[0])
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s: valid .srm (%d bytes)\n\n", a[0], srm.Size)

			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "REGION\tOFFSET\tSIZE\tSTATUS\tNON-FF\tNOTES")
			for _, r := range srm.Regions {
				b := img.Bytes(r)
				status, extra := "blank", ""
				if img.IsUsed(r) {
					status = "used"
				}
				switch r.Kind {
				case srm.EEP:
					if status == "used" {
						if srm.GuessEEPSize(b) == srm.EEP4KSize {
							extra = "looks 4 Kbit (512 B)"
						} else {
							extra = "16 Kbit (2048 B)"
						}
					}
				case srm.MPK:
					if n := len(srm.Notes(b)); n > 0 {
						extra = fmt.Sprintf("%d note(s)", n)
					}
				}
				fmt.Fprintf(tw, "%s\t0x%05X\t%d\t%s\t%d\t%s\n",
					r.Label(), r.Off, r.Len, status, srm.CountNonFF(b), extra)
			}
			tw.Flush()

			for _, r := range srm.Regions {
				if r.Kind != srm.MPK {
					continue
				}
				notes := srm.Notes(img.Bytes(r))
				if len(notes) == 0 {
					continue
				}
				fmt.Fprintf(out, "\n%s notes:\n", r.Label())
				tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
				fmt.Fprintln(tw, "  #\tGAME\tPUBLISHER\tNAME (raw)")
				for _, n := range notes {
					fmt.Fprintf(tw, "  %d\t%s\t%s\t%X\n",
						n.Index, printable(n.GameCode[:]), printable(n.Publisher[:]), n.RawName)
				}
				tw.Flush()
			}
			return nil
		},
	}
}

// printable shows b as text if it is printable ASCII, otherwise as hex.
func printable(b []byte) string {
	for _, c := range b {
		if c < 0x20 || c > 0x7E {
			return fmt.Sprintf("%X", b)
		}
	}
	return string(b)
}

func loadSRM(path string) (*srm.Image, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	img, err := srm.Parse(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return img, nil
}
