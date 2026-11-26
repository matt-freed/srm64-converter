package main

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/matt-freed/srm64-converter/internal/srm"
	"github.com/spf13/cobra"
)

func inspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect FILE",
		Short: "Show what an .srm or native save file holds",
		Long: `Show what an .srm or native save file holds.

For an .srm, list every region with its offset, size and whether it holds
data. For a native file (.eep, .sra, .fla or .mpk, recognised by
extension), show the same details for that one save. Paks also list their
notes.`,
		Args: args(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, a []string) error {
			if k, _, err := srm.KindFromPath(a[0]); err == nil {
				return inspectNative(cmd.OutOrStdout(), a[0], k)
			}
			return inspectSRM(cmd.OutOrStdout(), a[0])
		},
	}
}

func inspectSRM(out io.Writer, path string) error {
	img, err := loadSRM(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%s: valid .srm (%d bytes)\n\n", path, srm.Size)

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "REGION\tOFFSET\tSIZE\tSTATUS\tNON-FF\tNOTES")
	for _, r := range srm.Regions {
		b := img.Bytes(r)
		status, notes := describe(r.Kind, b)
		if r.Kind == srm.EEP && status == "used" {
			// The region is always 2 KB; the real size is a guess.
			if srm.GuessEEPSize(b) == srm.EEP4KSize {
				notes = "looks 4 Kbit (512 B)"
			} else {
				notes = "16 Kbit (2048 B)"
			}
		}
		fmt.Fprintf(tw, "%s\t0x%05X\t%d\t%s\t%d\t%s\n",
			r.Label(), r.Off, r.Len, status, srm.CountNonFF(b), notes)
	}
	tw.Flush()

	for _, r := range srm.Regions {
		if r.Kind == srm.MPK {
			printNotes(out, r.Label(), img.Bytes(r))
		}
	}
	return nil
}

func inspectNative(out io.Writer, path string, k srm.Kind) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := srm.CheckNativeSize(k, len(b)); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	fmt.Fprintf(out, "%s: %s file (%d bytes)\n\n", path, k.Name(), len(b))

	// A 128 KB .mpk is four paks back to back; show one row each.
	type part struct {
		label string
		b     []byte
	}
	parts := []part{{k.Name(), b}}
	if k == srm.MPK && len(b) == srm.MPK4Size {
		parts = nil
		for i := range srm.Paks {
			parts = append(parts, part{fmt.Sprintf("%s %d", k.Name(), i+1), b[i*srm.PakSize : (i+1)*srm.PakSize]})
		}
	}

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TYPE\tSIZE\tSTATUS\tNON-FF\tNOTES")
	for _, p := range parts {
		status, notes := describe(k, p.b)
		if k == srm.EEP {
			// A native file's size is the cart's real size.
			notes = fmt.Sprintf("%d Kbit", len(p.b)*8/1024)
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%d\t%s\n", p.label, len(p.b), status, srm.CountNonFF(p.b), notes)
	}
	tw.Flush()

	for _, p := range parts {
		if k == srm.MPK {
			printNotes(out, p.label, p.b)
		}
	}
	return nil
}

// describe returns the STATUS and NOTES columns for save data b of kind k.
func describe(k srm.Kind, b []byte) (status, notes string) {
	status = "blank"
	if srm.HasData(k, b) {
		status = "used"
	}
	if k == srm.MPK {
		if n := len(srm.Notes(b)); n > 0 {
			notes = fmt.Sprintf("%d note(s)", n)
		}
	}
	return status, notes
}

// printNotes lists the notes of the 32 KB pak in b, if it has any.
func printNotes(out io.Writer, label string, b []byte) {
	notes := srm.Notes(b)
	if len(notes) == 0 {
		return
	}
	fmt.Fprintf(out, "\n%s notes:\n", label)
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  #\tGAME\tPUBLISHER\tNAME (raw)")
	for _, n := range notes {
		fmt.Fprintf(tw, "  %d\t%s\t%s\t%X\n",
			n.Index, printable(n.GameCode[:]), printable(n.Publisher[:]), n.RawName)
	}
	tw.Flush()
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
