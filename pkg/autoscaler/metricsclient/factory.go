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

package metricsclient

import (
	"github.com/v3io/scaler/pkg/scalertypes"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	"k8s.io/client-go/rest"
)

func NewMetricsClient(logger logger.Logger,
	restConfig *rest.Config,
	autoScalerConf scalertypes.AutoScalerOptions) (scalertypes.MetricsClient, error) {
	switch autoScalerConf.MetricsClientOptions.MetricsClientKind {
	case scalertypes.KindK8sMetricsClient:
		return NewCustomMetricsClient(
			logger,
			restConfig,
			autoScalerConf.Namespace,
			autoScalerConf.GroupKind)
	case scalertypes.KindPrometheusClient:
		return NewPrometheusClient(
			logger,
			autoScalerConf.MetricsClientOptions.URL,
			autoScalerConf.Namespace,
			autoScalerConf.MetricsClientOptions.QueryTemplates)
	default:
		return nil, errors.Errorf("unsupported metrics client kind: %s", autoScalerConf.MetricsClientOptions.MetricsClientKind)
	}
}
