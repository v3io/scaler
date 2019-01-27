package app

import (
	"os"
	"time"

	"github.com/v3io/scaler/pkg/autoscaler"
	"github.com/v3io/scaler/pkg/pluginloader"

	"github.com/nuclio/errors"
	"github.com/nuclio/zap"
	"github.com/v3io/scaler-types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	custommetricsv1 "k8s.io/metrics/pkg/client/custom_metrics"
)

func Run(kubeconfigPath string,
	namespace string,
	scaleInterval time.Duration,
	scaleWindow time.Duration,
	metricName string,
	scaleThreshold int64,
	metricsInterval time.Duration) error {
	autoScalerOptions := scaler_types.AutoScalerOptions{
		Namespace:     namespace,
		ScaleInterval: scaleInterval,
		ScaleWindow:   scaleWindow,
		Threshold:     scaleThreshold,
		MetricName:    metricName,
	}

	pollerOptions := scaler_types.PollerOptions{
		Namespace:      namespace,
		MetricName:     metricName,
		MetricInterval: metricsInterval,
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
		pollerOptions = resourceScalerConfig.PollerOptions
	}

	restConfig, err := getClientConfig(kubeconfigPath)
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

	newPoller, err := createPoller(restConfig, newScaler, pollerOptions)
	if err != nil {
		return errors.Wrap(err, "Failed to create poller")
	}

	if err = newScaler.Start(); err != nil {
		return errors.Wrap(err, "Failed to start scaler")
	}

	if err := newPoller.Start(); err != nil {
		return errors.Wrap(err, "Failed to start poller")
	}

	select {}
}

func createAutoScaler(restConfig *rest.Config,
	resourceScaler scaler_types.ResourceScaler,
	options scaler_types.AutoScalerOptions) (*autoscaler.Autoscaler, error) {
	rootLogger, err := nucliozap.NewNuclioZap("autoscaler", "console", os.Stdout, os.Stderr, nucliozap.DebugLevel)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to initialize root logger")
	}

	newScaler, err := autoscaler.NewAutoScaler(rootLogger, resourceScaler, options)

	if err != nil {
		return nil, err
	}

	return newScaler, nil
}

func createPoller(restConfig *rest.Config,
	reporter autoscaler.MetricReporter,
	options scaler_types.PollerOptions) (*autoscaler.MetricsPoller, error) {
	rootLogger, err := nucliozap.NewNuclioZap("autoscaler", "console", os.Stdout, os.Stderr, nucliozap.DebugLevel)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to initialize root logger")
	}

	customMetricsClient, err := custommetricsv1.NewForConfig(restConfig)
	if err != nil {
		return nil, errors.Wrap(err, "Failed create custom metrics client set")
	}

	newPoller, err := autoscaler.NewMetricsPoller(rootLogger, reporter, customMetricsClient, options)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create metrics poller")
	}

	return newPoller, err
}

func getClientConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}

	return rest.InClusterConfig()
}
