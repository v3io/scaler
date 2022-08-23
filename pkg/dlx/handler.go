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
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/v3io/scaler/pkg/scalertypes"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	"k8s.io/apimachinery/pkg/util/cache"
)

type Handler struct {
	logger              logger.Logger
	HandleFunc          func(http.ResponseWriter, *http.Request)
	resourceStarter     *ResourceStarter
	resourceScaler      scalertypes.ResourceScaler
	targetNameHeader    string
	targetPathHeader    string
	targetPort          int
	multiTargetStrategy scalertypes.MultiTargetStrategy
	targetURLCache      *cache.LRUExpireCache
	proxyLock           sync.Locker
	lastProxyErrorTime  time.Time
}

func NewHandler(parentLogger logger.Logger,
	resourceStarter *ResourceStarter,
	resourceScaler scalertypes.ResourceScaler,
	targetNameHeader string,
	targetPathHeader string,
	targetPort int,
	multiTargetStrategy scalertypes.MultiTargetStrategy) (Handler, error) {
	h := Handler{
		logger:              parentLogger.GetChild("handler"),
		resourceStarter:     resourceStarter,
		resourceScaler:      resourceScaler,
		targetNameHeader:    targetNameHeader,
		targetPathHeader:    targetPathHeader,
		targetPort:          targetPort,
		multiTargetStrategy: multiTargetStrategy,
		targetURLCache:      cache.NewLRUExpireCache(100),
		proxyLock:           &sync.Mutex{},
		lastProxyErrorTime:  time.Now(),
	}
	h.HandleFunc = h.handleRequest
	return h, nil
}

func (h *Handler) handleRequest(res http.ResponseWriter, req *http.Request) {
	var resourceNames []string

	responseChannel := make(chan ResourceStatusResult, 1)
	defer close(responseChannel)

	// first try to see if our request came from ingress controller
	forwardedHost := req.Header.Get("X-Forwarded-Host")
	forwardedPort := req.Header.Get("X-Forwarded-Port")
	originalURI := req.Header.Get("X-Original-Uri")
	resourceName := req.Header.Get("X-Resource-Name")

	resourceTargetURLMap := map[string]*url.URL{}

	if forwardedHost != "" && forwardedPort != "" && resourceName != "" {
		targetURL, err := url.Parse(fmt.Sprintf("http://%s:%s/%s", forwardedHost, forwardedPort, originalURI))
		if err != nil {
			res.WriteHeader(h.URLBadParse(resourceName, err))
			return
		}
		resourceNames = append(resourceNames, resourceName)
		resourceTargetURLMap[resourceName] = targetURL
	} else {
		targetNameHeaderValue := req.Header.Get(h.targetNameHeader)
		path := req.Header.Get(h.targetPathHeader)
		if targetNameHeaderValue == "" {
			h.logger.WarnWith("When ingress not set, must pass header value",
				"missingHeader", h.targetNameHeader)
			res.WriteHeader(http.StatusBadRequest)
			return
		}
		resourceNames = strings.Split(targetNameHeaderValue, ",")
		for _, resourceName := range resourceNames {
			targetURL, status := h.parseTargetURL(resourceName, path)
			if targetURL == nil {
				res.WriteHeader(status)
				return
			}

			resourceTargetURLMap[resourceName] = targetURL
		}
	}

	statusResult := h.startResources(resourceNames)

	if statusResult != nil && statusResult.Error != nil {
		res.WriteHeader(statusResult.Status)
		return
	}

	targetURL, err := h.selectTargetURL(resourceNames, resourceTargetURLMap)
	if err != nil {
		res.WriteHeader(http.StatusInternalServerError)
		return
	}

	h.proxyLock.Lock()
	targetURLCacheKey := targetURL.String()

	// if in cache, do not log to avoid multiple identical log lines.
	if _, found := h.targetURLCache.Get(targetURLCacheKey); !found {
		h.logger.DebugWith("Creating reverse proxy", "targetURLCache", targetURL)

		// store in cache
		h.targetURLCache.Add(targetURLCacheKey, true, 5*time.Second)
	}
	h.proxyLock.Unlock()

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// override the proxy's error handler in order to make the "context canceled" log appear once every hour at most,
	// because it occurs frequently and spams the logs file, but we didn't want to remove it entirely.
	proxy.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		if err == nil {
			return
		}
		timeSinceLastCtxErr := time.Since(h.lastProxyErrorTime).Hours() > 1
		if strings.Contains(err.Error(), "context canceled") && timeSinceLastCtxErr {
			h.lastProxyErrorTime = time.Now()
		}
		if !strings.Contains(err.Error(), "context canceled") || timeSinceLastCtxErr {
			h.logger.DebugWith("http: proxy error", "error", err)
		}
		rw.WriteHeader(http.StatusBadGateway)
	}

	proxy.ServeHTTP(res, req)
}

func (h *Handler) parseTargetURL(resourceName, path string) (*url.URL, int) {
	serviceName, err := h.resourceScaler.ResolveServiceName(scalertypes.Resource{Name: resourceName})
	if err != nil {
		h.logger.WarnWith("Failed resolving service name",
			"err", errors.GetErrorStackString(err, 10))
		return nil, http.StatusInternalServerError
	}
	targetURL, err := url.Parse(fmt.Sprintf("http://%s:%d/%s", serviceName, h.targetPort, path))
	if err != nil {
		return nil, h.URLBadParse(resourceName, err)
	}
	return targetURL, 0
}

func (h *Handler) startResources(resourceNames []string) *ResourceStatusResult {
	responseChannel := make(chan ResourceStatusResult, len(resourceNames))
	defer close(responseChannel)

	// Start all resources in separate go routines
	for _, resourceName := range resourceNames {
		go h.resourceStarter.handleResourceStart(resourceName, responseChannel)
	}

	// Wait for all resources to finish starting
	for range resourceNames {
		statusResult := <-responseChannel

		if statusResult.Error != nil {
			h.logger.WarnWith("Failed to start resource",
				"resource", statusResult.ResourceName,
				"err", errors.GetErrorStackString(statusResult.Error, 10))
			return &statusResult
		}
	}

	return nil
}

func (h *Handler) selectTargetURL(resourceNames []string, resourceTargetURLMap map[string]*url.URL) (*url.URL, error) {
	if len(resourceNames) == 1 {
		return resourceTargetURLMap[resourceNames[0]], nil
	} else if len(resourceNames) != 2 {
		h.logger.WarnWith("Unsupported amount of targets",
			"targetsAmount", len(resourceNames))
		return nil, errors.Errorf("Unsupported amount of targets: %d", len(resourceNames))
	}

	switch h.multiTargetStrategy {
	case scalertypes.MultiTargetStrategyRandom:
		rand.Seed(time.Now().Unix())
		return resourceTargetURLMap[resourceNames[rand.Intn(len(resourceNames))]], nil
	case scalertypes.MultiTargetStrategyPrimary:
		return resourceTargetURLMap[resourceNames[0]], nil
	case scalertypes.MultiTargetStrategyCanary:
		return resourceTargetURLMap[resourceNames[1]], nil
	default:
		h.logger.WarnWith("Unsupported multi target strategy",
			"strategy", h.multiTargetStrategy)
		return nil, errors.Errorf("Unsupported multi target strategy: %s", h.multiTargetStrategy)
	}
}

func (h *Handler) URLBadParse(resourceName string, err error) int {
	h.logger.Warn("Failed to parse url for resource",
		"resourceName", resourceName,
		"err", errors.GetErrorStackString(err, 10))
	return http.StatusBadRequest
}
