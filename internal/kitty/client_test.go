package kitty

import (
	"fmt"
	"net"
	"os"
	"testing"
)

func TestSocketAlive_LiveSocket(t *testing.T) {
	wsName := fmt.Sprintf("test-live-%d", os.Getpid())
	socket := SocketPath(wsName)
	t.Cleanup(func() { os.Remove(socket) })

	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("creating test socket: %v", err)
	}
	defer ln.Close()

	if !SocketAlive(wsName) {
		t.Error("SocketAlive should return true for a live socket")
	}
}

func TestSocketAlive_DeadSocket(t *testing.T) {
	wsName := fmt.Sprintf("test-dead-%d", os.Getpid())
	socket := SocketPath(wsName)
	t.Cleanup(func() { os.Remove(socket) })

	// Create socket file, then close the listener so nobody is listening
	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("creating test socket: %v", err)
	}
	ln.Close()

	if SocketAlive(wsName) {
		t.Error("SocketAlive should return false for a dead socket (no listener)")
	}
}

func TestSocketAlive_NoSocket(t *testing.T) {
	wsName := fmt.Sprintf("test-nosock-%d", os.Getpid())
	// Ensure no file exists
	os.Remove(SocketPath(wsName))

	if SocketAlive(wsName) {
		t.Error("SocketAlive should return false when no socket file exists")
	}
}

func TestIsAlive_SocketTakesPrecedence(t *testing.T) {
	wsName := fmt.Sprintf("test-sockprio-%d", os.Getpid())
	socket := SocketPath(wsName)
	t.Cleanup(func() { os.Remove(socket) })

	ln, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("creating test socket: %v", err)
	}
	defer ln.Close()

	// PID 99999999 is almost certainly dead, but socket is alive
	if !IsAlive(wsName, 99999999) {
		t.Error("IsAlive should return true when socket is alive (regardless of PID)")
	}
}

func TestIsAlive_FallbackToPID(t *testing.T) {
	wsName := fmt.Sprintf("test-pidfb-%d", os.Getpid())
	// No socket exists
	os.Remove(SocketPath(wsName))

	// Current process PID is alive
	if !IsAlive(wsName, os.Getpid()) {
		t.Error("IsAlive should return true when PID is alive (no socket)")
	}
}

func TestIsAlive_BothDead(t *testing.T) {
	wsName := fmt.Sprintf("test-bothdead-%d", os.Getpid())
	os.Remove(SocketPath(wsName))

	if IsAlive(wsName, 99999999) {
		t.Error("IsAlive should return false when both socket and PID are dead")
	}
}

func TestIsRunning_CurrentProcess(t *testing.T) {
	if !IsRunning(os.Getpid()) {
		t.Error("IsRunning should return true for current process")
	}
}

func TestIsRunning_DeadPID(t *testing.T) {
	if IsRunning(99999999) {
		t.Error("IsRunning should return false for non-existent PID")
	}
}

func TestIsRunning_ZeroPID(t *testing.T) {
	if IsRunning(0) {
		t.Error("IsRunning should return false for PID 0")
	}
}
