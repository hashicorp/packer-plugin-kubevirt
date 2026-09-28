// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"fmt"
	"log"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubevirt.io/client-go/kubecli"
)

// StepPreValidate fails the build before any resource is created when the
// bootable volume it produces already exists, instead of after the whole
// installation when the volume is created.
type StepPreValidate struct {
	Config Config
	Client kubecli.KubevirtClient
}

func (s *StepPreValidate) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	name := s.Config.Name
	namespace := s.Config.Namespace

	ui.Sayf("Checking that the bootable volume (%s/%s) does not exist yet...", namespace, name)

	cdi := s.Client.CdiClient().CdiV1beta1()
	checks := []struct {
		kind string
		get  func() error
	}{
		{"DataVolume", func() error {
			_, err := cdi.DataVolumes(namespace).Get(ctx, name, metav1.GetOptions{})
			return err
		}},
		{"DataSource", func() error {
			_, err := cdi.DataSources(namespace).Get(ctx, name, metav1.GetOptions{})
			return err
		}},
		{"PersistentVolumeClaim", func() error {
			_, err := s.Client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, name, metav1.GetOptions{})
			return err
		}},
	}

	for _, check := range checks {
		err := check.get()
		switch {
		case err == nil:
			return halt(state, fmt.Errorf("a %s named %s/%s already exists, delete it or choose another name for the image", check.kind, namespace, name))
		case apierrors.IsNotFound(err):
		case apierrors.IsForbidden(err):
			// Building the image does not necessarily need this permission,
			// so the check must not make it a requirement.
			log.Printf("[WARN] not allowed to check whether the %s %s/%s exists: %s", check.kind, namespace, name, err)
		default:
			return halt(state, fmt.Errorf("failed to check whether the %s %s/%s exists: %w", check.kind, namespace, name, err))
		}
	}
	return multistep.ActionContinue
}

func (s *StepPreValidate) Cleanup(state multistep.StateBag) {
	// Left blank intentionally
}
