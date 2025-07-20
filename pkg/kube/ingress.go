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

	"github.com/v3io/scaler/pkg/ingresscache"
	"github.com/v3io/scaler/pkg/scalertypes"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

type IngressValue struct {
	Name    string
	Host    string
	Path    string
	Targets []string
}

// IngressWatcher watches for changes in Kubernetes Ingress resources and updates the ingress cache accordingly
type IngressWatcher struct {
	ctx                    context.Context
	cancel                 context.CancelFunc
	logger                 logger.Logger
	cache                  ingresscache.IngressHostCache
	factory                informers.SharedInformerFactory
	informer               cache.SharedIndexInformer
	resolveTargetsCallback scalertypes.ResolveTargetsFromIngressCallback
}

func NewIngressWatcher(
	dlxCtx context.Context,
	dlxLogger logger.Logger,
	kubeClient kubernetes.Interface,
	resolveTargetsCallback scalertypes.ResolveTargetsFromIngressCallback,
	resyncInterval scalertypes.Duration,
	namespace string,
	labelSelector string,
) (*IngressWatcher, error) {
	if resyncInterval.Duration == 0 {
		resyncInterval = scalertypes.Duration{Duration: scalertypes.DefaultResyncInterval}
	}

	ctxWithCancel, cancel := context.WithCancel(dlxCtx)

	factory := informers.NewSharedInformerFactoryWithOptions(
		kubeClient,
		resyncInterval.Duration,
		informers.WithNamespace(namespace),
		informers.WithTweakListOptions(func(options *metav1.ListOptions) {
			options.LabelSelector = labelSelector
		}),
	)
	ingressInformer := factory.Networking().V1().Ingresses().Informer()

	ingressWatcher := &IngressWatcher{
		ctx:                    ctxWithCancel,
		cancel:                 cancel,
		logger:                 dlxLogger.GetChild("watcher"),
		cache:                  ingresscache.NewIngressCache(dlxLogger),
		factory:                factory,
		informer:               ingressInformer,
		resolveTargetsCallback: resolveTargetsCallback,
	}

	if _, err := ingressInformer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    ingressWatcher.AddHandler,
		UpdateFunc: ingressWatcher.UpdateHandler,
		DeleteFunc: ingressWatcher.DeleteHandler,
	}); err != nil {
		return nil, errors.Wrap(err, "Failed to add event handlers to informer")
	}

	return ingressWatcher, nil
}

func (iw *IngressWatcher) Start() error {
	iw.logger.Info("Starting ingress watcher")
	iw.factory.Start(iw.ctx.Done())

	if !cache.WaitForCacheSync(iw.ctx.Done(), iw.informer.HasSynced) {
		return errors.New("Failed to sync ingress cache")
	}

	iw.logger.Info("Ingress watcher started successfully")

	return nil
}

func (iw *IngressWatcher) Stop() {
	iw.logger.Info("Stopping ingress watcher")
	iw.cancel()
	iw.factory.Shutdown()
}

// GetIngressHostCacheReader expose read-only access to the ingress cache
func (iw *IngressWatcher) GetIngressHostCacheReader() ingresscache.IngressHostCacheReader {
	return iw.cache
}

// --- ResourceEventHandler methods ---

func (iw *IngressWatcher) AddHandler(obj interface{}) {
	ingress, err := iw.extractValuesFromIngressResource(obj)
	if err != nil {
		iw.logger.DebugWith("Add ingress handler failure - failed to extract values from ingress resource",
			"error", err.Error())
		return
	}

	if err := iw.cache.Set(ingress.Host, ingress.Path, ingress.Targets); err != nil {
		iw.logger.WarnWith("Add ingress handler failure - failed to add the new value to ingress cache",
			"error", err.Error(),
			"object", obj,
			"ingressName", ingress.Name,
			"host", ingress.Host,
			"path", ingress.Path,
			"targets", ingress.Targets)
		return
	}

	iw.logger.DebugWith("Add ingress handler - successfully added ingress to cache",
		"ingressName", ingress.Name,
		"host", ingress.Host,
		"path", ingress.Path,
		"targets", ingress.Targets)
}

