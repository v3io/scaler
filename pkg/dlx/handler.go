package dlx

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
)

type Handler struct {
	logger           logger.Logger
	HandleFunc       func(http.ResponseWriter, *http.Request)
	resourceStarter  *ResourceStarter
	targetNameHeader string
	targetPathHeader string
	targetPort       int
}

func NewHandler(logger logger.Logger,
	resourceStarter *ResourceStarter,
	targetNameHeader string,
	targetPathHeader string,
	targetPort int) (Handler, error) {
	h := Handler{
		logger:           logger,
		resourceStarter:  resourceStarter,
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
	resourceName = req.Header.Get("X-Service-Name")

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
		targetURL, err = url.Parse(fmt.Sprintf("http://%s:%d/%s", resourceName, h.targetPort, path))
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

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ServeHTTP(res, req)
}

func (h *Handler) URLBadParse(resourceName string, err error) int {
	h.logger.Warn("Failed to parse url for resource",
		"resourceName", resourceName,
		"err", errors.GetErrorStackString(err, 10))
	return http.StatusBadRequest
}
