package app

import (
	"os"
	"time"

	"github.com/v3io/scaler/pkg"
	"github.com/v3io/scaler/pkg/autoscaler"
	"github.com/v3io/scaler/pkg/resourcescaler"

	"github.com/nuclio/errors"
	"github.com/nuclio/zap"
	"k8s.io/client-go/kubernetes"
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
	autoScalerOptions := scaler.AutoScalerOptions{
		Namespace:     namespace,
		ScaleInterval: scaleInterval,
		ScaleWindow:   scaleWindow,
		Threshold:     scaleThreshold,
		MetricName:    metricName,
	}

	pollerOptions := scaler.PollerOptions{
		Namespace:      namespace,
		MetricName:     metricName,
		MetricInterval: metricsInterval,
	}

	resourceScaler := resourcescaler.New()

	resourceScalerConfig, err := resourceScaler.GetConfig()
	if err != nil {
		return errors.Wrap(err, "Failed to get resource scaler config")
	}

	if resourceScalerConfig != nil {
		kubeconfigPath = resourceScalerConfig.KubeconfigPath
		autoScalerOptions = resourceScalerConfig.AutoScalerOptions
		pollerOptions = resourceScalerConfig.PollerOptions
	}

	autoScalerOptions.ResourceScaler = resourceScaler
	pollerOptions.ResourceScaler = resourceScaler

	restConfig, err := getClientConfig(kubeconfigPath)
	if err != nil {
		return errors.Wrap(err, "Failed to get client configuration")
	}

	newScaler, err := createAutoScaler(restConfig, autoScalerOptions)
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

func createAutoScaler(restConfig *rest.Config, options scaler.AutoScalerOptions) (*autoscaler.Autoscaler, error) {
	rootLogger, err := nucliozap.NewNuclioZap("autoscaler", "console",nil, os.Stdout, os.Stderr, nucliozap.DebugLevel)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to initialize root logger")
	}

	kubeClientSet, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create k8s client set")
	}

	options.KubeClientSet = kubeClientSet

	newScaler, err := autoscaler.NewAutoScaler(rootLogger, options)

	if err != nil {
		return nil, err
	}

	return newScaler, nil
}

func createPoller(restConfig *rest.Config, reporter autoscaler.MetricReporter, options scaler.PollerOptions) (*autoscaler.MetricsPoller, error) {
	rootLogger, err := nucliozap.NewNuclioZap("autoscaler", "console", nil,os.Stdout, os.Stderr, nucliozap.DebugLevel)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to initialize root logger")
	}

	customMetricsClient, err := custommetricsv1.NewForConfig(restConfig)
	if err != nil {
		return nil, errors.Wrap(err, "Failed create custom metrics client set")
	}

	options.CustomMetricsClientSet = customMetricsClient

	newPoller, err := autoscaler.NewMetricsPoller(rootLogger, reporter, options)
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
