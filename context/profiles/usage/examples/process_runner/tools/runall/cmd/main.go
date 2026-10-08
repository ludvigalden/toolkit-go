// Command runall runs commands in order; see the example README.
package main

import (
	"os"

	"example.com/process_runner/tools/runall/internal/app"
)

func main() {
	os.Exit(app.Main(os.Args[1:]))
}
