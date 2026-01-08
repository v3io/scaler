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
	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8scustommetrics "k8s.io/metrics/pkg/client/custom_metrics"
)

type K8sCustomMetricsClient struct {
	k8scustommetrics.CustomMetricsClient
	namespace string
	groupKind schema.GroupKind
	logger    logger.Logger
}

func NewCustomMetricsClient(
	parentLogger logger.Logger,
	customMetricsClient k8scustommetrics.CustomMetricsClient,
	namespace string,
	groupKind schema.GroupKind) *K8sCustomMetricsClient {
	return &K8sCustomMetricsClient{
		logger:              parentLogger.GetChild("custom-metrics"),
		CustomMetricsClient: customMetricsClient,
		namespace:           namespace,
		groupKind:           groupKind,
	}
}

func (cmw *K8sCustomMetricsClient) GetResourceMetrics(metricNames []string) (map[string]map[string]int, error) {
	resourcesMetricsMap := make(map[string]map[string]int)
	resourceLabels := labels.Everything()
	metricSelectorLabels := labels.Everything()
	metricsClient := cmw.NamespacedMetrics(cmw.namespace)

	for _, metricName := range metricNames {
		// getting the metric values for all object of schema group kind (e.g. deployment)
		metrics, err := metricsClient.GetForObjects(cmw.groupKind, resourceLabels, metricName, metricSelectorLabels)
		if err != nil {
			// No data points have been submitted yet; this is expected—proceed to the next metric.
			if k8serrors.IsNotFound(err) {
				continue
			}
			return nil, errors.Wrap(err, "Failed to get custom metrics")
		}

		// fill the resourcesMetricsMap with the metrics data we got
		for _, item := range metrics.Items {
			resourceName := item.DescribedObject.Name
			value := int(item.Value.MilliValue())

			cmw.logger.DebugWith("Got metric entry",
				"resourceName", resourceName,
				"metricName", metricName,
				"value", value)

			if _, found := resourcesMetricsMap[resourceName]; !found {
				resourcesMetricsMap[resourceName] = make(map[string]int)
			}

			// sanity
			if _, found := resourcesMetricsMap[resourceName][metricName]; found {
				return nil, errors.New("Can not have more than one metric value per resource")
			}

			resourcesMetricsMap[resourceName][metricName] = value
		}
	}

	return resourcesMetricsMap, nil
}
