package autoscaler

import (
	"github.com/v3io/scaler/pkg/common"
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
	resourceScaler         scaler_types.ResourceScaler
	pollingTicker          *time.Ticker
	reconfigureTicker      *time.Ticker
	namespace              string
	metricNames            []string
	groupKind              string
}

func NewMetricsPoller(parentLogger logger.Logger,
	metricReporter MetricReporter,
	customMetricsClientSet custommetricsv1.CustomMetricsClient,
	resourceScaler scaler_types.ResourceScaler,
	options scaler_types.PollerOptions) (*MetricsPoller, error) {
	var err error

	loggerInstance := parentLogger.GetChild("metrics")
	loggerInstance.DebugWith("Creating metrics poller",
		"namespace", options.Namespace,
		"options", options)

	pollingTicker := time.NewTicker(options.MetricInterval)
	reconfigureTicker := time.NewTicker(options.ReconfigureInterval)

	newMetricsOperator := &MetricsPoller{
		logger:                 loggerInstance,
		customMetricsClientSet: customMetricsClientSet,
		metricReporter:         metricReporter,
		resourceScaler:         resourceScaler,
		pollingTicker:          pollingTicker,
		reconfigureTicker:      reconfigureTicker,
		namespace:              options.Namespace,
		metricNames:            options.MetricNames,
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

	// Creating a copy cause reconfigure might happen while we're polling
	metricNames := append([]string(nil), mp.metricNames...)
	for _, metricName := range metricNames {
		cm, err := c.GetForObjects(schemaGroupKind,
			resourceLabels,
			metricName)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return nil
			}
			return errors.Wrap(err, "Failed to get custom metrics")
		}

		for _, item := range cm.Items {

			mp.logger.DebugWith("Publishing new metric",
				"resource", item.DescribedObject.Name,
				"value", item.Value.MilliValue())
			newEntry := metricEntry{
				timestamp:    time.Now(),
				value:        item.Value.MilliValue(),
				resourceName: item.DescribedObject.Name,
				metricName:   metricName,
			}
			err := mp.metricReporter.ReportMetric(newEntry)
			if err != nil {
				return errors.Wrap(err, "Failed to report metric")
			}
		}
	}
	return nil
}

func (mp *MetricsPoller) reconfigure() error {
	mp.logger.Debug("Reconfiguring poller")
	var metricNames []string
	resourcesList, err := mp.resourceScaler.GetResources()
	if err != nil {
		return errors.Wrap(err, "Failed to get resources")
	}
	for _, resource := range resourcesList {
		for _, metricName := range resource.MetricNames {
			metricNames = append(metricNames, metricName)
		}
	}
	mp.metricNames = common.UniquifyStringList(metricNames)
	mp.logger.DebugWith("Poller reconfigured", "metricNames", mp.metricNames)
	return nil
}

func (mp *MetricsPoller) Start() error {
	go func() {
		for range mp.pollingTicker.C {
			err := mp.getResourceMetrics()
			if err != nil {
				mp.logger.WarnWith("Failed to get resource metrics", "err", err)
			}
		}
	}()

	go func() {
		for range mp.reconfigureTicker.C {
			err := mp.reconfigure()
			if err != nil {
				mp.logger.WarnWith("Failed to reconfigure poller", "err", err)
			}
		}
	}()

	return nil
}
