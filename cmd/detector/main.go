package main

import (
	"fmt"
	"os"

	"github.com/jmhobbs/odol/internal/detector"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: %s <p3d-file>\n", os.Args[0])
		os.Exit(2)
	}

	format, err := detector.DetectFile(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Println(format.String())
}
