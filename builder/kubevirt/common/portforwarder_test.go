// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	kvcorev1 "kubevirt.io/client-go/kubevirt/typed/core/v1"
)

// fakeResource hands out one end of an in-memory pipe for every tunnel and
// publishes the other end, which plays the role of the VM.
type fakeResource struct {
	mu    sync.Mutex
	err   error
	vmEnd chan net.Conn
}

func newFakeResource() *fakeResource {
	return &fakeResource{vmEnd: make(chan net.Conn, 10)}
}

func (f *fakeResource) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeResource) PortForward(name string, port int, protocol string) (kvcorev1.StreamInterface, error) {
	f.mu.Lock()
	err := f.err
	f.mu.Unlock()
	if err != nil {
		return nil, err
	}

	local, vm := net.Pipe()
	f.vmEnd <- vm
	return &fakeStream{conn: local}, nil
}

type fakeStream struct{ conn net.Conn }

func (s *fakeStream) Stream(kvcorev1.StreamOptions) error { return nil }
func (s *fakeStream) AsConn() net.Conn                    { return s.conn }

func startForwarder(t *testing.T, resource PortforwardableResource) (*PortForwarder, net.Addr) {
	t.Helper()
	forwarder := &PortForwarder{Kind: "vm", Namespace: "ns", Name: "vm", Resource: resource}
	addr, err := forwarder.StartForwarding(&net.IPAddr{IP: net.IPv4(127, 0, 0, 1)}, ForwardedPort{Local: 0, Remote: 22, Protocol: ProtocolTCP})
	if err != nil {
		t.Fatalf("StartForwarding: %v", err)
	}
	t.Cleanup(func() { _ = forwarder.Close() })
	return forwarder, addr
}

func dial(t *testing.T, addr net.Addr) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr.String(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	return conn
}

func receiveVMEnd(t *testing.T, resource *fakeResource) net.Conn {
	t.Helper()
	select {
	case vm := <-resource.vmEnd:
		t.Cleanup(func() { _ = vm.Close() })
		_ = vm.SetDeadline(time.Now().Add(5 * time.Second))
		return vm
	case <-time.After(5 * time.Second):
		t.Fatal("no tunnel was opened")
		return nil
	}
}

func TestStartForwardingAllocatesAPortAndForwardsTraffic(t *testing.T) {
	resource := newFakeResource()
	_, addr := startForwarder(t, resource)

	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok || tcpAddr.Port == 0 || !tcpAddr.IP.IsLoopback() {
		t.Fatalf("expected a loopback address with an allocated port, got %v", addr)
	}

	client := dial(t, addr)
	vm := receiveVMEnd(t, resource)

	if _, err := client.Write([]byte("ping")); err != nil {
		t.Fatalf("write to tunnel: %v", err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(vm, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("expected the VM to receive %q, got %q (%v)", "ping", buf, err)
	}

	if _, err := vm.Write([]byte("pong")); err != nil {
		t.Fatalf("write from VM: %v", err)
	}
	if _, err := io.ReadFull(client, buf); err != nil || string(buf) != "pong" {
		t.Fatalf("expected the client to receive %q, got %q (%v)", "pong", buf, err)
	}
}

func TestForwardingKeepsListeningAfterATunnelError(t *testing.T) {
	resource := newFakeResource()
	resource.setErr(errors.New("dialing VM: connection refused"))
	_, addr := startForwarder(t, resource)

	// The failed tunnel closes the client connection...
	failed := dial(t, addr)
	if _, err := failed.Read(make([]byte, 1)); err == nil {
		t.Fatal("expected the connection to be closed after the tunnel error")
	}

	// ...but later attempts, e.g. communicator retries, still go through.
	resource.setErr(nil)
	client := dial(t, addr)
	vm := receiveVMEnd(t, resource)
	if _, err := client.Write([]byte("x")); err != nil {
		t.Fatalf("write to tunnel: %v", err)
	}
	if _, err := io.ReadFull(vm, make([]byte, 1)); err != nil {
		t.Fatalf("expected the VM to receive data: %v", err)
	}
}

func TestLastErrorReportsTheMostRecentTunnel(t *testing.T) {
	resource := newFakeResource()
	tunnelErr := errors.New("Websocket failed with http status: 403 Forbidden")
	resource.setErr(tunnelErr)
	forwarder, addr := startForwarder(t, resource)

	if err := forwarder.LastError(); err != nil {
		t.Fatalf("expected no error before any tunnel, got %v", err)
	}

	failed := dial(t, addr)
	_, _ = failed.Read(make([]byte, 1))
	if err := forwarder.LastError(); !errors.Is(err, tunnelErr) {
		t.Fatalf("expected the tunnel error, got %v", err)
	}

	// A tunnel that opens successfully clears the previous error.
	resource.setErr(nil)
	_ = dial(t, addr)
	_ = receiveVMEnd(t, resource)
	deadline := time.Now().Add(5 * time.Second)
	for forwarder.LastError() != nil {
		if time.Now().After(deadline) {
			t.Fatalf("expected the error to be cleared, got %v", forwarder.LastError())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCloseStopsAcceptingConnections(t *testing.T) {
	forwarder, addr := startForwarder(t, newFakeResource())

	if err := forwarder.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := forwarder.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	if conn, err := net.DialTimeout("tcp", addr.String(), time.Second); err == nil {
		_ = conn.Close()
		t.Fatal("expected the listener to be closed")
	}
}

func TestCloseBeforeStartIsANoop(t *testing.T) {
	forwarder := &PortForwarder{}
	if err := forwarder.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}
