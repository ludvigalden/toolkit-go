// Command census counts a repository's files; see the example README.
package main

import (
	"os"

	"example.com/repo_walker/tools/census/internal/app"
)

func main() {
	os.Exit(app.Main(os.Args[1:]))
}
