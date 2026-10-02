package framework

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

const (
	kindNetworkPolicyEnv = "KIND_NETWORK_POLICY"
	kindNetworkPolicyOn  = "on"
	kindNetworkPolicyOff = "off"
)

// EffectiveKindNetworkPolicy returns the effective KIND_NETWORK_POLICY value ("on" or "off").
// When unset, defaults to "off" on macOS (Darwin) and "on" elsewhere.
func EffectiveKindNetworkPolicy() (string, error) {
	raw := strings.TrimSpace(os.Getenv(kindNetworkPolicyEnv))
	if raw == "" {
		if runtime.GOOS == "darwin" {
			return kindNetworkPolicyOff, nil
		}
		return kindNetworkPolicyOn, nil
	}
	normalized := strings.ToLower(raw)
	switch normalized {
	case kindNetworkPolicyOn, kindNetworkPolicyOff:
		return normalized, nil
	default:
		return "", fmt.Errorf("%s must be %q or %q (got %q)", kindNetworkPolicyEnv, kindNetworkPolicyOn, kindNetworkPolicyOff, raw)
	}
}

// KindNetworkPolicyOff reports whether the harness should apply an allow-all NetworkPolicy
// in HCP namespaces to work around kindnet DNS breakage from HyperShift ingress NetworkPolicies.
// When KIND_NETWORK_POLICY=on, the harness leaves NetworkPolicies alone so they can be tested.
func KindNetworkPolicyOff() (bool, error) {
	mode, err := EffectiveKindNetworkPolicy()
	if err != nil {
		return false, err
	}
	return mode == kindNetworkPolicyOff, nil
}
