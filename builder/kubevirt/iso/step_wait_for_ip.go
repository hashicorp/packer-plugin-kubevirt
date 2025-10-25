// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

// The wait_for_ip functionality here was heavily borrowed from the
// Packer vSphere plugin

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type WaitIpConfig

package iso

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"kubevirt.io/client-go/kubecli"
)

type WaitIpConfig struct {
	// Amount of time to wait for VM's IP, similar to 'ssh_timeout'.
	// Defaults to `30m` (30 minutes). Refer to the Golang
	// [ParseDuration](https://golang.org/pkg/time/#ParseDuration)
	// documentation for full details.
	WaitTimeout time.Duration `mapstructure:"ip_wait_timeout"`
	// Amount of time to wait for VM's IP to settle down, sometimes VM may
	// report incorrect IP initially, then it is recommended to set that
	// parameter to apx. 2 minutes. Examples `45s` and `10m`.
	// Defaults to `5s` (5 seconds). Refer to the Golang
	// [ParseDuration](https://golang.org/pkg/time/#ParseDuration)
	// documentation for full details.
	SettleTimeout time.Duration `mapstructure:"ip_settle_timeout"`

	// WaitTimeout is a total timeout. If the virtual machine changes IP frequently, and does not settle down, wait
	// until the timeout expires.
}

type StepWaitForIp struct {
	Config Config
	Client kubecli.KubevirtClient
}

func (c *WaitIpConfig) Prepare() []error {
	var errs []error

	if c.SettleTimeout == 0 {
		c.SettleTimeout = 5 * time.Second
	}
	if c.WaitTimeout == 0 {
		c.WaitTimeout = 30 * time.Minute
	}

	return errs
}

func (s *StepWaitForIp) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)

	var ip string
	var err error

	if s.Config.WaitIpConfig.WaitTimeout == 0 {
		log.Print("[INFO] ip_wait_timeout == 0, skipping wait")
		return multistep.ActionContinue
	}

	sub, cancel := context.WithCancel(ctx)
	waitDone := make(chan bool, 1)
	defer func() {
		cancel()
	}()

	log.Printf("[INFO] Waiting for IP, up to total timeout: %s, settle timeout: %s", s.Config.WaitIpConfig.WaitTimeout, s.Config.WaitIpConfig.SettleTimeout)
	timeout := time.After(s.Config.WaitIpConfig.WaitTimeout)

	ui.Say("Waiting for IP...")

	go func() {
		ip, err = doGetIp(sub, s.Client, s.Config)
		waitDone <- true
	}()

	for {
		select {
		case <-timeout:
			cancel()
			<-waitDone
			if ip != "" {
				state.Put("ip", ip)
				log.Printf("[WARN] API timeout waiting for IP but one IP was found. Using IP: %s", ip)
				return multistep.ActionContinue
			}
			err := fmt.Errorf("timeout waiting for IP address")
			state.Put("error", err)
			ui.Errorf("%s", err)
			return multistep.ActionHalt
		case <-ctx.Done():
			cancel()
			log.Println("[WARN] Interrupt detected, quitting waiting for IP.")
			return multistep.ActionHalt
		case <-waitDone:
			if err != nil {
				state.Put("error", err)
				ui.Errorf("%s", err)
				return multistep.ActionHalt
			}
			state.Put("ip", ip)
			ui.Sayf("VM IP found: %s", ip)
			return multistep.ActionContinue
		case <-time.After(1 * time.Second):
			if _, ok := state.GetOk(multistep.StateCancelled); ok {
				return multistep.ActionHalt
			}
		}
	}
}

func doGetIp(ctx context.Context, client kubecli.KubevirtClient, config Config) (string, error) {
	var prevIp = ""
	var stopTime time.Time
	var interval time.Duration

	c := config.WaitIpConfig
	if c.SettleTimeout.Seconds() >= 120 {
		interval = 30 * time.Second
	} else if c.SettleTimeout.Seconds() >= 60 {
		interval = 15 * time.Second
	} else if c.SettleTimeout.Seconds() >= 10 {
		interval = 5 * time.Second
	} else {
		interval = 1 * time.Second
	}

loop:
	ip, err := vmGetIp(ctx, client, config.Namespace, config.VMName)
	if err != nil {
		return "", err
	}

	// Check for ctx cancellation to avoid printing any IP logs at the timeout
	select {
	case <-ctx.Done():
		return ip, fmt.Errorf("cancelled waiting for IP address")
	case <-time.After(interval):
		if prevIp == "" && ip == "" {
			log.Printf("[DEBUG] IP not yet acquired")
		} else if prevIp == "" || prevIp != ip {
			if prevIp == "" {
				log.Printf("VM IP acquired: %s", ip)
				if c.SettleTimeout.Seconds() == 0 {
					log.Printf("ip_settle_timeout is 0, using the first IP seen")
					return ip, nil
				}
			} else {
				log.Printf("VM IP changed from %s to %s", prevIp, ip)
			}
			prevIp = ip
			stopTime = time.Now().Add(c.SettleTimeout)
		} else {
			log.Printf("VM IP is still the same: %s", prevIp)
			if time.Now().After(stopTime) {
				if strings.Contains(ip, ":") {
					// To use a literal IPv6 address in a URL the literal address should be enclosed in
					// "[" and "]" characters. Refer to https://www.ietf.org/rfc/rfc2732.
					// Example: ssh example@[2010:836B:4179::836B:4179]
					ip = "[" + ip + "]"
				}
				log.Printf("VM IP seems stable enough: %s", ip)
				return ip, nil
			}
		}
		goto loop
	}
}

func vmGetIp(ctx context.Context, client kubecli.KubevirtClient, namespace string, name string) (string, error) {
	vmi, err := client.VirtualMachineInstance(namespace).Get(ctx, name, metav1.GetOptions{})

	if err != nil {
		return "", fmt.Errorf("error getting VMI instance: %v", err)
	}

	if vmi.Status.Interfaces == nil {
		return "", fmt.Errorf("vmi instance unexpectededly does not have 'Interfaces' members")
	}

	if len(vmi.Status.Interfaces) == 0 {
		return "", nil
	}

	ip := vmi.Status.Interfaces[0].IP

	if ip == "" {
		return "", nil
	} else {
		return ip, nil
	}
}

func (s *StepWaitForIp) Cleanup(state multistep.StateBag) {}
