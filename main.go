/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"flag"
	"os"
	"sync"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	observabilityv1alpha1 "github.com/kaasops/vector-operator/api/v1alpha1"
	"github.com/kaasops/vector-operator/controllers"
	"github.com/kaasops/vector-operator/controllers/factory/utils/k8s"
	monitorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(observabilityv1alpha1.AddToScheme(scheme))
	utilruntime.Must(monitorv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var namespace string
	var watchLabel string
	var pipelineCheckWG sync.WaitGroup
	var PipelineCheckTimeout time.Duration
	var PipelineDeleteEventTimeout time.Duration
	var ConfigCheckTimeout time.Duration

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&namespace, "watch-namespace", "", "Namespace to filter the list of watched objects")
	flag.StringVar(&watchLabel, "watch-name", "", "Filter the list of watched objects by checking the app.kubernetes.io/managed-by label")
	flag.DurationVar(&PipelineCheckTimeout, "pipeline-check-timeout", 15*time.Second, "wait pipeline checks before force vector reconcile. Default: 15s")
	flag.DurationVar(&PipelineDeleteEventTimeout, "pipeline-delete-timeout", 5*time.Second, "collect delete events timeout")
	flag.DurationVar(&ConfigCheckTimeout, "configcheck-timeout", 300*time.Second, "collect delete events timeout")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	config := ctrl.GetConfigOrDie()

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		setupLog.Error(err, "unable to create clientset")
		os.Exit(1)
	}

	dc, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		setupLog.Error(err, "unable to create discovery client")
		os.Exit(1)
	}

	mgrOptions := ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "79cbe7f3.kaasops.io",
	}
	customMgrOptions, err := setupCustomCache(&mgrOptions, namespace, watchLabel)

	if err != nil {
		setupLog.Error(err, "unable to set up custom cache settings")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(config, *customMgrOptions)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.VectorReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		Clientset:            clientset,
		PipelineCheckWG:      &pipelineCheckWG,
		PipelineCheckTimeout: PipelineCheckTimeout,
		ConfigCheckTimeout:   ConfigCheckTimeout,
		DiscoveryClient:      dc,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Vector")
		os.Exit(1)
	}
	if err = (&controllers.PipelineReconciler{
		Client:                     mgr.GetClient(),
		Scheme:                     mgr.GetScheme(),
		Clientset:                  clientset,
		PipelineCheckWG:            &pipelineCheckWG,
		PipelineDeleteEventTimeout: PipelineDeleteEventTimeout,
		ConfigCheckTimeout:         ConfigCheckTimeout,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VectorPipeline")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

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

func setupCustomCache(mgrOptions *ctrl.Options, namespace string, watchLabel string) (*ctrl.Options, error) {
	if namespace == "" && watchLabel == "" {
		return mgrOptions, nil
	}

	var namespaceSelector fields.Selector
	var labelSelector labels.Selector
	if namespace != "" {
		namespaceSelector = fields.Set{"metadata.namespace": namespace}.AsSelector()
	}
	if watchLabel != "" {
		labelSelector = labels.Set{k8s.ManagedByLabelKey: "vector-operator", k8s.NameLabelKey: watchLabel}.AsSelector()
	}

	selectorsByObject := cache.SelectorsByObject{
		&corev1.Pod{}: {
			Field: namespaceSelector,
			Label: labelSelector,
		},
		&appsv1.DaemonSet{}: {
			Field: namespaceSelector,
			Label: labelSelector,
		},
		&corev1.Service{}: {
			Field: namespaceSelector,
			Label: labelSelector,
		},
		&corev1.Secret{}: {
			Field: namespaceSelector,
			Label: labelSelector,
		},
		&corev1.ServiceAccount{}: {
			Field: namespaceSelector,
			Label: labelSelector,
		},
	}

	mgrOptions.NewCache = cache.BuilderWithOptions(cache.Options{SelectorsByObject: selectorsByObject})

	return mgrOptions, nil
}
