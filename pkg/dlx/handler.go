package dlx

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/v3io/scaler/pkg/common"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	"github.com/v3io/scaler-types"
	"k8s.io/apimachinery/pkg/util/cache"
)

type Handler struct {
	logger             logger.Logger
	HandleFunc         func(http.ResponseWriter, *http.Request)
	resourceStarter    *ResourceStarter
	resourceScaler     scaler_types.ResourceScaler
	targetNameHeader   string
	targetPathHeader   string
	targetPort         int
	targetURLCache     *cache.LRUExpireCache
	proxyLock          sync.Locker
	lastProxyErrorTime time.Time
}

func NewHandler(parentLogger logger.Logger,
	resourceStarter *ResourceStarter,
	resourceScaler scaler_types.ResourceScaler,
	targetNameHeader string,
	targetPathHeader string,
	targetPort int) (Handler, error) {
	h := Handler{
		logger:             parentLogger.GetChild("handler"),
		resourceStarter:    resourceStarter,
		resourceScaler:     resourceScaler,
		targetNameHeader:   targetNameHeader,
		targetPathHeader:   targetPathHeader,
		targetPort:         targetPort,
		targetURLCache:     cache.NewLRUExpireCache(100),
		proxyLock:          &sync.Mutex{},
		lastProxyErrorTime: time.Now(),
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

	targetURL := h.selectTargetURL(resourceNames, resourceTargetURLMap)
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
	serviceName, err := h.resourceScaler.ResolveServiceName(scaler_types.Resource{Name: resourceName})
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

func (h *Handler) selectTargetURL(resourceNames []string, resourceTargetURLMap map[string]*url.URL) *url.URL {
	resourceName := resourceNames[common.SeededRand.Intn(len(resourceNames))]
	resourceTargetURL := resourceTargetURLMap[resourceName]
	return resourceTargetURL
}

func (h *Handler) URLBadParse(resourceName string, err error) int {
	h.logger.Warn("Failed to parse url for resource",
		"resourceName", resourceName,
		"err", errors.GetErrorStackString(err, 10))
	return http.StatusBadRequest
}
