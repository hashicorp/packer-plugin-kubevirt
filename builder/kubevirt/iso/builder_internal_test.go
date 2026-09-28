// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"golang.org/x/crypto/ssh"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	"kubevirt.io/client-go/kubecli"
	kvcorev1 "kubevirt.io/client-go/kubevirt/typed/core/v1"
)

// startSSHServer runs an SSH server that accepts the given credentials and
// counts the successful logins.
func startSSHServer(t *testing.T, user, password string) (string, *atomic.Int32) {
	t.Helper()

	_, hostKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		t.Fatal(err)
	}

	logins := &atomic.Int32{}
	config := &ssh.ServerConfig{
		PasswordCallback: func(meta ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if meta.User() == user && string(pass) == password {
				logins.Add(1)
				return nil, nil
			}
			return nil, io.EOF
		},
	}
	config.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				_, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(requests)
				for newChannel := range channels {
					_, channelRequests, err := newChannel.Accept()
					if err != nil {
						continue
					}
					go ssh.DiscardRequests(channelRequests)
				}
			}()
		}
	}()
	return listener.Addr().String(), logins
}

// sshTunnel plays the role of the KubeVirt portforward subresource by
// connecting every tunnel to the test SSH server.
type sshTunnel struct {
	address    string
	remotePort atomic.Int32
}

func (s *sshTunnel) PortForward(name string, port int, protocol string) (kvcorev1.StreamInterface, error) {
	s.remotePort.Store(int32(port))
	conn, err := net.Dial("tcp", s.address)
	if err != nil {
		return nil, err
	}
	return &connStream{conn: conn}, nil
}

type connStream struct{ conn net.Conn }

func (c *connStream) Stream(kvcorev1.StreamOptions) error { return nil }
func (c *connStream) AsConn() net.Conn                    { return c.conn }

func TestProvisionStepsConnectThroughThePortForward(t *testing.T) {
	sshAddress, logins := startSSHServer(t, "fedora", "secret")
	tunnel := &sshTunnel{address: sshAddress}

	b := &Builder{}
	_, err := b.config.Prepare(map[string]interface{}{
		"kube_config":     "/tmp/kubeconfig",
		"name":            "fedora-image",
		"namespace":       "images",
		"iso_volume_name": "fedora-iso",
		"disk_size":       "10Gi",
		"instance_type":   "u1.medium",
		"preference":      "fedora",
		"communicator":    "ssh",
		"ssh_username":    "fedora",
		"ssh_password":    "secret",
		"ssh_timeout":     "30s",
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	client := kubecli.NewMockKubevirtClient(gomock.NewController(t))
	client.EXPECT().VirtualMachine("images").Return(nil).AnyTimes()
	b.client = client

	steps := b.provisionSteps()
	for _, step := range steps {
		if forward, ok := step.(*StepStartPortForward); ok {
			forward.ForwarderFunc = func(kind, namespace, name string, _ common.PortforwardableResource) PortForwarder {
				return DefaultPortForwarder(kind, namespace, name, tunnel)
			}
		}
	}

	ui := &packer.BasicUi{Reader: strings.NewReader(""), Writer: io.Discard, ErrorWriter: io.Discard}
	hook := &packer.MockHook{}
	state := new(multistep.BasicStateBag)
	state.Put("ui", ui)
	state.Put("hook", hook)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	(&multistep.BasicRunner{Steps: steps}).Run(ctx, state)

	if err, ok := state.GetOk("error"); ok {
		t.Fatalf("provisioning failed: %v", err)
	}
	comm, ok := state.GetOk("communicator")
	if !ok {
		t.Fatal("the communicator did not connect")
	}
	if logins.Load() == 0 {
		t.Fatal("the SSH server did not see a login")
	}
	if got := tunnel.remotePort.Load(); got != 22 {
		t.Fatalf("expected the tunnel to target port 22 of the VM, got %d", got)
	}
	if !hook.RunCalled || hook.RunComm != comm {
		t.Fatal("expected the provisioners to run with the connected communicator")
	}
}
