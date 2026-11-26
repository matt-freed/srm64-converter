// Command srm64 converts between Mupen64Plus-Next .srm save files and native
// N64 save files (.eep, .sra, .fla, .mpk).
package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// stdout is buffered so small writes don't each hit the pipe; flushed on exit.
var stdout = bufio.NewWriter(os.Stdout)

// usageError marks a bad command line (exit code 2).
type usageError struct{ error }

func usagef(format string, a ...any) error {
	return usageError{fmt.Errorf(format, a...)}
}

// args wraps a cobra positional-args check so failures are usage errors.
func args(fn cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, a []string) error {
		if err := fn(cmd, a); err != nil {
			return usageError{err}
		}
		return nil
	}
}

func newRoot(out io.Writer) *cobra.Command {
	root := &cobra.Command{
		Use:           "srm64",
		Short:         "Convert Mupen64Plus-Next .srm saves to and from native N64 save files",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(out)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err} })
	root.AddCommand(inspectCmd(), splitCmd(), packCmd(), insertCmd())
	return root
}

func main() {
	err := newRoot(stdout).Execute()
	stdout.Flush()
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "srm64:", err)
	var ue usageError
	if errors.As(err, &ue) {
		fmt.Fprintln(os.Stderr, "Run 'srm64 --help' for usage.")
		os.Exit(2)
	}
	os.Exit(1)
}

// writeFile writes b to path atomically. Unless force is set it refuses to
// replace an existing file.
func writeFile(path string, b []byte, force bool) error {
	if !force {
		if _, err := os.Lstat(path); err == nil {
			return fmt.Errorf("%s already exists (use -f to overwrite)", path)
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".srm64-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
