// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	"kubevirt.io/client-go/kubecli"
	kvcorev1 "kubevirt.io/client-go/kubevirt/typed/core/v1"
)

// serveVNC performs the server side of an RFB 3.8 handshake without
// authentication, then reports the pressed keys and when the client
// disconnects.
func serveVNC(conn net.Conn) (<-chan uint32, <-chan struct{}) {
	keys := make(chan uint32, 64)
	closed := make(chan struct{})

	go func() {
		defer close(closed)
		defer close(keys)

		b := make([]byte, 12)
		steps := []func() error{
			func() error { _, err := conn.Write([]byte("RFB 003.008\n")); return err },
			func() error { _, err := io.ReadFull(conn, b[:12]); return err },
			func() error { _, err := conn.Write([]byte{1, 1}); return err },
			func() error { _, err := io.ReadFull(conn, b[:1]); return err },
			func() error { return binary.Write(conn, binary.BigEndian, uint32(0)) },
			func() error { _, err := io.ReadFull(conn, b[:1]); return err },
			func() error { return binary.Write(conn, binary.BigEndian, []uint16{800, 600}) },
			func() error { _, err := conn.Write(make([]byte, 16)); return err },
			func() error { return binary.Write(conn, binary.BigEndian, uint32(0)) },
		}
		for _, step := range steps {
			if err := step(); err != nil {
				return
			}
		}

		for {
			msg := make([]byte, 8)
			if _, err := io.ReadFull(conn, msg); err != nil {
				return
			}
			if msg[0] == 4 && msg[1] == 1 {
				keys <- binary.BigEndian.Uint32(msg[4:])
			}
		}
	}()
	return keys, closed
}

type pipeStream struct{ conn net.Conn }

func (p *pipeStream) Stream(kvcorev1.StreamOptions) error { return nil }
func (p *pipeStream) AsConn() net.Conn                    { return p.conn }

func newBootCommandState() *multistep.BasicStateBag {
	state := new(multistep.BasicStateBag)
	state.Put("ui", &packer.BasicUi{Reader: strings.NewReader(""), Writer: io.Discard, ErrorWriter: io.Discard})
	return state
}

func TestStepBootCommandSkipsVNCWithoutBootCommand(t *testing.T) {
	// No expectations: any call to the client fails the test.
	client := kubecli.NewMockKubevirtClient(gomock.NewController(t))
	step := &StepBootCommand{
		config: Config{Name: "vm", Namespace: "ns", BootWait: time.Hour},
		client: client,
	}

	if action := step.Run(context.Background(), newBootCommandState()); action != multistep.ActionContinue {
		t.Fatalf("expected the step to continue, got %v", action)
	}
}

func TestStepBootCommandTypesAndClosesTheVNCConnection(t *testing.T) {
	t.Setenv("PACKER_KEY_INTERVAL", "1ms")

	clientEnd, serverEnd := net.Pipe()
	t.Cleanup(func() { _ = serverEnd.Close() })
	keys, closed := serveVNC(serverEnd)

	ctrl := gomock.NewController(t)
	vmis := kubecli.NewMockVirtualMachineInstanceInterface(ctrl)
	vmis.EXPECT().VNC("vm").Return(&pipeStream{conn: clientEnd}, nil)
	client := kubecli.NewMockKubevirtClient(ctrl)
	client.EXPECT().VirtualMachineInstance("ns").Return(vmis)

	step := &StepBootCommand{
		config: Config{Name: "vm", Namespace: "ns", BootCommand: []string{"a"}},
		client: client,
	}

	state := newBootCommandState()
	if action := step.Run(context.Background(), state); action != multistep.ActionContinue {
		t.Fatalf("expected the step to continue, got %v: %v", action, state.Get("error"))
	}

	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("the VNC connection was left open")
	}

	var typed []uint32
	for key := range keys {
		typed = append(typed, key)
	}
	if len(typed) != 1 || typed[0] != 'a' {
		t.Fatalf("expected the key 'a' to be typed, got %v", typed)
	}
}
