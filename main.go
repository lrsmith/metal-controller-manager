/*
Copyright 2020 Talos Systems, Inc.
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
	"fmt"
	"os"

	metalv1alpha1 "github.com/talos-systems/metal-controller-manager/api/v1alpha1"
	"github.com/talos-systems/metal-controller-manager/controllers"
	"github.com/talos-systems/metal-controller-manager/internal/ipxe"
	"github.com/talos-systems/metal-controller-manager/internal/tftp"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = metalv1alpha1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var discoveryKubeconfig string
	flag.StringVar(&metricsAddr, "metrics-addr", ":8081", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false, "Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&discoveryKubeconfig, "discovery-kubeconfig", "", "The URL for the kubeconfig used by the discovery agent to add a server to the inventory.")

	flag.Parse()

	ctrl.SetLogger(zap.New(func(o *zap.Options) {
		o.Development = true
	}))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "controller-leader-election-metal-controller-manager",
		Port:               9443,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.EnvironmentReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Environment"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr, controller.Options{MaxConcurrentReconciles: 10}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Environment")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting TFTP server")

	go func() {
		if err := tftp.ServeTFTP(); err != nil {
			setupLog.Error(err, "unable to start TFTP server", "controller", "Environment")
		}
	}()

	setupLog.Info("starting iPXE server")

	go func() {
		discoeryNamespace, ok := os.LookupEnv("NAMESPACE")
		if !ok {
			setupLog.Error(fmt.Errorf("missing NAMESPACE environment variable"), "unable to start iPXE server", "controller", "Environment")
		}

		if err := ipxe.ServeIPXE(discoveryKubeconfig, discoeryNamespace); err != nil {
			setupLog.Error(err, "unable to start iPXE server", "controller", "Environment")
		}
	}()

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
