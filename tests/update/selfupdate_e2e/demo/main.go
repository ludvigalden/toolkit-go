// Command demo is the binary selfupdate_e2e updates in place: `demo
// version` prints the -ldflags version, `demo self-update <base-url>`
// applies the newest release the server offers.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/ludvigalden/toolkit-go/pkg/update/selfupdate"
)

var version = "0.0.0"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: demo version | demo self-update <base-url> [--no-verify]")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println(version)
	case "self-update":
		if len(os.Args) < 3 {
			os.Exit(2)
		}
		up, err := selfupdate.NewHTTP(os.Args[2], selfupdate.Config{Repo: "example/demo"})
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		rel, updated, err := up.Apply(context.Background(), version)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("latest=%s updated=%t\n", rel.Version, updated)
	default:
		os.Exit(2)
	}
}
