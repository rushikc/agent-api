package cli

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

func printTable(out io.Writer, header []string, rows [][]string) error {
	if out == nil {
		out = os.Stdout
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	for i, h := range header {
		if i > 0 {
			fmt.Fprint(w, "\t")
		}
		fmt.Fprint(w, h)
	}
	fmt.Fprintln(w)
	for _, row := range rows {
		for i, c := range row {
			if i > 0 {
				fmt.Fprint(w, "\t")
			}
			fmt.Fprint(w, c)
		}
		fmt.Fprintln(w)
	}
	return w.Flush()
}
