package kube

import (
	"context"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"time"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	"github.com/v3io/scaler/pkg/ingresscache"
)

// IngressWatcher watches for changes in Kubernetes Ingress resources and updates the ingress cache accordingly
type IngressWatcher struct {
	ctx          context.Context
	logger       logger.Logger
	ingressCache ingresscache.IngressHostCache
	factory      informers.SharedInformerFactory
	informer     cache.SharedIndexInformer
}

func NewIngressWatcher(
	ctx context.Context,
	dlxLogger logger.Logger,
	kubeClient kubernetes.Interface,
	namespace, labelsFilter string,
) (*IngressWatcher, error) {
	factory := informers.NewSharedInformerFactoryWithOptions(
		kubeClient,
		30*time.Second, //TODO - consider make if configurable or const
		informers.WithNamespace(namespace),
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.LabelSelector = labelsFilter
		}),
	)
	ingressInformer := factory.Networking().V1().Ingresses().Informer()

	ingressWatcher := &IngressWatcher{
		ctx:          ctx,
		logger:       dlxLogger,
		ingressCache: ingresscache.NewIngressCache(dlxLogger), //TODO - consider decouple the ingress cache init from the watcher
		factory:      factory,
		informer:     ingressInformer,
	}

	//TODO - need to add event handler here -> ingressInformer.AddEventHandler

	return ingressWatcher, nil
}

func (iw *IngressWatcher) Start() error {
	iw.logger.Debug("Starting IngressWatcher")
	iw.factory.Start(iw.ctx.Done())

	if !cache.WaitForCacheSync(iw.ctx.Done(), iw.informer.HasSynced) {
		return errors.New("failed to sync ingress cache")
	}

	return nil
}

func (iw *IngressWatcher) Stop() {
	iw.logger.Debug("Stopping IngressWatcher")
	iw.factory.Shutdown()
}

// --- internal methods ---

// extractIngressValuesFromIngressResource extracts the host, path, and function name from the ingress resource.
func (iw *IngressWatcher) extractIngressValuesFromIngressResource(obj interface{}) (string, string, string, error) {
	ingress, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return "", "", "", errors.New("Failed to cast object to Ingress")
	}

	//todo - add function to extract host, path and function name from ingress resource
	// For now, we just store WA
	host, path, function, err := iw.extractHostPathFunction(ingress)
	if err != nil {
		return "", "", "", errors.Wrap(err, "Failed to extract host, path and function from ingress")
	}

	return host, path, function, nil
}

func (iw *IngressWatcher) extractHostPathFunction(ingress *networkingv1.Ingress) (string, string, string, error) {
	return "", "", "", nil //todo - implement this function to extract host, path and function name from ingress resource
}
