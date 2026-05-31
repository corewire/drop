/*
Copyright (c) 2026 Breee

SPDX-License-Identifier: MIT
*/

// drop-ui is a standalone web server that serves the Drop Control Center UI.
// It connects to the Kubernetes API to read drop.corewire.io CRDs and
// exposes a REST + SSE API consumed by the browser.
//
// Usage:
//
//	drop-ui --bind-address :8888
//	drop-ui --kubeconfig ~/.kube/config --bind-address :8888
package main

import (
	"flag"
	"net/http"
	"os"
	"time"

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	dropv1alpha1 "github.com/Breee/drop/api/v1alpha1"
	"github.com/Breee/drop/internal/ui"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(dropv1alpha1.AddToScheme(scheme))
}

func main() {
	var bindAddr string
	var pollInterval time.Duration

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.StringVar(&bindAddr, "bind-address", ":8888", "Address the UI server listens on.")
	flag.DurationVar(&pollInterval, "poll-interval", 10*time.Second, "How often to poll Kubernetes for live SSE updates.")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	logger := ctrl.Log.WithName("drop-ui")

	cfg, err := ctrl.GetConfig()
	if err != nil {
		logger.Error(err, "failed to get kubeconfig")
		os.Exit(1)
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		logger.Error(err, "failed to create Kubernetes client")
		os.Exit(1)
	}

	srv := ui.NewServer(c, pollInterval)
	httpSrv := &http.Server{
		Addr:         bindAddr,
		Handler:      srv.Handler(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 0, // SSE streams need no write timeout
		IdleTimeout:  120 * time.Second,
	}

	logger.Info("Drop Control Center UI starting", "address", bindAddr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error(err, "server exited with error")
		os.Exit(1)
	}
}
