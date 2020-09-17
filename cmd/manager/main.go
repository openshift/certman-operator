package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	// Hive provides cluster deployment status
	routev1 "github.com/openshift/api/route/v1"
	hivev1 "github.com/openshift/hive/pkg/apis/hive/v1"
	"github.com/openshift/operator-custom-metrics/pkg/metrics"
	"github.com/operator-framework/operator-sdk/pkg/k8sutil"
	"github.com/operator-framework/operator-sdk/pkg/leader"
	"github.com/operator-framework/operator-sdk/pkg/log/zap"
	sdkVersion "github.com/operator-framework/operator-sdk/version"
	"github.com/spf13/pflag"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	operatorconfig "github.com/openshift/certman-operator/config"
	"github.com/openshift/certman-operator/pkg/apis"
	"github.com/openshift/certman-operator/pkg/controller"
	"github.com/openshift/certman-operator/pkg/localmetrics"
)

// Change below variables to serve metrics on different host or port.
var (
	metricsHost                   = "0.0.0.0"
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

	namespace, err := k8sutil.GetWatchNamespace()
	if err != nil {
		log.Error(err, "Failed to get watch namespace")
		os.Exit(1)
	}

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	ctx := context.TODO()
	// Become the leader before proceeding
	err = leader.Become(ctx, "certman-operator-lock")
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Set default manager options
	options := manager.Options{
		Namespace: namespace,
		// Disable controller-runtime metrics serving
		MetricsBindAddress: "0",
	}

	// Add support for MultiNamespace set in WATCH_NAMESPACE (e.g ns1,ns2)
	// Note that this is not intended to be used for excluding namespaces, this is better done via a Predicate
	// Also note that you may face performance issues when using this with a high number of namespaces.
	// More Info: https://godoc.org/github.com/kubernetes-sigs/controller-runtime/pkg/cache#MultiNamespacedCacheBuilder
	if strings.Contains(namespace, ",") {
		options.Namespace = ""
		options.NewCache = cache.MultiNamespacedCacheBuilder(strings.Split(namespace, ","))
	}

	// Create a new manager to provide shared dependencies and start components
	mgr, err := manager.New(cfg, options)
	if err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	log.Info("Registering Components.")

	// Setup Scheme for all resources
	if err := apis.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Assemble hivev1 runtime scheme.
	if err := hivev1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "error registering hive objects")
		os.Exit(1)
	}

	// Assemble routev1 runtime scheme.
	if err := routev1.AddToScheme(mgr.GetScheme()); err != nil {
		log.Error(err, "error registering prometheus monitoring objects")
		os.Exit(1)
	}

	// Setup all Controllers
	if err := controller.AddToManager(mgr); err != nil {
		log.Error(err, "")
		os.Exit(1)
	}

	// Instantiate metricsServer object configured with variables defined in
	// localmetrics package.
	metricsServer := metrics.NewBuilder(operatorconfig.OperatorNamespace, operatorconfig.OperatorName).
		WithPort(metricsPort).
		WithPath(metricsPath).
		WithCollectors(localmetrics.MetricsList).
		WithRoute().
		GetConfig()

	// Get the namespace the operator is currently deployed in.
	if _, err := k8sutil.GetOperatorNamespace(); err != nil {
		if errors.Is(err, k8sutil.ErrRunLocal) {
			log.Info("Skipping CR metrics server creation; not running in a cluster.")
		} else {
			log.Error(err, "Failed to get operator namespace")
		}
	} else {
		if err := metrics.ConfigureMetrics(context.TODO(), *metricsServer); err != nil {
			log.Error(err, "Failed to configure Metrics")
			os.Exit(1)
		}
		log.Info("Successfully configured Metrics")
	}

	// Invoke UpdateMetrics at a frequency defined as hours within a goroutine.
	go localmetrics.UpdateMetrics(hours)
	log.Info("Starting the Cmd.")

	// Start the Cmd
	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		log.Error(err, "Manager exited non-zero")
		os.Exit(1)
	}
}
