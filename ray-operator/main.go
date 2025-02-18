package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/ray-project/kuberay/ray-operator/controllers/ray"
	"github.com/ray-project/kuberay/ray-operator/controllers/ray/batchscheduler"

	routev1 "github.com/openshift/api/route/v1"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	k8szap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	rayv1alpha1 "github.com/ray-project/kuberay/ray-operator/apis/ray/v1alpha1"
	// +kubebuilder:scaffold:imports
)

var (
	_version_   = "0.2"
	_buildTime_ = ""
	_commitId_  = ""
	scheme      = runtime.NewScheme()
	setupLog    = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(rayv1alpha1.AddToScheme(scheme))
	utilruntime.Must(routev1.AddToScheme(scheme))
	batchscheduler.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var version bool
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var reconcileConcurrency int
	var watchNamespace string
	var logFile string
	flag.BoolVar(&version, "version", false, "Show the version information.")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8082", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", true,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.IntVar(&reconcileConcurrency, "reconcile-concurrency", 1, "max concurrency for reconciling")
	flag.StringVar(
		&watchNamespace,
		"watch-namespace",
		"",
		"Specify a list of namespaces to watch for custom resources, separated by commas. If left empty, all namespaces will be watched.")
	flag.BoolVar(&ray.ForcedClusterUpgrade, "forced-cluster-upgrade", false,
		"Forced cluster upgrade flag")
	flag.StringVar(&logFile, "log-file-path", "",
		"Synchronize logs to local file")
	flag.BoolVar(&ray.EnableBatchScheduler, "enable-batch-scheduler", false,
		"Enable batch scheduler. Currently is volcano, which supports gang scheduler policy.")

	opts := k8szap.Options{
		Development: true,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	if version {
		fmt.Printf("Version:\t%s\n", _version_)
		fmt.Printf("Commit ID:\t%s\n", _commitId_)
		fmt.Printf("Build time:\t%s\n", _buildTime_)
		os.Exit(0)
	}

	if logFile != "" {
		fileWriter := &lumberjack.Logger{
			Filename:   logFile,
			MaxSize:    500, // megabytes
			MaxBackups: 10,  // files
			MaxAge:     30,  // days
		}

		pe := zap.NewProductionEncoderConfig()
		pe.EncodeTime = zapcore.ISO8601TimeEncoder
		consoleEncoder := zapcore.NewConsoleEncoder(pe)

		k8sLogger := k8szap.NewRaw(k8szap.UseFlagOptions(&opts))
		zapOpts := append(opts.ZapOpts, zap.AddCallerSkip(1))
		combineLogger := zap.New(zapcore.NewTee(
			k8sLogger.Core(),
			zapcore.NewCore(consoleEncoder, zapcore.AddSync(fileWriter), zap.InfoLevel),
		)).WithOptions(zapOpts...)
		combineLoggerR := zapr.NewLogger(combineLogger)

		ctrl.SetLogger(combineLoggerR)
	} else {
		ctrl.SetLogger(k8szap.New(k8szap.UseFlagOptions(&opts)))
	}

	setupLog.Info("the operator", "version:", os.Getenv("OPERATOR_VERSION"))
	if ray.ForcedClusterUpgrade {
		setupLog.Info("Feature flag forced-cluster-upgrade is enabled.")
	}
	if ray.EnableBatchScheduler {
		setupLog.Info("Feature flag enable-batch-scheduler is enabled.")
	}

	watchNamespaces := strings.Split(watchNamespace, ",")
	options := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "ray-operator-leader",
	}

	if len(watchNamespaces) == 1 { // It is not possible for len(watchNamespaces) == 0 to be true. The length of `strings.Split("", ",")`` is still 1.
		options.Namespace = watchNamespaces[0]
		if watchNamespaces[0] == "" {
			setupLog.Info("Flag watchNamespace is not set. Watch custom resources in all namespaces.")
		} else {
			setupLog.Info(fmt.Sprintf("Only watch custom resources in the namespace: %s", watchNamespaces[0]))
		}
	} else {
		options.NewCache = cache.MultiNamespacedCacheBuilder(watchNamespaces)
		setupLog.Info(fmt.Sprintf("Only watch custom resources in multiple namespaces: %v", watchNamespaces))
	}
	setupLog.Info("Setup manager")
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = ray.NewReconciler(mgr).SetupWithManager(mgr, reconcileConcurrency); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RayCluster")
		os.Exit(1)
	}
	if err = ray.NewRayServiceReconciler(mgr).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RayService")
		os.Exit(1)
	}
	if err = ray.NewRayJobReconciler(mgr).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RayJob")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
