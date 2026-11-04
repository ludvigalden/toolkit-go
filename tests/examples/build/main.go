// Command build compiles and vets every example module under context/:
// each is its own module, so it is checked from its own directory with
// the workspace off, exactly as a reader would build it.
// MODULE_ROOT is the toolkit-go checkout.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	root := os.Getenv("MODULE_ROOT")
	if root == "" {
		root = "."
	}
	var mods []string
	walkErr := filepath.WalkDir(filepath.Join(root, "context"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && d.Name() == "go.mod" {
			mods = append(mods, filepath.Dir(p))
		}
		return nil
	})
	if walkErr != nil || len(mods) == 0 {
		fmt.Fprintln(os.Stderr, "no example modules found:", walkErr)
		os.Exit(1)
	}
	failed := 0
	for _, m := range mods {
		for _, args := range [][]string{{"build", "./..."}, {"vet", "./..."}} {
			cmd := exec.Command("go", args...)
			cmd.Dir = m
			cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=-mod=mod")
			out, err := cmd.CombinedOutput()
			rel, _ := filepath.Rel(root, m)
			if err != nil {
				failed++
				fmt.Printf("FAIL go %s  %s\n%s", args[0], rel, out)
				continue
			}
			fmt.Printf("ok   go %s  %s\n", args[0], rel)
		}
	}
	if failed > 0 {
		os.Exit(1)
	}
}
