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

	if _, err := ingressInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    ingressWatcher.IngressHandlerAddFunc,
		UpdateFunc: ingressWatcher.IngressHandlerUpdateFunc,
		DeleteFunc: ingressWatcher.IngressHandlerDeleteFunc,
	}); err != nil {
		return nil, err
	}

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

// --- ResourceEventHandler methods ---

func (iw *IngressWatcher) IngressHandlerAddFunc(obj interface{}) {
	host, path, functions, err := iw.extractIngressValuesFromIngressResource(obj)
	if err != nil {
		iw.logger.WarnWith("Add ingress handler failure", "error", err)
		return
	}

	if err = iw.ingressCache.Set(host, path, functions); err != nil {
		iw.logger.WarnWith("Add ingress handler failure- failed to add the new value", "error", err, "object", obj)
		return
	}
}

func (iw *IngressWatcher) IngressHandlerUpdateFunc(oldObj, newObj interface{}) {
	oldHost, oldPath, oldFunctions, err := iw.extractIngressValuesFromIngressResource(oldObj)
	if err != nil {
		iw.logger.WarnWith("Update ingress handler - failed to extract values from old object", "error", err)
		return
	}

	newHost, newPath, newFunctions, err := iw.extractIngressValuesFromIngressResource(newObj)
	if err != nil {
		iw.logger.WarnWith("Update ingress handler - failed to extract values from new object", "error", err)
		return
	}

	//TODO - think on the scenarios here again TOMORROW
	// The current implementation is- remove the old ingress and add the new one if the host, path or function name has changed

	if err = iw.ingressCache.Delete(oldHost, oldPath, oldFunctions); err != nil {
		iw.logger.WarnWith("Update ingress handler failure - failed to delete old ingress", "error", err)
	}

	//TODO - need to see how the ingress is being used in the cache- see the diff or only new ingress
	if err = iw.ingressCache.Set(newHost, newPath, newFunctions); err != nil {
		iw.logger.WarnWith("Update ingress handler failure- failed to add the new value", "error", err, "object", newObj)
		return
	}
}

func (iw *IngressWatcher) IngressHandlerDeleteFunc(obj interface{}) {
	host, path, functions, err := iw.extractIngressValuesFromIngressResource(obj)
	if err != nil {
		iw.logger.WarnWith("Delete ingress handler failure- failed to extract values from object", "error", err)
		return
	}

	if err = iw.ingressCache.Delete(host, path, functions); err != nil {
		iw.logger.WarnWith("Delete ingress handler failure- failed delete from cache", "error", err, "object", obj)
		return
	}
}

// --- internal methods ---

// extractIngressValuesFromIngressResource extracts the host, path, and function name from the ingress resource.
func (iw *IngressWatcher) extractIngressValuesFromIngressResource(obj interface{}) (string, string, []string, error) {
	ingress, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return "", "", nil, errors.New("Failed to cast object to Ingress")
	}

	//todo - add function to extract host, path and function name from ingress resource
	// For now, we just store WA
	host, path, functions, err := iw.extractHostPathFunction(ingress)
	if err != nil {
		return "", "", nil, errors.Wrap(err, "Failed to extract host, path and function from ingress")
	}

	return host, path, functions, nil
}

func (iw *IngressWatcher) extractHostPathFunction(ingress *networkingv1.Ingress) (string, string, []string, error) {
	return "", "", nil, nil //todo - implement this function to extract host, path and function name from ingress resource
}
