package spawn

import (
	"context"
	"testing"
	"time"
)

func TestCaptureAndExit(t *testing.T) {
	p, err := Start(context.Background(), Config{Path: "/bin/sh", Args: []string{"-c", "echo out; echo err >&2; exit 3"}}, "sh")
	if err != nil {
		t.Fatal(err)
	}
	o := p.Wait(nil)
	if o.Status != Exited || o.ExitCode != 3 || string(o.Stdout) != "out\n" || string(o.Stderr) != "err\n" {
		t.Fatalf("outcome = %+v", o)
	}
}

func TestDeadlineAndStreaming(t *testing.T) {
	var killed string
	p, err := Start(context.Background(), Config{
		Path: "/bin/sh", Args: []string{"-c", "echo hi; sleep 30"},
		Timeout: 200 * time.Millisecond, Streaming: true,
		OnKill: func(d string) { killed = d },
	}, "sh")
	if err != nil {
		t.Fatal(err)
	}
	if line := <-p.Lines; line != "hi" {
		t.Fatalf("line = %q", line)
	}
	if o := p.Wait(nil); o.Status != Deadline || o.ExitCode != -1 || killed != "timeout" {
		t.Fatalf("outcome = %+v killed=%q", o, killed)
	}
}
