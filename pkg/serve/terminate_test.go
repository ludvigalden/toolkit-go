package serve

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestTerminatePIDCleanExit(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skip("cannot start a child process")
	}
	waited := make(chan *os.ProcessState, 1)
	go func() { st, _ := cmd.Process.Wait(); waited <- st }()
	if err := TerminatePID(context.Background(), cmd.Process.Pid, 2*time.Second); err != nil {
		t.Fatalf("TerminatePID = %v", err)
	}
	select {
	case state := <-waited:
		if state.Success() {
			t.Fatalf("child exited cleanly (%v), want signal death", state)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("child did not exit")
	}
}

func TestTerminatePIDDeadProcessIsSuccess(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Skip("cannot start a child process")
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := TerminatePID(context.Background(), cmd.Process.Pid, 0); err != nil {
		t.Fatalf("TerminatePID(dead) = %v", err)
	}
}

func startIgnoringTerm(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sh", "-c", `trap "" TERM; while :; do sleep 1; done`)
	if err := cmd.Start(); err != nil {
		t.Skip("cannot start a child process")
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	time.Sleep(100 * time.Millisecond) // let the shell install its trap
	return cmd
}

func TestTerminatePIDEscalatesToKill(t *testing.T) {
	cmd := startIgnoringTerm(t)
	go func() { _, _ = cmd.Process.Wait() }()
	if err := TerminatePID(context.Background(), cmd.Process.Pid, 100*time.Millisecond); err != nil {
		t.Fatalf("TerminatePID = %v", err)
	}
}

func TestTerminatePIDContextCancel(t *testing.T) {
	cmd := startIgnoringTerm(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := TerminatePID(ctx, cmd.Process.Pid, time.Minute); err == nil {
		t.Fatal("TerminatePID with canceled ctx returned nil")
	}
}
