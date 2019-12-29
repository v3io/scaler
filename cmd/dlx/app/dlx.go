package app

import (
	"os"
	"time"

	"github.com/v3io/scaler/pkg/dlx"
	"github.com/v3io/scaler/pkg/pluginloader"

	"github.com/nuclio/errors"
	"github.com/nuclio/zap"
	"github.com/v3io/scaler-types"
)

func Run(kubeconfigPath string,
	namespace string,
	targetNameHeader string,
	targetPathHeader string,
	targetPort int,
	listenAddress string,
	resourceReadinessTimeout string) error {
	pluginLoader, err := pluginloader.New()
	if err != nil {
		return errors.Wrap(err, "Failed to initialize plugin loader")
	}

	resourceScaler, err := pluginLoader.Load(kubeconfigPath, namespace)
	if err != nil {
		return errors.Wrap(err, "Failed to load plugin")
	}

	resourceReadinessTimeoutDuration, err := time.ParseDuration(resourceReadinessTimeout)
	if err != nil {
		return errors.Wrap(err, "Failed to parse resource readiness timeout")
	}

	dlxOptions := scaler_types.DLXOptions{
		TargetNameHeader:         targetNameHeader,
		TargetPathHeader:         targetPathHeader,
		TargetPort:               targetPort,
		ListenAddress:            listenAddress,
		Namespace:                namespace,
		ResourceReadinessTimeout: scaler_types.Duration{Duration: resourceReadinessTimeoutDuration},
	}

	// see if resource scaler wants to override the arguments
	resourceScalerConfig, err := resourceScaler.GetConfig()
	if err != nil {
		return errors.Wrap(err, "Failed to get resource scaler config")
	}

	if resourceScalerConfig != nil {
		dlxOptions = resourceScalerConfig.DLXOptions
	}

	newDLX, err := createDLX(resourceScaler, dlxOptions)
	if err != nil {
		return errors.Wrap(err, "Failed to create dlx")
	}

	// start the scaler
	if err := newDLX.Start(); err != nil {
		return errors.Wrap(err, "Failed to start dlx")
	}

	select {}
}

func createDLX(resourceScaler scaler_types.ResourceScaler, options scaler_types.DLXOptions) (*dlx.DLX, error) {
	rootLogger, err := nucliozap.NewNuclioZap("scaler", "console", os.Stdout, os.Stderr, nucliozap.DebugLevel)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to initialize root logger")
	}

	newScaler, err := dlx.NewDLX(rootLogger, resourceScaler, options)

	if err != nil {
		return nil, err
	}

	return newScaler, nil
}
