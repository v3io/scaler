package app

import (
	"os"
	"time"

	"github.com/v3io/scaler/pkg/autoscaler"
	"github.com/v3io/scaler/pkg/pluginloader"

	"github.com/nuclio/errors"
	"github.com/nuclio/zap"
	"github.com/v3io/scaler-types"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/metrics/pkg/client/custom_metrics"
)

func Run(kubeconfigPath string,
	namespace string,
	scaleInterval time.Duration,
	metricsResourceKind string,
	metricsResourceGroup string) error {
	autoScalerOptions := scaler_types.AutoScalerOptions{
		Namespace:     namespace,
		ScaleInterval: scaler_types.Duration{Duration: scaleInterval},
		GroupKind: schema.GroupKind{
			Kind:  metricsResourceKind,
			Group: metricsResourceGroup,
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

	select {}
}

func createAutoScaler(restConfig *rest.Config,
	resourceScaler scaler_types.ResourceScaler,
	options scaler_types.AutoScalerOptions) (*autoscaler.Autoscaler, error) {
	rootLogger, err := nucliozap.NewNuclioZap("scaler",
		"console",
		nil,
		os.Stdout,
		os.Stderr,
		nucliozap.DebugLevel)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to initialize root logger")
	}

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create discovery client")
	}
	availableAPIsGetter := custom_metrics.NewAvailableAPIsGetter(discoveryClient)
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient))
	customMetricsClient := custom_metrics.NewForConfig(restConfig, restMapper, availableAPIsGetter)

	// create auto scaler
	newScaler, err := autoscaler.NewAutoScaler(rootLogger, resourceScaler, customMetricsClient, options)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create auto scaler")
	}

	return newScaler, nil
}

func getClientConfig(kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}

	return rest.InClusterConfig()
}
