package app

import (
	"os"

	"github.com/v3io/scaler/pkg"
	"github.com/v3io/scaler/pkg/dlx"
	"github.com/v3io/scaler/pkg/resourcescaler"

	"github.com/nuclio/errors"
	"github.com/nuclio/zap"
)

func Run(namespace string, targetNameHeader string, targetPathHeader string, targetPort int, listenAddress string) error {
	resourceScaler := resourcescaler.New()

	dlxOptions := scaler.DLXOptions{
		TargetNameHeader: targetNameHeader,
		TargetPathHeader: targetPathHeader,
		TargetPort:       targetPort,
		ListenAddress:    listenAddress,
		Namespace:        namespace,
		ResourceScaler:   resourceScaler,
	}

	// see if resource scaler wants to override the arguments
	resourceScalerConfig, err := resourceScaler.GetConfig()
	if err != nil {
		return errors.Wrap(err, "Failed to get resource scaler config")
	}

	if resourceScalerConfig != nil {
		dlxOptions = resourceScalerConfig.DLXOptions
	}

	newDLX, err := createDLX(dlxOptions)
	if err != nil {
		return errors.Wrap(err, "Failed to create dlx")
	}

	// start the scaler
	if err := newDLX.Start(); err != nil {
		return errors.Wrap(err, "Failed to start dlx")
	}

	select {}
}

func createDLX(options scaler.DLXOptions) (*dlx.DLX, error) {
	rootLogger, err := nucliozap.NewNuclioZap("dlx", "console", os.Stdout, os.Stderr, nucliozap.DebugLevel)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to initialize root logger")
	}

	newScaler, err := dlx.NewDLX(rootLogger, options)

	if err != nil {
		return nil, err
	}

	return newScaler, nil
}
