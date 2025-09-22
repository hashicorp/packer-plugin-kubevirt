//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type WaitForAgent

package iso

import (
	"context"
	"log"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubevirt.io/client-go/kubecli"
)

type WaitForAgentConfig struct {
	// AgentWaitTimeout is the amount of time to wait for the QEMU Guest Agent to be
	// available.
	// If the Guest Agent does not become available before the timeout, the installation
	// will be cancelled. When set to `0s`, waiting for the guest agent to be available
	// is skipped. Defaults to `0s` (do not wait for the QEMU Guest Agent).
	// Refer to the Golang [ParseDuration](https://golang.org/pkg/time/#ParseDuration)
	// documentation for full details.
	AgentWaitTimeout time.Duration `mapstructure:"agent_wait_timeout" required:"false"`
}

type StepWaitForAgent struct {
	Config Config
	Client kubecli.KubevirtClient
}

func (s *StepWaitForAgent) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	var interval time.Duration

	ui := state.Get("ui").(packer.Ui)
	name := s.Config.Name
	namespace := s.Config.Namespace
	agentWaitTimeout := s.Config.AgentWaitTimeout

	if agentWaitTimeout.Seconds() == 0 {
		ui.Say("agent_wait_timeout is not set, not waiting for guest agent")
		return multistep.ActionContinue
	}
	ui.Sayf("Waiting for up to %s for the Guest Agent to become available", agentWaitTimeout)

	if agentWaitTimeout.Seconds() >= 120 {
		interval = 30 * time.Second
	} else if agentWaitTimeout.Seconds() >= 60 {
		interval = 15 * time.Second
	} else if agentWaitTimeout.Seconds() >= 10 {
		interval = 5 * time.Second
	} else {
		interval = 1 * time.Second
	}

	timeout := time.After(agentWaitTimeout)

	for {
		select {
		case <-timeout:
			ui.Error("Guest Agent wait timeout exceeded")
			return multistep.ActionHalt
		case <-ctx.Done():
			log.Println("[DEBUG] Guest Agent wait cancelled. Exiting loop.")
			ui.Error("Guest Agent wait cancelled")
			return multistep.ActionHalt
		case <-time.After(interval):
			log.Println("[DEBUG] Looping waiting for Guest Agent...")
		}

		vmi, err := s.Client.VirtualMachineInstance(namespace).Get(ctx, name, metav1.GetOptions{})

		if err != nil {
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		for _, condition := range vmi.Status.Conditions {
			if condition.Type == "AgentConnected" {
				if condition.Status == v1.ConditionTrue {
					ui.Sayf("Guest Agent connection has been detected")
					state.Put("guest_agent", true)

					return multistep.ActionContinue
				}
			}
		}
	}
}

func (s *StepWaitForAgent) Cleanup(multistep.StateBag) {
	// Left blank intentionally
}
