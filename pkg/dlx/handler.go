package dlx

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	"github.com/v3io/scaler-types"
)

type Handler struct {
	logger           logger.Logger
	HandleFunc       func(http.ResponseWriter, *http.Request)
	resourceStarter  *ResourceStarter
	resourceScaler   scaler_types.ResourceScaler
	targetNameHeader string
	targetPathHeader string
	targetPort       int
}

func NewHandler(parentLogger logger.Logger,
	resourceStarter *ResourceStarter,
	resourceScaler scaler_types.ResourceScaler,
	targetNameHeader string,
	targetPathHeader string,
	targetPort int) (Handler, error) {
	h := Handler{
		logger:           parentLogger.GetChild("handler"),
		resourceStarter:  resourceStarter,
		resourceScaler:   resourceScaler,
		targetNameHeader: targetNameHeader,
		targetPathHeader: targetPathHeader,
		targetPort:       targetPort,
	}
	h.HandleFunc = h.handleRequest
	return h, nil
}

func (h *Handler) handleRequest(res http.ResponseWriter, req *http.Request) {
	var targetURL *url.URL
	var err error
	var resourceName string

	responseChannel := make(chan ResourceStatusResult, 1)
	defer close(responseChannel)

	// first try to see if our request came from ingress controller
	forwardedHost := req.Header.Get("X-Forwarded-Host")
	forwardedPort := req.Header.Get("X-Forwarded-Port")
	originalURI := req.Header.Get("X-Original-Uri")
	resourceName = req.Header.Get("X-Resource-Name")

	if forwardedHost != "" && forwardedPort != "" && resourceName != "" {
		targetURL, err = url.Parse(fmt.Sprintf("http://%s:%s/%s", forwardedHost, forwardedPort, originalURI))
		if err != nil {
			res.WriteHeader(h.URLBadParse(resourceName, err))
			return
		}
	} else {
		resourceName = req.Header.Get(h.targetNameHeader)
		path := req.Header.Get(h.targetPathHeader)
		if resourceName == "" {
			h.logger.WarnWith("When ingress not set, must pass header value", "missingHeader", h.targetNameHeader)
			res.WriteHeader(http.StatusBadRequest)
			return
		}
		serviceName, err := h.resourceScaler.ResolveServiceName(scaler_types.Resource{Name: resourceName})
		if err != nil {
			h.logger.WarnWith("Failed resolving service name",
				"err", errors.GetErrorStackString(err, 10))
			res.WriteHeader(http.StatusInternalServerError)
			return
		}
		targetURL, err = url.Parse(fmt.Sprintf("http://%s:%d/%s", serviceName, h.targetPort, path))
		if err != nil {
			res.WriteHeader(h.URLBadParse(resourceName, err))
			return
		}
	}

	h.resourceStarter.handleResourceStart(resourceName, responseChannel)
	statusResult := <-responseChannel

	if statusResult.Error != nil {
		h.logger.WarnWith("Failed to forward request to resource",
			"resource", statusResult.ResourceName,
			"err", errors.GetErrorStackString(statusResult.Error, 10))
		res.WriteHeader(statusResult.Status)
		return
	}

	// let the function http trigger server some time before blasting him
	time.Sleep(3 * time.Second)
	h.logger.DebugWith("Creating reverse proxy", "targetURL", targetURL)
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ServeHTTP(res, req)
}

func (h *Handler) URLBadParse(resourceName string, err error) int {
	h.logger.Warn("Failed to parse url for resource",
		"resourceName", resourceName,
		"err", errors.GetErrorStackString(err, 10))
	return http.StatusBadRequest
}
