package framework

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	homanifests "github.com/openshift/hypershift/hypershift-operator/controllers/manifests"

	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/go-logr/logr"
)

const integrationAllowAllNetworkPolicyName = "integration-allow-all"

// ensureIntegrationAllowAllNetworkPolicy waits for the HCP namespace (created by the HO after
// HostedCluster apply) and applies an allow-all NetworkPolicy there.
// Used when KIND_NETWORK_POLICY=off. Stock kindest/kindnetd enforces NetworkPolicies without a
// disable flag; HyperShift ingress NetworkPolicies in HCP namespaces break DNS under common kind
// setups. Set KIND_NETWORK_POLICY=on to skip this and test real NetworkPolicies.
func ensureIntegrationAllowAllNetworkPolicy(ctx context.Context, logger logr.Logger, opts *Options, hostedClusterNamespace, hostedClusterName string) error {
	namespace := homanifests.HostedControlPlaneNamespace(hostedClusterNamespace, hostedClusterName)
	logger.Info("waiting for HCP namespace before applying integration allow-all NetworkPolicy", "namespace", namespace)

	if err := waitForNamespace(ctx, opts, namespace); err != nil {
		return fmt.Errorf("waiting for HCP namespace %s: %w", namespace, err)
	}

	logger.Info("ensuring integration allow-all NetworkPolicy for kind DNS workaround", "namespace", namespace, "name", integrationAllowAllNetworkPolicyName)

	yamlPath := filepath.Join("install", "integration-allow-all-networkpolicy.yaml")
	yamlFile, err := Artifact(opts, yamlPath)
	if err != nil {
		return err
	}
	content := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: %s
  namespace: %s
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - {}
  egress:
  - {}
`, integrationAllowAllNetworkPolicyName, namespace)
	if _, err := yamlFile.WriteString(content); err != nil {
		_ = yamlFile.Close()
		return fmt.Errorf("failed to write allow-all NetworkPolicy artifact: %w", err)
	}
	if err := yamlFile.Close(); err != nil {
		return fmt.Errorf("failed to close allow-all NetworkPolicy artifact: %w", err)
	}

	applyLogPath := filepath.Join("install", "integration-allow-all-networkpolicy.apply.log")
	applyCmd := exec.CommandContext(ctx, opts.OCPath,
		"apply", "--server-side", "-f", filepath.Join(opts.ArtifactDir, yamlPath), "--kubeconfig", opts.Kubeconfig,
	)
	applyCmd.Env = append(os.Environ(), "KUBECONFIG="+opts.Kubeconfig)
	if err := RunCommand(logger, opts, applyLogPath, applyCmd); err != nil {
		return fmt.Errorf("failed to apply allow-all NetworkPolicy: %w", err)
	}
	return nil
}

func waitForNamespace(ctx context.Context, opts *Options, namespace string) error {
	return wait.PollUntilContextTimeout(ctx, time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		cmd := exec.CommandContext(ctx, opts.OCPath,
			"get", "namespace", namespace, "--kubeconfig", opts.Kubeconfig,
		)
		if err := cmd.Run(); err != nil {
			return false, nil
		}
		return true, nil
	})
}
