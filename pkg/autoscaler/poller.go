package autoscaler

import (
	"time"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	"github.com/v3io/scaler-types"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	custommetricsv1 "k8s.io/metrics/pkg/client/custom_metrics"
)

type MetricsPoller struct {
	logger                 logger.Logger
	customMetricsClientSet custommetricsv1.CustomMetricsClient
	metricReporter         MetricReporter
	ticker                 *time.Ticker
	namespace              string
	metricName             string
	groupKind              string
}

func NewMetricsPoller(parentLogger logger.Logger,
	metricReporter MetricReporter,
	customMetricsClientSet custommetricsv1.CustomMetricsClient,
	options scaler_types.PollerOptions) (*MetricsPoller, error) {
	var err error

	loggerInstance := parentLogger.GetChild("metrics")
	loggerInstance.DebugWith("Creating metrics poller",
		"namespace", options.Namespace,
		"metricName", options.MetricName)

	ticker := time.NewTicker(options.MetricInterval)

	newMetricsOperator := &MetricsPoller{
		logger:                 loggerInstance,
		customMetricsClientSet: customMetricsClientSet,
		metricReporter:         metricReporter,
		ticker:                 ticker,
		namespace:              options.Namespace,
		metricName:             options.MetricName,
		groupKind:              options.GroupKind,
	}
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create project operator")
	}

	return newMetricsOperator, nil
}

func (mp *MetricsPoller) getResourceMetrics() error {
	schemaGroupKind := schema.GroupKind{Group: "", Kind: mp.groupKind}
	resourceLabels := labels.Everything()
	c := mp.customMetricsClientSet.NamespacedMetrics(mp.namespace)
	cm, err := c.
		GetForObjects(schemaGroupKind,
			resourceLabels,
			mp.metricName)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		} else {
			return errors.Wrap(err, "Failed to get custom metrics")
		}
	}

	for _, item := range cm.Items {

		mp.logger.DebugWith("Publishing new metric",
			"resource", item.DescribedObject.Name,
			"value", item.Value.MilliValue())
		newEntry := metricEntry{
			timestamp:    time.Now(),
			value:        item.Value.MilliValue(),
			resourceName: scaler_types.Resource(item.DescribedObject.Name),
			metricName:   mp.metricName,
		}
		err := mp.metricReporter.ReportMetric(newEntry)
		if err != nil {
			return errors.Wrap(err, "Failed to report metric")
		}
	}
	return nil
}

func (mp *MetricsPoller) Start() error {
	go func() {
		for range mp.ticker.C {
			err := mp.getResourceMetrics()
			if err != nil {
				mp.logger.WarnWith("Failed to get resource metrics", "err", err)
			}
		}
	}()

	return nil
}
