/*
Copyright 2019 Iguazio Systems Ltd.

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

package dlx

import (
	"context"
	"net/http"

	"github.com/v3io/scaler/pkg/ingresscache"
	"github.com/v3io/scaler/pkg/kube"
	"github.com/v3io/scaler/pkg/scalertypes"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
)

type DLX struct {
	logger  logger.Logger
	handler Handler
	server  *http.Server
	cache   ingresscache.IngressHostCache
	watcher *kube.IngressWatcher
}

func NewDLX(parentLogger logger.Logger,
	resourceScaler scalertypes.ResourceScaler,
	options scalertypes.DLXOptions) (*DLX, error) {
	childLogger := parentLogger.GetChild("dlx")
	childLogger.InfoWith("Creating DLX",
		"options", options)
	resourceStarter, err := NewResourceStarter(childLogger,
		resourceScaler,
		options.Namespace,
		options.ResourceReadinessTimeout.Duration)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create function starter")
	}

	cache := ingresscache.NewIngressCache(childLogger)
	handler, err := NewHandler(childLogger,
		resourceStarter,
		resourceScaler,
		options.TargetNameHeader,
		options.TargetPathHeader,
		options.TargetPort,
		options.MultiTargetStrategy)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create handler")
	}

	watcher, err := kube.NewIngressWatcher(
		context.Background(),
		childLogger,
		options.KubeClientSet,
		cache,
		options.ResolveTargetsFromIngressCallback,
		options.ResyncInterval,
		options.Namespace,
		options.LabelSelector,
	)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to create ingress watcher")
	}

	return &DLX{
		logger:  childLogger,
		handler: handler,
		server: &http.Server{
			Addr: options.ListenAddress,
		},
		cache:   cache,
		watcher: watcher,
	}, nil
}

func (d *DLX) Start() error {
	d.logger.DebugWith("Starting", "server", d.server.Addr)
	http.HandleFunc("/", d.handler.HandleFunc)

	// Start the ingress watcher synchronously to ensure cache is fully synced before DLX begins handling traffic
	if err := d.watcher.Start(); err != nil {
		return errors.Wrap(err, "Failed to start ingress watcher")
	}

	go d.server.ListenAndServe() // nolint: errcheck
	return nil
}

func (d *DLX) Stop(context context.Context) error {
	d.logger.DebugWith("Stopping", "server", d.server.Addr)
	d.watcher.Stop()
	return d.server.Shutdown(context)
}
