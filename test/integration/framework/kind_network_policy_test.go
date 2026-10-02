package framework

import (
	"runtime"
	"testing"
)

func TestEffectiveKindNetworkPolicy(t *testing.T) {
	t.Run("When KIND_NETWORK_POLICY is unset, it should use the platform default", func(t *testing.T) {
		t.Setenv(kindNetworkPolicyEnv, "")
		got, err := EffectiveKindNetworkPolicy()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := kindNetworkPolicyOn
		if runtime.GOOS == "darwin" {
			want = kindNetworkPolicyOff
		}
		if got != want {
			t.Fatalf("expected %q, got %q", want, got)
		}
	})

	t.Run("When KIND_NETWORK_POLICY is on, it should return on", func(t *testing.T) {
		t.Setenv(kindNetworkPolicyEnv, "on")
		got, err := EffectiveKindNetworkPolicy()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != kindNetworkPolicyOn {
			t.Fatalf("expected %q, got %q", kindNetworkPolicyOn, got)
		}
	})

	t.Run("When KIND_NETWORK_POLICY is OFF (mixed case), it should return off", func(t *testing.T) {
		t.Setenv(kindNetworkPolicyEnv, "OFF")
		got, err := EffectiveKindNetworkPolicy()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != kindNetworkPolicyOff {
			t.Fatalf("expected %q, got %q", kindNetworkPolicyOff, got)
		}
	})

	t.Run("When KIND_NETWORK_POLICY is invalid, it should return an error", func(t *testing.T) {
		t.Setenv(kindNetworkPolicyEnv, "maybe")
		_, err := EffectiveKindNetworkPolicy()
		if err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestKindNetworkPolicyOff(t *testing.T) {
	t.Run("When KIND_NETWORK_POLICY is off, it should return true", func(t *testing.T) {
		t.Setenv(kindNetworkPolicyEnv, "off")
		got, err := KindNetworkPolicyOff()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got {
			t.Fatal("expected true")
		}
	})

	t.Run("When KIND_NETWORK_POLICY is on, it should return false", func(t *testing.T) {
		t.Setenv(kindNetworkPolicyEnv, "on")
		got, err := KindNetworkPolicyOff()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got {
			t.Fatal("expected false")
		}
	})
}
