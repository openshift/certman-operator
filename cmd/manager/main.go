package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	// Hive provides cluster deployment status
	routev1 "github.com/openshift/api/route/v1"
	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/openshift/operator-custom-metrics/pkg/metrics"
	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/spf13/pflag"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	operatorconfig "github.com/openshift/certman-operator/config"
	"github.com/openshift/certman-operator/pkg/apis"
	"github.com/openshift/certman-operator/pkg/controller"
	"github.com/openshift/certman-operator/pkg/localmetrics"
)

// Change below variables to serve metrics on different host or port.
var (
	metricsPath                   = "/metrics"
	metricsPort                   = "8080"
	secretWatcherScanInterval     = time.Duration(10) * time.Minute
	hours                     int = 4
)
var log = logf.Log.WithName("cmd")

// Set const to analyze to determine operator-sdk up local
const envSDK string = "OSDK_FORCE_RUN_MODE"

func printVersion() {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Version of operator-sdk: %v", sdkVersion.Version))
}

func start() error {
	// Add the zap logger flag set to the CLI. The flag set must
	// be added before calling pflag.Parse().
	pflag.CommandLine.AddFlagSet(zap.FlagSet())

	// Add flags registered by imported packages (e.g. glog and
	// controller-runtime)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Parse()

	stopCh := signals.SetupSignalHandler()

	// Use a zap logr.Logger implementation. If none of the zap
	// flags are configured (or if the zap flag set is not being
	// used), this defaults to a production zap logger.
	//
	// The logger instantiated here can be changed to any logger
	// implementing the logr.Logger interface. This logger will
	// be propagated through the whole operator, generating
	// uniform and structured logs.
	logf.SetLogger(zap.Logger())

	printVersion()

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	ctx := context.TODO()

	// Become the leader pod if within namespace. If outside a cluster, skip
	// and return nil.
	err = leader.Become(ctx, "certman-operator-lock")
	if err != nil {
		log.Error(err, "")
		return err
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, manager.Options{
		Namespace: "",
	})
	if err != nil {
		log.Error(err, "")
		return err
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	// Assemble apis runtime scheme.
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		return err
	}

	// Assemble hivev1alpha1 runtime scheme.
	if err := hivev1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "error registering hive objects")
		return err
	}

	// Assemble routev1 runtime scheme.
	if err := routev1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "error registering prometheus monitoring objects")
		return err
	}

	// Setup all Controllers.
	if err := controller.AddToManager(mgr); err != nil {
		log.Error(err, "")
		return err
	}

	// start cache and wait for sync
	cache := mgr.GetCache()
	go cache.Start(stopCh)
	cache.WaitForCacheSync(stopCh)

	// Instantiate metricsServer object configured with variables defined in
	// localmetrics package.
	metricsServer := metrics.NewBuilder().WithPort(metricsPort).WithPath(metricsPath).
		WithCollectors(localmetrics.MetricsList).
		WithRoute().
		WithServiceName(operatorconfig.OperatorName).
		GetConfig()

	// detect if operator-sdk up local is run
	detectSDKLocal := os.Getenv(envSDK)

	// Configure metrics. If it errors, log the error and exit.
	if detectSDKLocal == "local" {
		log.Info("Skipping metrics configuration; not running in a cluster.")
	} else {
		if err := metrics.ConfigureMetrics(context.TODO(), *metricsServer); err != nil {
			log.Error(err, "Failed to configure Metrics")
			return err
		}
	}

	// Invoke UpdateMetrics at a frequency defined as hours within a goroutine.
	go localmetrics.UpdateMetrics(hours)
	log.Info("Starting the Cmd.")

	// Start all registered controllers
	return mgr.Start(stopCh)
}

func main() {
	if err := start(); err != nil {
		panic(err)
	}
}
