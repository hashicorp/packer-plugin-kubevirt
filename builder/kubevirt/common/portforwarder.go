/*
 * This file is part of the KubeVirt project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright The KubeVirt Authors.
 *
 */

package common

import (
	"errors"
	"io"
	"net"
	"strings"
	"sync"

	kvcorev1 "kubevirt.io/client-go/kubevirt/typed/core/v1"
	"kubevirt.io/client-go/log"
)

const (
	ProtocolTCP = "tcp"
)

type PortForward struct {
	Address  *net.IPAddr
	Resource PortforwardableResource
}

type PortForwarder struct {
	Kind, Namespace, Name string
	Resource              PortforwardableResource

	mu       sync.Mutex
	listener net.Listener
	conns    map[net.Conn]struct{}
	closed   bool
	lastErr  error
}

type ForwardedPort struct {
	Local    int
	Remote   int
	Protocol string
}

type PortforwardableResource interface {
	PortForward(name string, port int, protocol string) (kvcorev1.StreamInterface, error)
}

// StartForwarding listens on the given local address and forwards every
// accepted connection to the remote port of the resource. It returns the
// address the listener is bound to, which carries the port allocated by the
// operating system when port.Local is 0.
func (p *PortForwarder) StartForwarding(address *net.IPAddr, port ForwardedPort) (net.Addr, error) {
	log.Log.Infof("forwarding %s %s:%d to %d", port.Protocol, address, port.Local, port.Remote)

	if port.Protocol == ProtocolTCP {
		return p.StartForwardingTCP(address, port)
	}
	return nil, errors.New("unknown protocol: " + port.Protocol)
}

func (p *PortForwarder) StartForwardingTCP(address *net.IPAddr, port ForwardedPort) (net.Addr, error) {
	listener, err := net.ListenTCP(
		port.Protocol,
		&net.TCPAddr{
			IP:   address.IP,
			Zone: address.Zone,
			Port: port.Local,
		})
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	p.listener = listener
	p.mu.Unlock()

	go p.WaitForConnection(listener, port)
	return listener.Addr(), nil
}

// Close stops accepting new connections and closes the open tunnels, such as
// the persistent connection of the SSH communicator. The forwarder cannot be
// reused afterwards.
func (p *PortForwarder) Close() error {
	p.mu.Lock()
	listener, conns := p.listener, p.conns
	p.listener, p.conns, p.closed = nil, nil, true
	p.mu.Unlock()

	var err error
	if listener != nil {
		if err = listener.Close(); errors.Is(err, net.ErrClosed) {
			err = nil
		}
	}
	for conn := range conns {
		conn.Close()
	}
	return err
}

// track registers the connections of a tunnel so that Close can close them.
// It returns false if the forwarder is already closed.
func (p *PortForwarder) track(conns ...net.Conn) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return false
	}
	if p.conns == nil {
		p.conns = make(map[net.Conn]struct{})
	}
	for _, conn := range conns {
		p.conns[conn] = struct{}{}
	}
	return true
}

func (p *PortForwarder) untrack(conns ...net.Conn) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, conn := range conns {
		delete(p.conns, conn)
	}
}

func (p *PortForwarder) closeListener(listener net.Listener) {
	if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		log.Log.Errorf("error closing listener: %v", err)
	}
}

func (p *PortForwarder) WaitForConnection(listener net.Listener, port ForwardedPort) {
	defer p.closeListener(listener)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				log.Log.Errorf("error accepting connection: %v", err)
			}
			return
		}
		log.Log.Infof("opening new tcp tunnel to %d", port.Remote)
		stream, err := p.Resource.PortForward(p.Name, port.Remote, port.Protocol)
		p.recordError(err)
		if err != nil {
			log.Log.Errorf("can't access %s/%s.%s: %v", p.Kind, p.Name, p.Namespace, err)
			conn.Close()
			continue
		}
		go p.HandleConnection(conn, stream.AsConn(), port)
	}
}

// LastError returns the error of the most recent tunnel to the resource, or
// nil if it was opened and closed cleanly. The communicator only sees a closed
// connection, so this is the only place the actual reason is available.
func (p *PortForwarder) LastError() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastErr
}

func (p *PortForwarder) recordError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastErr = err
}

// handleConnection copies data between the local connection and the stream to
// the remote server.
func (p *PortForwarder) HandleConnection(local, remote net.Conn, port ForwardedPort) {
	if !p.track(local, remote) {
		local.Close()
		remote.Close()
		return
	}
	defer p.untrack(local, remote)

	log.Log.Infof("handling tcp connection for %d", port.Local)
	errs := make(chan error)
	go func() {
		_, err := io.Copy(remote, local)
		errs <- err
	}()
	go func() {
		_, err := io.Copy(local, remote)
		errs <- err
	}()

	// Only the first error explains why the tunnel ended, the second one is
	// caused by closing both connections below.
	if err := <-errs; isConnectionError(err) {
		HandleConnectionError(err, port)
		p.recordError(err)
	}
	local.Close()
	remote.Close()
	HandleConnectionError(<-errs, port)
}

func HandleConnectionError(err error, port ForwardedPort) {
	if isConnectionError(err) {
		log.Log.Errorf("error handling connection for %d: %v", port.Local, err)
	}
}

func isConnectionError(err error) bool {
	return err != nil && !strings.Contains(err.Error(), "use of closed network connection")
}
