package autoscaler

import (
	"time"

	"github.com/v3io/scaler/pkg"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
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
	functionPodNameCache   map[string]string
}

func NewMetricsPoller(parentLogger logger.Logger, metricReporter MetricReporter, options scaler.PollerOptions) (*MetricsPoller, error) {
	var err error

	loggerInstance := parentLogger.GetChild("metrics")

	ticker := time.NewTicker(options.MetricInterval)

	newMetricsOperator := &MetricsPoller{
		logger:                 loggerInstance,
		customMetricsClientSet: options.CustomMetricsClientSet,
		metricReporter:         metricReporter,
		ticker:                 ticker,
		namespace:              options.Namespace,
		metricName:             options.MetricName,
		functionPodNameCache:   make(map[string]string),
	}
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create project operator")
	}

	return newMetricsOperator, nil
}

func (mp *MetricsPoller) getResourceMetrics() error {
	schemaGroupKind := schema.GroupKind{Group: "", Kind: "Function"}
	functionLabels := labels.Everything()
	c := mp.customMetricsClientSet.NamespacedMetrics(mp.namespace)
	cm, err := c.
		GetForObjects(schemaGroupKind,
			functionLabels,
			mp.metricName)
	if err != nil {
		return errors.Wrap(err, "Failed to get custom metrics")
	}

	for _, item := range cm.Items {

		mp.logger.DebugWith("Publishing new metric",
			"function", item.DescribedObject.Name,
			"value", item.Value.MilliValue())
		newEntry := metricEntry{
			timestamp:    time.Now(),
			value:        item.Value.MilliValue(),
			resourceName: scaler.Resource(item.DescribedObject.Name),
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
				mp.logger.WarnWith("Failed to get function metrics", "err", err)
			}
		}
	}()

	return nil
}
