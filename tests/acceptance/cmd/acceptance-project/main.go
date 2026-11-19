// Command acceptance-project evaluates a trusted project for the private acceptance harness.
package main

import (
	"context"
	"fmt"
	"github.com/magelift/magelift/internal/projectphp"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: acceptance-project PROJECT.php")
		os.Exit(2)
	}
	data, err := projectphp.Evaluate(context.Background(), os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err = os.Stdout.Write(data); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
