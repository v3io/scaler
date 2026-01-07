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

	"github.com/v3io/scaler/pkg/autoscaler"
	"github.com/v3io/scaler/pkg/autoscaler/metricsclient"
	"github.com/v3io/scaler/pkg/common"
	"github.com/v3io/scaler/pkg/pluginloader"
	"github.com/v3io/scaler/pkg/scalertypes"

	"github.com/nuclio/errors"
	"github.com/nuclio/zap"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
)

func Run(kubeconfigPath string,
	namespace string,
	scaleInterval time.Duration,
	metricsResourceKind string,
	metricsResourceGroup string) error {
	autoScalerOptions := scalertypes.AutoScalerOptions{
		Namespace:     namespace,
		ScaleInterval: scalertypes.Duration{Duration: scaleInterval},
		GroupKind: schema.GroupKind{
			Kind:  metricsResourceKind,
			Group: metricsResourceGroup,
		},
		MetricsClientOptions: scalertypes.MetricsClientOptions{
			Kind: scalertypes.KindCustomMetrics,
		},
	}

	pluginLoader, err := pluginloader.New()
	if err != nil {
		return errors.Wrap(err, "Failed to initialize plugin loader")
	}

	resourceScaler, err := pluginLoader.Load(kubeconfigPath, namespace)
	if err != nil {
		return errors.Wrap(err, "Failed to load plugin")
	}

	resourceScalerConfig, err := resourceScaler.GetConfig()
	if err != nil {
		return errors.Wrap(err, "Failed to get resource scaler config")
	}

	if resourceScalerConfig != nil {
		kubeconfigPath = resourceScalerConfig.KubeconfigPath
		autoScalerOptions = resourceScalerConfig.AutoScalerOptions
	}

	restConfig, err := common.GetClientConfig(kubeconfigPath)
	if err != nil {
		return errors.Wrap(err, "Failed to get client configuration")
	}

	newScaler, err := createAutoScaler(restConfig, resourceScaler, autoScalerOptions)
	if err != nil {
		return errors.Wrap(err, "Failed to create scaler")
	}

	if err = newScaler.Start(); err != nil {
		return errors.Wrap(err, "Failed to start scaler")
	}

	select {}
}

func createAutoScaler(restConfig *rest.Config,
	resourceScaler scalertypes.ResourceScaler,
	options scalertypes.AutoScalerOptions) (*autoscaler.Autoscaler, error) {
	rootLogger, err := nucliozap.NewNuclioZap("scaler",
		"console",
		nil,
		os.Stdout,
		os.Stderr,
		nucliozap.DebugLevel)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to initialize root logger")
	}

	// create metrics client using factory
	metricsClient, err := metricsclient.NewMetricsClient(rootLogger, restConfig, options)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create metrics client")
	}

	// create auto scaler
	newScaler, err := autoscaler.NewAutoScaler(rootLogger, resourceScaler, metricsClient, options)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create auto scaler")
	}

	return newScaler, nil
}
