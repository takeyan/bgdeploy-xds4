/*
Copyright 2021.

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

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	bgdeployv1alpha1 "bgdeploy-xds4/api/v1alpha1"
	"bgdeploy-xds4/controllers"
	//+kubebuilder:scaffold:imports

	// ### import for envoy control-plane START
	"context"
	//        "flag"
	//        "os"
	"fmt"
	"net/http" // HTTP Server

	// cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	// serverv3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	// testv3 "github.com/envoyproxy/go-control-plane/pkg/test/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	serverv3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	testv3 "github.com/envoyproxy/go-control-plane/pkg/test/v3"

	// "bgdeploy-xds4/example" // bgdeploy-xds4
	example2 "bgdeploy-xds4/xdshelper4" // bgdeploy-xds4
	// example "github.com/envoyproxy/go-control-plane/internal/example"  // envoyproxy/go-control-plane/internal/example/main/main.goを参照
	"github.com/google/uuid"
	// ### import for envoy control-plane END
)

// ### Global variables for envoy control-plane: START
var (
	l example2.Logger

	port     uint
	basePort uint
	mode     string

	nodeID string

	upstreamHostname string = "localhost"
	snapshotVersion  string = "1"

	cache    cachev3.SnapshotCache
	snapshot cachev3.Snapshot
)

// ### Global variables for envoy control-plane: END

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(bgdeployv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	// ### start envoy control-plane first: START
	go xds()
	// ### start envoy control-plane first: END
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		//                LeaderElection:         enableLeaderElection,
		LeaderElection:             false,
		LeaderElectionID:           "773ff1db.swallowlab.com",
		LeaderElectionResourceLock: "configmaps",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.BGDeployReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BGDeploy")
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

// ### functions for envoy control-plane: START

func xds() {

	// copied from the "func init()"" of envoy control-plane
	l = example2.Logger{}

	flag.BoolVar(&l.Debug, "debug", false, "Enable xDS server debug logging")

	// The port that this xDS server listens on
	flag.UintVar(&port, "port", 18000, "xDS management server port")

	// Tell Envoy to use this Node ID
	flag.StringVar(&nodeID, "nodeID", "test-id", "Node ID")
	// end of copy

	flag.Parse()

	// Create a cache
	cache = cachev3.NewSnapshotCache(false, cachev3.IDHash{}, l)

	// Create the snapshot that we'll serve to Envoy
	snapshot = example2.GenerateSnapshot2(upstreamHostname, "80", snapshotVersion)
	if err := snapshot.Consistent(); err != nil {
		l.Errorf("snapshot inconsistency: %+v\n%+v", snapshot, err)
		os.Exit(1)
	}
	l.Debugf("will serve snapshot %+v", snapshot)

	// Add the snapshot to the cache
	if err := cache.SetSnapshot(nodeID, snapshot); err != nil {
		l.Errorf("snapshot error %q for %+v", err, snapshot)
		os.Exit(1)
	}

	// Run HTTP server to switch the target host
	http.HandleFunc("/xds", changeHost)
	go http.ListenAndServe(":18080", nil)

	// Run the xDS server
	ctx := context.Background()
	cb := &testv3.Callbacks{Debug: l.Debug}
	srv := serverv3.NewServer(ctx, cache, cb)
	example2.RunServer(ctx, srv, port)

}

func changeHost(w http.ResponseWriter, r *http.Request) {

	host := r.URL.Query().Get("host")
	port := r.URL.Query().Get("port")
	fmt.Fprint(w, "Change the target server to ", host, ":", port, "\n")

	u, _ := uuid.NewRandom()
	snapshot = example2.GenerateSnapshot2(host, port, u.String())
	cache.SetSnapshot(nodeID, snapshot)

}

// ### functions for envoy control-plane: END