func (iw *IngressWatcher) UpdateHandler(oldObj, newObj interface{}) {
	oldIngressResource, ok := oldObj.(*networkingv1.Ingress)
	if !ok {
		iw.logger.DebugWith("Failed to cast old object to Ingress",
			"object", oldObj)
		return
	}

	newIngressResource, ok := newObj.(*networkingv1.Ingress)
	if !ok {
		iw.logger.DebugWith("Failed to cast new object to Ingress",
			"object", newObj)
		return
	}

	// ResourceVersion is managed by Kubernetes and indicates whether the resource has changed.
	// Comparing resourceVersion helps avoid unnecessary updates triggered by periodic informer resync
	if oldIngressResource.ResourceVersion == newIngressResource.ResourceVersion {
		return
	}

	oldIngress, err := iw.extractValuesFromIngressResource(oldObj)
	if err != nil {
		iw.logger.DebugWith("Update ingress handler - failed to extract values from old object",
			"error", err.Error())
		return
	}

	newIngress, err := iw.extractValuesFromIngressResource(newObj)
	if err != nil {
		iw.logger.DebugWith("Update ingress handler - failed to extract values from new object",
			"error", err.Error())
		return
	}

	// if the host or path has changed, we need to delete the old entry
	if oldIngress.Host != newIngress.Host || oldIngress.Path != newIngress.Path {
		if err := iw.cache.Delete(oldIngress.Host, oldIngress.Path, oldIngress.Targets); err != nil {
			iw.logger.WarnWith("Update ingress handler failure - failed to delete old ingress",
				"error", err.Error(),
				"ingressName", oldIngress.Name,
				"object", oldObj,
				"host", oldIngress.Host,
				"path", oldIngress.Path,
				"targets", oldIngress.Targets)
		}
	}

	if err := iw.cache.Set(newIngress.Host, newIngress.Path, newIngress.Targets); err != nil {
		iw.logger.WarnWith("Update ingress handler failure - failed to add the new value",
			"error", err.Error(),
			"object", newObj,
			"ingressName", newIngress.Name,
			"host", newIngress.Host,
			"path", newIngress.Path,
			"targets", newIngress.Targets)
		return
	}

	iw.logger.DebugWith("Update ingress handler - successfully updated ingress in cache",
		"ingressName", newIngress.Name,
		"host", newIngress.Host,
		"path", newIngress.Path,
		"targets", newIngress.Targets)
}

func (iw *IngressWatcher) DeleteHandler(obj interface{}) {
	ingress, err := iw.extractValuesFromIngressResource(obj)
	if err != nil {
		iw.logger.DebugWith("Delete ingress handler failure - failed to extract values from object",
			"error", err.Error())
		return
	}

	if err := iw.cache.Delete(ingress.Host, ingress.Path, ingress.Targets); err != nil {
		iw.logger.WarnWith("Delete ingress handler failure - failed delete from cache",
			"error", err.Error(),
			"object", obj,
			"ingressName", ingress.Name,
			"host", ingress.Host,
			"path", ingress.Path,
			"targets", ingress.Targets)
		return
	}

	iw.logger.DebugWith("Delete ingress handler - successfully deleted ingress from cache",
		"ingressName", ingress.Name,
		"host", ingress.Host,
		"path", ingress.Path,
		"targets", ingress.Targets)
}

// --- internal methods ---

// extractValuesFromIngressResource extracts the relevant values from a Kubernetes Ingress resource
func (iw *IngressWatcher) extractValuesFromIngressResource(obj interface{}) (*IngressValue, error) {
	ingress, ok := obj.(*networkingv1.Ingress)
	if !ok {
		return nil, errors.New("Failed to cast object to Ingress")
	}

	targets, err := iw.resolveTargetsCallback(ingress)
	if err != nil {
		return nil, errors.Wrapf(err, "Failed to extract targets from ingress labels: %s", err.Error())
	}

	if len(targets) == 0 {
		return nil, errors.New("No targets found in ingress")
	}

	host, err := iw.getHostFromIngress(ingress)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to extract host from ingress")
	}

	path, err := iw.getPathFromIngress(ingress)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to extract path from ingress")
	}

	return &IngressValue{
		Host:    host,
		Path:    path,
		Targets: targets,
		Name:    ingress.Name,
	}, nil
}

func (iw *IngressWatcher) getHostFromIngress(ingress *networkingv1.Ingress) (string, error) {
	rule, err := iw.getFirstRule(ingress)
	if err != nil || rule == nil {
		return "", errors.Wrap(err, "Failed to get first rule from ingress")
	}

	if rule.Host == "" {
		return "", errors.New("Host is empty in ingress rule")
	}

	return rule.Host, nil
}

func (iw *IngressWatcher) getPathFromIngress(ingress *networkingv1.Ingress) (string, error) {
	rule, err := iw.getFirstRule(ingress)
	if err != nil || rule == nil {
		return "", errors.Wrap(err, "Failed to get first rule from ingress")
	}

	if rule.HTTP == nil {
		return "", errors.New("No HTTP configuration found in ingress rule")
	}

	switch len(rule.HTTP.Paths) {
	case 0:
		return "", errors.New("No paths found in ingress HTTP paths")
	case 1:
		// Exactly one path exists — proceed with it as expected
	default:
		// Although Kubernetes allows defining multiple paths in a single HTTP rule,
		// the scaler takes only the first path by design to ensure consistent scaling behavior.
		iw.logger.InfoWith("Multiple paths found in ingress, taking first path",
			"ingress", ingress)
	}

	firstPath := rule.HTTP.Paths[0]
	path := firstPath.Path
	if path == "" {
		return "", errors.New("Path is empty in the first ingress HTTP path")
	}

	return path, nil
}

func (iw *IngressWatcher) getFirstRule(ingress *networkingv1.Ingress) (*networkingv1.IngressRule, error) {
	if ingress == nil {
		return nil, errors.New("Ingress is nil")
	}

	switch len(ingress.Spec.Rules) {
	case 0:
		return nil, errors.New("No rules found in ingress")
	case 1:
		// Exactly one rule exists — proceed with it as expected
	default:
		// Although Kubernetes allows defining multiple rules in a single Ingress resource,
		// the scaler takes only the first rule by design to ensure consistent scaling behavior.
		iw.logger.InfoWith("Multiple rules found in ingress, taking first rule",
			"ingress", ingress)
	}
	return &ingress.Spec.Rules[0], nil
}
