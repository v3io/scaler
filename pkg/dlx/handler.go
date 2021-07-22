package dlx

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/v3io/scaler/pkg/scalertypes"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
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

	h.logger.DebugWith("Creating reverse proxy", "targetURL", targetURL)
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ServeHTTP(res, req)
}

func (h *Handler) parseTargetURL(resourceName, path string) (*url.URL, int) {
	h.logger.DebugWith("Resolving service name", "resourceName", resourceName)
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

	for _, resourceName := range resourceNames {
		h.resourceStarter.handleResourceStart(resourceName, responseChannel)
	}

	for range resourceNames {
		statusResult := <-responseChannel

		if statusResult.Error != nil {
			h.logger.WarnWith("Failed to forward request to resource",
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
