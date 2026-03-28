package liveness

import (
	"os"
	"testing"
)

func TestAlive(t *testing.T) {
	exe, _ := os.Executable()
	if !AliveAndOurs(os.Getpid(), BootID(), exe) {
		t.Fatal("self not alive and ours")
	}
	if Alive(1<<30, "") || Alive(0, "") {
		t.Fatal("impossible pid alive")
	}
	if BootID() != "" && Alive(os.Getpid(), "other-boot") {
		t.Fatal("foreign boot accepted")
	}
	if n, err := Nonce(); err != nil || len(n) != 32 {
		t.Fatalf("Nonce = %q %v", n, err)
	}
}
