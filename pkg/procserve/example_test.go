package procserve_test

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/ludvigalden/toolkit-go/pkg/procserve"
)

// A program that is both client and warm server: started normally it
// connects (spawning itself on first use); started by procserve it
// serves until idle.
func Example() {
	if procserve.Spawned() {
		l, err := procserve.Listen(procserve.ServerOptions{IdleTimeout: time.Minute})
		if err != nil {
			os.Exit(3)
		}
		_ = procserve.Serve(context.Background(), l, procserve.HandlerFunc(
			func(_ context.Context, conn net.Conn) {
				line, _ := bufio.NewReader(conn).ReadString('\n')
				_, _ = fmt.Fprint(conn, "warm: "+line)
			}))
		os.Exit(0)
	}

	exe, _ := os.Executable()
	client := &procserve.Client{
		Dir: filepath.Join(os.TempDir(), "example-servers"),
		Command: func(identity string) *exec.Cmd {
			// The identity keys the server; pass it on however the
			// server needs it (flags, env, a config file).
			return exec.Command(exe, "--config", identity)
		},
		MaxIdle: 4,
	}
	conn, err := client.Connect(context.Background(), "build-v1/config-a")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer func() { _ = conn.Close() }()
	_, _ = fmt.Fprintln(conn, "hello")
	reply, _ := bufio.NewReader(conn).ReadString('\n')
	fmt.Print(reply)
}
