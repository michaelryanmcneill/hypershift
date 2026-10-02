package framework

import "testing"

func TestIsHostedClusterTeardownArtifact(t *testing.T) {
	t.Run("When name is assets.yaml, it should return true", func(t *testing.T) {
		if !isHostedClusterTeardownArtifact("assets.yaml") {
			t.Fatal("expected true")
		}
	})

	t.Run("When name is integration-allow-all-networkpolicy.yaml, it should return true", func(t *testing.T) {
		if !isHostedClusterTeardownArtifact("integration-allow-all-networkpolicy.yaml") {
			t.Fatal("expected true")
		}
	})

	t.Run("When name is an unrelated log file, it should return false", func(t *testing.T) {
		if isHostedClusterTeardownArtifact("assets.apply.log") {
			t.Fatal("expected false")
		}
	})
}
