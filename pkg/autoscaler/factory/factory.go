/*
Copyright 2026 Iguazio Systems Ltd.

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

package factory

import (
	"github.com/v3io/scaler/pkg/autoscaler/metricsclients"
	"github.com/v3io/scaler/pkg/scalertypes"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	"k8s.io/client-go/rest"
)

func NewMetricsClient(logger logger.Logger, restConfig *rest.Config, autoScalerConf scalertypes.AutoScalerOptions) (scalertypes.MetricsClient, error) {
	switch autoScalerConf.MetricClientOptions.Kind {
	case scalertypes.KindCustomMetrics:
		customMetricsClient, err := metricsclients.NewCustomMetricsClientFromConfig(restConfig)
		if err != nil {
			return nil, errors.Wrap(err, "Failed to create custom metrics client")
		}
		return metricsclients.NewCustomMetricsWrapper(
			logger.GetChild("customMetricsClient"),
			customMetricsClient,
			autoScalerConf.Namespace,
			autoScalerConf.GroupKind), nil
	default:
		return nil, errors.New("unsupported metrics client kind")
	}
}
