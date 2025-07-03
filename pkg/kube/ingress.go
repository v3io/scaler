/*
Copyright 2025 Iguazio Systems Ltd.

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

package kube

import (
	"context"
	"time"

	"github.com/v3io/scaler/pkg/ingresscache"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type ResolveTargetsFromIngressCallback func(ingress *networkingv1.Ingress) ([]string, error)

// IngressWatcher watches for changes in Kubernetes Ingress resources and updates the ingress cache accordingly
type IngressWatcher struct {
	ctx                    context.Context
	logger                 logger.Logger
	ingressCache           ingresscache.IngressHostCache
	factory                informers.SharedInformerFactory
	informer               cache.SharedIndexInformer
	resolveTargetsCallback ResolveTargetsFromIngressCallback
}

func NewIngressWatcher(
	ctx context.Context,
	dlxLogger logger.Logger,
	kubeClient kubernetes.Interface,
	ingressCache *ingresscache.IngressCache,
	resolveCallback ResolveTargetsFromIngressCallback,
	namespace, labelsFilter string,
) (*IngressWatcher, error) {
	factory := informers.NewSharedInformerFactoryWithOptions(
		kubeClient,
		30*time.Second,
		informers.WithNamespace(namespace),
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.LabelSelector = labelsFilter
		}),
	)
	ingressInformer := factory.Networking().V1().Ingresses().Informer()

	ingressWatcher := &IngressWatcher{
		ctx:                    ctx,
		logger:                 dlxLogger,
		ingressCache:           ingressCache,
		factory:                factory,
		informer:               ingressInformer,
		resolveTargetsCallback: resolveCallback,
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
	host, path, targets, err := iw.extractIngressValuesFromIngressResource(obj)
	if err != nil {
		iw.logger.WarnWith("Add ingress handler failure", "error", err)
		return
	}

	if err = iw.ingressCache.Set(host, path, targets); err != nil {
		iw.logger.WarnWith("Add ingress handler failure- failed to add the new value", "error", err, "object", obj)
		return
	}
}

func (iw *IngressWatcher) IngressHandlerUpdateFunc(oldObj, newObj interface{}) {
	oldHost, oldPath, oldTargets, err := iw.extractIngressValuesFromIngressResource(oldObj)
	if err != nil {
		iw.logger.WarnWith("Update ingress handler - failed to extract values from old object", "error", err)
		return
	}

	newHost, newPath, newTargets, err := iw.extractIngressValuesFromIngressResource(newObj)
	if err != nil {
		iw.logger.WarnWith("Update ingress handler - failed to extract values from new object", "error", err)
		return
	}

	// if the host or path has changed, we need to delete the old entry
	if oldHost != newHost || oldPath != newPath {
		if err = iw.ingressCache.Delete(oldHost, oldPath, oldTargets); err != nil {
			iw.logger.WarnWith("Update ingress handler failure - failed to delete old ingress", "error", err)
		}
	}

	if err = iw.ingressCache.Set(newHost, newPath, newTargets); err != nil {
		iw.logger.WarnWith("Update ingress handler failure- failed to add the new value", "error", err, "object", newObj)
		return
	}
}

func (iw *IngressWatcher) IngressHandlerDeleteFunc(obj interface{}) {
	host, path, targets, err := iw.extractIngressValuesFromIngressResource(obj)
	if err != nil {
		iw.logger.WarnWith("Delete ingress handler failure- failed to extract values from object", "error", err)
		return
	}

	if err = iw.ingressCache.Delete(host, path, targets); err != nil {
		iw.logger.WarnWith("Delete ingress handler failure- failed delete from cache", "error", err, "object", obj)
		return
	}
}

// --- internal methods ---

// extractIngressValuesFromIngressResource extracts the host, path, and targets from the ingress resource.
func (iw *IngressWatcher) extractIngressValuesFromIngressResource(obj interface{}) (string, string, []string, error) {
	ingress, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return "", "", nil, errors.New("Failed to cast object to Ingress")
	}

	host, path, targets, err := iw.extractHostPathTarget(ingress)
	if err != nil {
		return "", "", nil, errors.Wrap(err, "Failed to extract host, path and targets from ingress")
	}

	return host, path, targets, nil
}

// extractHostPathTarget extracts the host, path, and targets from the ingress resource
func (iw *IngressWatcher) extractHostPathTarget(ingress *networkingv1.Ingress) (string, string, []string, error) {
	host, err := iw.getHostFromIngress(ingress)
	if err != nil {
		return "", "", nil, errors.Wrap(err, "Failed to extract host from ingress")
	}

	path, err := iw.getPathFromIngress(ingress)
	if err != nil {
		return "", "", nil, errors.Wrap(err, "Failed to extract path from ingress")
	}

	targets, err := iw.resolveTargetsCallback(ingress)
	if err != nil {
		return "", "", nil, errors.Wrap(err, "Failed to extract targets from ingress labels")
	}

	return host, path, targets, nil
}

func (iw *IngressWatcher) getHostFromIngress(ingress *networkingv1.Ingress) (string, error) {
	if ingress == nil {
		return "", errors.New("ingress is nil")
	}

	if len(ingress.Spec.Rules) == 0 {
		return "", errors.New("no rules found in ingress")
	}

	// Ingress must contain exactly one rule
	rule := ingress.Spec.Rules[0]
	if rule.Host == "" {
		return "", errors.New("host is empty in ingress rule")
	}

	return rule.Host, nil
}

func (iw *IngressWatcher) getPathFromIngress(ingress *networkingv1.Ingress) (string, error) {
	if ingress == nil {
		return "", errors.New("ingress is nil")
	}

	if len(ingress.Spec.Rules) == 0 {
		return "", errors.New("no rules found in ingress")
	}

	rule := ingress.Spec.Rules[0]
	if rule.HTTP == nil {
		return "", errors.New("no HTTP configuration found in ingress rule")
	}

	if len(rule.HTTP.Paths) == 0 {
		return "", errors.New("no paths found in ingress HTTP rule")
	}

	// Ingress must contain exactly one rule
	httpPath := rule.HTTP.Paths[0]
	if httpPath.Path == "" {
		return "", errors.New("path is empty in ingress HTTP rule")
	}

	return httpPath.Path, nil
}
