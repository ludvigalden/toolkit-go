// Command notes keeps one-line notes; see the example README.
package main

import (
	"os"

	"example.com/minimal_cli/tools/notes/internal/app"
)

func main() {
	os.Exit(app.Main(os.Args[1:]))
}
