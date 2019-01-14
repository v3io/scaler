package main

import (
	"flag"
	"os"

	"github.com/v3io/scaler/cmd/dlx/app"

	"github.com/nuclio/errors"
)

func main() {
	namespace := flag.String("namepsace", "", "Kubernetes namespace")
	targetNameHeader := flag.String("target-name-header", "", "Name of the header that holds information on target name")
	targetPathHeader := flag.String("target-path-header", "", "Name of the header that holds information on target path")
	targetPort := flag.Int("target-port", 0, "Name of the header that holds information on target port")
	listenAddress := flag.String("listen-address", ":8090", "Address to listen upon for http proxy")
	flag.Parse()

	if err := app.Run(*namespace, *targetNameHeader, *targetPathHeader, *targetPort, *listenAddress); err != nil {
		errors.PrintErrorStack(os.Stderr, err, 5)

		os.Exit(1)
	}
}
