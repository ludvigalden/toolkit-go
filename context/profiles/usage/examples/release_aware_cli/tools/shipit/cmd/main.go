// Command shipit shows the smallest release-aware binary: it knows its
// version and changelog from an embedded relmap export, and composes
// toolkit-go's update interfaces by hand instead of using kongcmd.
package main

import (
	"os"

	"example.com/shipit/tools/shipit/internal/app"
)

func main() { os.Exit(app.Main(os.Args[1:])) }
