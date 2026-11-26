package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/matt-freed/srm64-converter/internal/srm"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// stdinIsTerminal reports whether someone can answer a prompt. Tests replace
// it. (A ModeCharDevice check isn't enough: /dev/null is a character device.)
var stdinIsTerminal = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// confirmReplace warns that the regions in replaced already hold data and asks
// whether to go on. It returns nil to proceed. Without a terminal to ask, it
// refuses rather than guess; -f (checked by the caller) skips the prompt.
func confirmReplace(cmd *cobra.Command, replaced []srm.Region) error {
	if len(replaced) == 0 {
		return nil
	}
	labels := make([]string, len(replaced))
	for i, r := range replaced {
		labels[i] = r.Label()
	}
	// Show the mapping lines before the prompt; stdout is buffered.
	if f, ok := cmd.OutOrStdout().(interface{ Flush() error }); ok {
		f.Flush()
	}
	errw := cmd.ErrOrStderr()
	fmt.Fprintf(errw, "srm64: warning: these regions already hold data and will be replaced: %s\n",
		strings.Join(labels, ", "))
	if !stdinIsTerminal() {
		return errors.New("not replacing existing data without confirmation; use -f to replace it")
	}
	fmt.Fprint(errw, "Replace? [y/N] ")
	line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	}
	return errors.New("aborted; nothing was written")
}
