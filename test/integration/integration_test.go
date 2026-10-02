//go:build integration

package integration

import (
	"context"
	"flag"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/openshift/hypershift/test/integration/framework"
	"go.uber.org/zap/zapcore"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	// opts are global options for the test suite bound in TestMain.
	globalOpts *framework.Options

	// testContext should be used as the parent context for any test code, and will
	// be cancelled if a SIGINT or SIGTERM is received. It's set up in TestMain.
	testContext context.Context

	log = zap.New(zap.UseDevMode(true), zap.ConsoleEncoder(func(o *zapcore.EncoderConfig) {
		o.EncodeTime = zapcore.RFC3339TimeEncoder
	}))
)

func init() {
	// something in controller-runtime pollutes the global flag namespace with an implicit side-effect
	// registry of --kubeconfig on import of their packages, so we reset the flag.CommandLine in order
	// to clear their value ... if we do this in *our* init(), we still get the `go test` flags
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	ctrl.SetLogger(log)

	rand.Seed(time.Now().UnixNano())
}

func TestMain(m *testing.M) {
	globalOpts = framework.DefaultOptions()
	globalOpts.Bind(flag.CommandLine)
	flag.Parse()

	if err := globalOpts.Validate(); err != nil {
		log.Error(err, "invalid options")
		os.Exit(1)
	}

	// Set up a root context for all tests and set up signal handling
	testContext = framework.InterruptableContext(context.Background())

	os.Exit(run(m, testContext, log, globalOpts))
}

func run(m *testing.M, ctx context.Context, logger logr.Logger, opts *framework.Options) int {
	switch opts.Mode {
	case framework.TeardownMode:
		logger.Info("tearing down integration environment", "artifactDir", opts.ArtifactDir)
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		logger.Info("skipping the following cleanup steps", "steps", framework.SkippedCleanupSteps().UnsortedList())
		if err := framework.TearDown(cleanupCtx, logger, opts); err != nil {
			logger.Error(err, "teardown failed")
			return 1
		}
		logger.Info("teardown complete")
		return 0

	case framework.SetupMode:
		// Install shared infrastructure and HostedClusters (via tests), then exit.
		// Cleanup is explicit via TeardownMode so setup does not hold a terminal open.
		if err := installSharedInfrastructure(ctx, logger, opts); err != nil {
			logger.Error(err, "setup failed")
			return 1
		}
		logger.Info("running hosted cluster setup tests")
		code := m.Run()
		if code != 0 {
			return code
		}
		return 0

	case framework.AllInOneMode:
		var cleanups []func(context.Context)
		defer func() {
			// Prefer a timeout over an interruptible context so teardown is not SIGKILL'd
			// when the user hits Ctrl+C again while cleanup is running.
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			log.Info("skipping the following cleanup steps", "steps", framework.SkippedCleanupSteps().UnsortedList())
			for _, cleanup := range cleanups {
				cleanup(cleanupCtx)
			}
		}()

		if err := installSharedInfrastructureWithCleanups(ctx, logger, opts, &cleanups); err != nil {
			logger.Error(err, "setup failed")
			return 1
		}

	case framework.TestMode:
		break
	}

	logger.Info("running tests")
	return m.Run()
}

func installSharedInfrastructure(ctx context.Context, logger logr.Logger, opts *framework.Options) error {
	for _, item := range sharedInfrastructureBuilders() {
		if _, err := item.builder(ctx, logger, opts); err != nil {
			return err
		}
	}
	return nil
}

func installSharedInfrastructureWithCleanups(ctx context.Context, logger logr.Logger, opts *framework.Options, cleanups *[]func(context.Context)) error {
	for _, item := range sharedInfrastructureBuilders() {
		item := item
		cleanup, err := item.builder(ctx, logger, opts)
		*cleanups = append(*cleanups, func(ctx context.Context) {
			if err := cleanup(ctx); err != nil {
				logger.Error(err, "cleaning up "+item.name)
			}
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func sharedInfrastructureBuilders() []struct {
	name    string
	builder framework.Builder
} {
	return []struct {
		name    string
		builder framework.Builder
	}{
		{name: "assets", builder: framework.InstallAssets},
		{name: "crds", builder: framework.InstallHyperShiftCRDs},
		{name: "crd establishment", builder: framework.WaitForHyperShiftCRDs},
		{name: "hypershift operator assets", builder: framework.InstallHyperShiftOperator},
		{name: "hypershift operator readiness", builder: framework.WaitForHyperShiftOperator},
	}
}
