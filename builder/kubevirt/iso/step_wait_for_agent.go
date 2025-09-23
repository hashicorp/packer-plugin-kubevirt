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
	// AgentWaitTimeout is the amount of time to wait for the Guest Agent to be available.
	// If the Guest Agent does not become available before the timeout, the installation
	// will be cancelled.
	AgentWaitTimeout time.Duration `mapstructure:"agent_wait_timeout" required:"false"`
}

type StepWaitForAgent struct {
	Config Config
	Client kubecli.KubevirtClient
}

func (s *StepWaitForAgent) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	name := s.Config.Name
	namespace := s.Config.Namespace
	AgentWaitTimeout := s.Config.AgentWaitTimeout

	if int64(AgentWaitTimeout) == 0 {
		ui.Say("agent_wait_timeout is not set, not waiting for guest agent")
		return multistep.ActionContinue
	}
	ui.Sayf("Waiting for up to %s for the Guest Agent to become available", AgentWaitTimeout)

	timeout := time.After(AgentWaitTimeout)

	for {
		select {
		case <-timeout:
			ui.Error("Guest Agent wait timeout exceeded")
			return multistep.ActionHalt
		case <-ctx.Done():
			log.Println("[DEBUG] Guest Agent wait cancelled. Exiting loop.")
			ui.Error("Guest Agent wait cancelled")
			return multistep.ActionHalt
		case <-time.After(15 * time.Second):
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
