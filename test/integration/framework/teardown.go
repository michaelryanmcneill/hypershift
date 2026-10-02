package framework

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"github.com/go-logr/logr"
)

// TearDown removes HostedClusters, allow-all NetworkPolicies (when present), HyperShift Operator
// resources, CRDs, and static assets previously installed by SetupMode. It uses rendered YAML under
// opts.ArtifactDir plus the embedded static assets. Order matches the reverse of setup.
func TearDown(ctx context.Context, logger logr.Logger, opts *Options) error {
	var errs []error

	if err := tearDownHostedClusters(ctx, logger, opts); err != nil {
		errs = append(errs, err)
	}
	if err := tearDownHyperShiftOperator(ctx, logger, opts); err != nil {
		errs = append(errs, err)
	}
	if err := tearDownHyperShiftCRDs(ctx, logger, opts); err != nil {
		errs = append(errs, err)
	}
	if err := tearDownAssets(ctx, logger, opts); err != nil {
		errs = append(errs, err)
	}

	return utilerrors.NewAggregate(errs)
}

func tearDownHostedClusters(ctx context.Context, logger logr.Logger, opts *Options) error {
	if SkippedCleanupSteps().HasAny("all", "hosted-clusters") {
		return nil
	}
	if _, err := os.Stat(opts.ArtifactDir); err != nil {
		if os.IsNotExist(err) {
			logger.Info("artifact directory does not exist, skipping hosted cluster teardown", "dir", opts.ArtifactDir)
			return nil
		}
		return err
	}

	var errs []error
	err := filepath.WalkDir(opts.ArtifactDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !isHostedClusterTeardownArtifact(d.Name()) {
			return nil
		}
		logger.Info("cleaning up hosted cluster assets", "path", path)
		rel, relErr := filepath.Rel(opts.ArtifactDir, path)
		if relErr != nil {
			rel = path
		}
		deleteLogPath := filepath.Join("teardown", rel+".delete.log")
		deleteCmd := exec.CommandContext(ctx, opts.OCPath,
			"delete", "--ignore-not-found", "-f", path, "--kubeconfig", opts.Kubeconfig,
		)
		if runErr := RunCommand(logger, opts, deleteLogPath, deleteCmd); runErr != nil {
			errs = append(errs, fmt.Errorf("failed to delete hosted cluster assets %s: %w", path, runErr))
		}
		return nil
	})
	if err != nil {
		errs = append(errs, err)
	}
	return utilerrors.NewAggregate(errs)
}

// isHostedClusterTeardownArtifact reports whether a rendered artifact under a test's
// artifact directory should be deleted during teardown. Always includes the allow-all
// NetworkPolicy when present — teardown must not depend on KIND_NETWORK_POLICY.
func isHostedClusterTeardownArtifact(name string) bool {
	switch name {
	case "assets.yaml", "integration-allow-all-networkpolicy.yaml":
		return true
	default:
		return false
	}
}

func tearDownHyperShiftOperator(ctx context.Context, logger logr.Logger, opts *Options) error {
	if SkippedCleanupSteps().HasAny("all", "hypershift-operator") {
		return nil
	}
	yamlPath := filepath.Join(opts.ArtifactDir, "install", "hypershift-install.yaml")
	if _, err := os.Stat(yamlPath); err != nil {
		if os.IsNotExist(err) {
			logger.Info("hypershift operator install artifacts not found, skipping", "path", yamlPath)
			return nil
		}
		return err
	}

	logger.Info("dumping hypershift operator assets before teardown")
	dumpLogPath := filepath.Join("teardown", "hypershift-install.dump.yaml")
	dumpCmd := exec.CommandContext(ctx, opts.OCPath,
		"get", "--ignore-not-found", "--show-managed-fields", "-f", yamlPath, "--kubeconfig", opts.Kubeconfig,
	)
	if err := RunCommand(logger, opts, dumpLogPath, dumpCmd); err != nil {
		logger.Error(err, "failed to dump hypershift operator assets")
	}

	logger.Info("cleaning up hypershift operator assets")
	deleteLogPath := filepath.Join("teardown", "hypershift-install.delete.log")
	deleteCmd := exec.CommandContext(ctx, opts.OCPath,
		"delete", "--ignore-not-found", "-f", yamlPath, "--kubeconfig", opts.Kubeconfig,
	)
	return RunCommand(logger, opts, deleteLogPath, deleteCmd)
}

func tearDownHyperShiftCRDs(ctx context.Context, logger logr.Logger, opts *Options) error {
	if SkippedCleanupSteps().HasAny("all", "hypershift-operator") {
		return nil
	}
	yamlPath := filepath.Join(opts.ArtifactDir, "install", "hypershift-install-crds.yaml")
	if _, err := os.Stat(yamlPath); err != nil {
		if os.IsNotExist(err) {
			logger.Info("hypershift CRD install artifacts not found, skipping", "path", yamlPath)
			return nil
		}
		return err
	}

	logger.Info("cleaning up hypershift CRDs")
	deleteLogPath := filepath.Join("teardown", "hypershift-install-crds.delete.log")
	deleteCmd := exec.CommandContext(ctx, opts.OCPath,
		"delete", "--ignore-not-found", "-f", yamlPath, "--kubeconfig", opts.Kubeconfig,
	)
	return RunCommand(logger, opts, deleteLogPath, deleteCmd)
}

func tearDownAssets(ctx context.Context, logger logr.Logger, opts *Options) error {
	if SkippedCleanupSteps().HasAny("all", "assets") {
		return nil
	}
	logger.Info("cleaning up static assets")
	return fs.WalkDir(assets, "assets", processAsset(logger, func(path string, content io.Reader) error {
		logPath := filepath.Join("teardown", path+".delete.log")
		cmd := exec.CommandContext(ctx, opts.OCPath,
			"delete", "--ignore-not-found", "-f", "-", "--kubeconfig", opts.Kubeconfig,
		)
		cmd.Stdin = content
		if err := RunCommand(logger, opts, logPath, cmd); err != nil {
			return fmt.Errorf("failed to delete %s: %w", path, err)
		}
		return nil
	}))
}
