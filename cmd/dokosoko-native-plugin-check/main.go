package main

import (
	"fmt"
	"os"

	"github.com/dokosoko/dokosoko-service/nativeplugin/sourcecheck"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: dokosoko-native-plugin-check <source-directory> [...]")
		os.Exit(2)
	}
	failed := false
	for _, root := range os.Args[1:] {
		findings, err := sourcecheck.Check(root)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: check failed: %v\n", root, err)
			failed = true
			continue
		}
		for _, finding := range findings {
			fmt.Fprintln(os.Stderr, finding.String())
		}
		if len(findings) > 0 {
			failed = true
		} else {
			fmt.Printf("%s: native plugin source check passed\n", root)
		}
	}
	if failed {
		os.Exit(1)
	}
}
