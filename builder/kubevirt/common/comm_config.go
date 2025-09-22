package common

import (
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
)

func CommHost(host string) func(multistep.StateBag) (string, error) {
	return func(state multistep.StateBag) (string, error) {
		// forwarding_host may have been set by step_start_portforward.
		// If we are using forwarding, then it should override any other
		// IP source, as we need to instruct the communicator to
		// connect directly to it.
		fwdhost := state.Get("forwarding_host")
		if fwdhost != nil {
			return fwdhost.(string), nil
		}
		// host would be provided by the user via ssh_host config option.
		// If so, override any detected IP:
		if host != "" {
			return host, nil
		}
		// If the wait_for_ip step has detected an IP and we haven't yet
		// selected one, use that IP:
		guestIp := state.Get("ip")
		if guestIp != nil {
			return guestIp.(string), nil
		}
		// Otherwise, return an error. We don't want to return nothing
		// for the connection target:
		return "", fmt.Errorf("no forwarding host, guest-provided IP, or specified host found")
	}
}

func CommPort(port int) func(multistep.StateBag) (int, error) {
	return func(state multistep.StateBag) (int, error) {
		fwdport := state.Get("forwarding_port")
		if fwdport != nil {
			return fwdport.(int), nil
		}
		return port, nil
	}
}
