/*
Copyright 2019 Iguazio Systems Ltd.

Licensed under the Apache License, Version 2.0 (the "License") with
an addition restriction as set forth herein. You may not use this
file except in compliance with the License. You may obtain a copy of
the License at http://www.apache.org/licenses/LICENSE-2.0.

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
implied. See the License for the specific language governing
permissions and limitations under the License.

In addition, you may not use the software for any purposes that are
illegal under applicable law, and the grant of the foregoing license
under the Apache 2.0 license is conditioned upon your compliance with
such restriction.
*/

package app

import (
	"os"
	"time"

	"github.com/v3io/scaler/pkg/common"
	"github.com/v3io/scaler/pkg/dlx"
	"github.com/v3io/scaler/pkg/pluginloader"
	"github.com/v3io/scaler/pkg/scalertypes"

	"github.com/nuclio/errors"
	"github.com/nuclio/zap"
	"k8s.io/client-go/kubernetes"
)

func Run(kubeconfigPath string,
	namespace string,
	targetNameHeader string,
	targetPathHeader string,
	targetPort int,
	listenAddress string,
	resourceReadinessTimeout string,
	multiTargetStrategy string) error {
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

	dlxOptions := scalertypes.DLXOptions{
		TargetNameHeader:         targetNameHeader,
		TargetPathHeader:         targetPathHeader,
		TargetPort:               targetPort,
		ListenAddress:            listenAddress,
		Namespace:                namespace,
		ResourceReadinessTimeout: scalertypes.Duration{Duration: resourceReadinessTimeoutDuration},
		MultiTargetStrategy:      scalertypes.MultiTargetStrategy(multiTargetStrategy),
	}

	// see if resource scaler wants to override the arguments
	resourceScalerConfig, err := resourceScaler.GetConfig()
	if err != nil {
		return errors.Wrap(err, "Failed to get resource scaler config")
	}

	if resourceScalerConfig != nil {
		dlxOptions = resourceScalerConfig.DLXOptions
	}

	restConfig, err := common.GetClientConfig(kubeconfigPath)
	if err != nil {
		return errors.Wrap(err, "Failed to get client configuration")
	}

	if dlxOptions.KubeClientSet, err = kubernetes.NewForConfig(restConfig); err != nil {
		return errors.Wrap(err, "Failed to create k8s client set")
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

func createDLX(
	resourceScaler scalertypes.ResourceScaler,
	options scalertypes.DLXOptions,
) (*dlx.DLX, error) {
	rootLogger, err := nucliozap.NewNuclioZap("scaler",
		"console",
		nil,
		os.Stdout,
		os.Stderr,
		nucliozap.DebugLevel)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to initialize root logger")
	}

	newScaler, err := dlx.NewDLX(rootLogger, resourceScaler, options)

	if err != nil {
		return nil, err
	}

	return newScaler, nil
}
