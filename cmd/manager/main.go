package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	// Hive provides cluster deployment status
	hivev1alpha1 "github.com/openshift/hive/pkg/apis/hive/v1alpha1"
	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"

	routev1 "github.com/openshift/api/route/v1"
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
	"github.com/openshift/operator-custom-metrics/pkg/metrics"
)

// Change below variables to serve metrics on different host or port.
var (
	metricsPath                   = "/metrics"
	metricsPort                   = "8080"
	secretWatcherScanInterval     = time.Duration(10) * time.Minute
	hours                     int = 4
)
var log = logf.Log.WithName("cmd")

func printVersion() {
	log.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	log.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	log.Info(fmt.Sprintf("Version of operator-sdk: %v", sdkVersion.Version))
}

func main() {
	// Add the zap logger flag set to the CLI. The flag set must
	// be added before calling pflag.Parse().
	pflag.CommandLine.AddFlagSet(zap.FlagSet())

	// Add flags registered by imported packages (e.g. glog and
	// controller-runtime)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	pflag.Parse()

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
		os.Exit(1)
	}

	// Create a new Cmd to provide shared dependencies and start components
	mgr, err := manager.New(cfg, manager.Options{
		Namespace: "",
	})
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	// Assemble apis runtime scheme.
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Assemble hivev1alpha1 runtime scheme.
	if err := hivev1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "error registering hive objects")
		os.Exit(1)
	}

	// Assemble routev1 runtime scheme.
	if err := routev1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "error registering prometheus monitoring objects")
		os.Exit(1)
	}

	// Setup all Controllers.
	if err := controller.AddToManager(mgr); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Instantiate metricsServer object configured with variables defined in
	// localmetrics package.
	metricsServer := metrics.NewBuilder().WithPort(metricsPort).WithPath(metricsPath).
		WithCollectors(localmetrics.MetricsList).
		WithRoute().
		WithServiceName(operatorconfig.OperatorName).
		GetConfig()

	// detect if operator-sdk up local is run
	detectLocal := os.Getenv("OPERATOR_UP_LOCAL")

	// Configure metrics. If it errors, log the error but continue.
	if detectLocal != "true" {
		if err := metrics.ConfigureMetrics(context.TODO(), *metricsServer); err != nil {
			log.Error(err, "Failed to configure Metrics")
			os.Exit(1)
		}
	}
	// Invoke UpdateMetrics at a frequency defined as hours within a goroutine.
	go localmetrics.UpdateMetrics(hours)
	log.Info("Starting the Cmd.")

	// Start all registered controllers
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "Manager exited non-zero")
		os.Exit(1)
	}
}
