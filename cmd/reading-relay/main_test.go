package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestListenUnixCreatesPrivateSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run", "relay.sock")
	listener, err := listenUnix(path)
	if err != nil {
		t.Fatalf("listenUnix: %v", err)
	}
	defer listener.Close()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("path is not a Unix socket: %s", info.Mode())
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("socket permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestListenUnixRefusesToReplaceRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.sock")
	if err := os.WriteFile(path, []byte("do not remove"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := listenUnix(path); err == nil {
		t.Fatal("expected regular-file socket path to be rejected")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "do not remove" {
		t.Fatalf("regular file was modified: %q", contents)
	}
}

func TestListenUnixRefusesActiveSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.sock")
	first, err := listenUnix(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := listenUnix(path); err == nil {
		t.Fatal("expected active socket to be rejected")
	}
	connection, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("original socket was disrupted: %v", err)
	}
	connection.Close()
}
