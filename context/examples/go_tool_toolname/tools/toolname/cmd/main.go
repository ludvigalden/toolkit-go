// Command toolname is the binary entry point: it only hands os.Args to
// the app wiring and exits with the code the app returns.
package main

import (
	"os"

	"example.com/toolname/tools/toolname/internal/app"
)

// version is stamped at release time:
// go build -ldflags "-X main.version=0.2.0" ./tools/toolname/cmd
var version = ""

func main() {
	os.Exit(app.Main(os.Args[1:], version))
}
