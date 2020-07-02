package main

import (
	"flag"
	"os"
	"time"

	"github.com/v3io/scaler/cmd/autoscaler/app"
	"github.com/v3io/scaler/pkg/common"

	"github.com/nuclio/errors"
)

func main() {
	kubeconfigPath := flag.String("kubeconfig-path", os.Getenv("KUBECONFIG"), "Path of kubeconfig file")
	namespace := flag.String("namespace", "", "Namespace to listen on, or * for all")
	scaleInterval := flag.Duration("scale-interval", time.Minute, "Interval to call check scale function")
	metricsGroupName := flag.String("metrics-group-name", "", "Metrics resource group name")
	metricsGroupKind := flag.String("metrics-group-kind", "", "Metrics resource group kind")
	flag.Parse()

	*namespace = common.GetNamespace(*namespace)

	if err := app.Run(*kubeconfigPath,
		*namespace,
		*scaleInterval,
		*metricsGroupKind,
		*metricsGroupName); err != nil {
		errors.PrintErrorStack(os.Stderr, err, 5)

		os.Exit(1)
	}
}
