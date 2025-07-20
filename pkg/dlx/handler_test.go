package dlx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/v3io/scaler/pkg/ingresscache"
	"github.com/v3io/scaler/pkg/kube"
	resourcescalerMock "github.com/v3io/scaler/pkg/resourcescaler/mock"
	"github.com/v3io/scaler/pkg/scalertypes"

	"github.com/nuclio/logger"
	nucliozap "github.com/nuclio/zap"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
)

type HandlerTestSuite struct {
	suite.Suite
	logger      logger.Logger
	starter     *ResourceStarter
	scaler      *resourcescalerMock.ResourceScaler
	httpServer  *httptest.Server
	backendHost string
	backendPort int
}

func (suite *HandlerTestSuite) SetupSuite() {
	var err error
	suite.logger, err = nucliozap.NewNuclioZapTest("test")
	suite.Require().NoError(err)
}

func (suite *HandlerTestSuite) SetupTest() {
	suite.scaler = &resourcescalerMock.ResourceScaler{}
	suite.starter = &ResourceStarter{
		logger:                   suite.logger,
		scaler:                   suite.scaler,
		resourceReadinessTimeout: 3 * time.Second,
	}
	allowedPaths := map[string]struct{}{
		// TODO - To fix this test for a valid path (i.e.- '/test/path'), the path suffix needs to be removed from h.parseTargetURL
		"/test/path/test/path":                         {},
		"/test/path/to/multiple/test/path/to/multiple": {},
	}
	// Start a test server that always returns 200
	suite.httpServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, exists := allowedPaths[r.URL.Path]; exists {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))

	backendURL, _ := url.Parse(suite.httpServer.URL)
	suite.backendHost = backendURL.Hostname()
	backendPort := backendURL.Port()
	if backendPort == "" {
		backendPort = "8080" // Default HTTP port
	}
	backendPortInt, err := strconv.Atoi(backendPort)
	suite.Require().NoError(err)
	suite.backendPort = backendPortInt
}

func (suite *HandlerTestSuite) TearDownTest() {
	if suite.httpServer != nil {
		suite.httpServer.Close()
	}
}

func (suite *HandlerTestSuite) TestHandleRequest() {
	for _, testCase := range []struct {
		name                  string
		resolveServiceNameErr error
		initialCachedData     *kube.IngressValue
		reqHeaders            map[string]string
		reqHost               string
		reqPath               string
		expectedStatus        int
	}{
		{
			name:                  "No ingress headers, host and path found in ingress cache",
			resolveServiceNameErr: nil,
			initialCachedData: &kube.IngressValue{
				Host:    "www.example.com",
				Path:    "test/path",
				Targets: []string{"test-targets-name-1"},
			},
			reqHost:        "www.example.com",
			reqPath:        "test/path",
			expectedStatus: http.StatusOK,
		}, {
			name:                  "No ingress headers, multiple targets found in ingress cache",
			resolveServiceNameErr: nil,
			initialCachedData: &kube.IngressValue{
				Host:    "www.example.com",
				Path:    "test/path/to/multiple",
				Targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			reqHost:        "www.example.com",
			reqPath:        "test/path/to/multiple",
			expectedStatus: http.StatusOK,
		},
		{
			name:                  "No ingress headers, not found in ingress cache",
			resolveServiceNameErr: nil,
			initialCachedData:     nil,
			reqHost:               "unknown",
			reqPath:               "/notfound",
			expectedStatus:        http.StatusBadRequest,
		},
		{
			name:                  "No ingress headers, scaler fails",
			resolveServiceNameErr: errors.New("fail"),
			initialCachedData: &kube.IngressValue{
				Host:    "www.example.com",
				Path:    "test/path",
				Targets: []string{"test-targets-name-1"},
			},
			reqHost:        "www.example.com",
			reqPath:        "test/path",
			expectedStatus: http.StatusInternalServerError,
		},
	} {
		suite.Run(testCase.name, func() {
			// test case setup
			suite.scaler.ExpectedCalls = nil
			suite.scaler.On("ResolveServiceName", mock.Anything).Return(suite.backendHost, testCase.resolveServiceNameErr)
			suite.scaler.On("SetScaleCtx", mock.Anything, mock.Anything, mock.Anything).Return(nil)

			testHandler, err := suite.createTestHandlerAndInitTestCache(suite.backendPort, testCase.initialCachedData)
			suite.Require().NoError(err)
			testRequest := suite.createTestHTTPRequest(testCase.reqHeaders, testCase.reqHost, testCase.reqPath)
			testResponse := httptest.NewRecorder()

			// call the testHandler
			testHandler.handleRequest(testResponse, testRequest)

			// validate the response
			suite.Require().Equal(testCase.expectedStatus, testResponse.Code)
			suite.scaler.AssertExpectations(suite.T())
		})
	}
}

func (suite *HandlerTestSuite) TestGetPathAndResourceNames() {
	for _, testCase := range []struct {
		name                  string
		errMsg                string
		initialCachedData     *kube.IngressValue
		reqHeaders            map[string]string
		reqHost               string
		reqPath               string
		expectErr             bool
		expectedPath          string
		expectedResourceNames []string
	}{
		{
			name: "No ingress headers, host and path found in ingress cache",
			initialCachedData: &kube.IngressValue{
				Host:    "www.example.com",
				Path:    "test/path",
				Targets: []string{"test-targets-name-1"},
			},
			reqHost:               "www.example.com",
			reqPath:               "test/path",
			expectedPath:          "test/path",
			expectedResourceNames: []string{"test-targets-name-1"},
		}, {
			name:                  "Ingress headers, host and path did not found in ingress cache",
			reqHost:               "www.example.com",
			reqPath:               "test/path",
			expectedPath:          "test/path",
			expectedResourceNames: []string{"test-targets-name-1"},
			reqHeaders: map[string]string{
				"X-Resource-Name": "test-targets-name-1",
				"X-Resource-Path": "test/path",
			},
		}, {
			name:      "Missing both ingress headers and host and path did not found in ingress cache",
			reqHost:   "www.example.com",
			reqPath:   "test/path",
			expectErr: true,
			errMsg:    "No target name header found",
		}, {
			name:    "Both ingress headers and found in ingress cache, cache results should be taken",
			reqHost: "www.example.com",
			reqPath: "test/path",
			initialCachedData: &kube.IngressValue{
				Host:    "www.example.com",
				Path:    "test/path",
				Targets: []string{"test-targets-from-cache"},
			},
			reqHeaders: map[string]string{
				"X-Resource-Name": "test-targets-from-headers",
				"X-Resource-Path": "test/path",
			},
			expectedPath:          "test/path",
			expectedResourceNames: []string{"test-targets-from-cache"},
		},
	} {
		suite.Run(testCase.name, func() {
			// test case setup
			testHandler, err := suite.createTestHandlerAndInitTestCache(suite.backendPort, testCase.initialCachedData)
			suite.Require().NoError(err)
			testRequest := suite.createTestHTTPRequest(testCase.reqHeaders, testCase.reqHost, testCase.reqPath)
			resultPath, resultResourceNames, err := testHandler.getPathAndResourceNames(testRequest)

			// validate the result
			if testCase.expectErr {
				suite.Require().Error(err)
				suite.Require().ErrorContains(err, testCase.errMsg)
			} else {
				suite.Require().NoError(err)
				suite.Require().Equal(testCase.expectedPath, resultPath)
				suite.Require().Equal(testCase.expectedResourceNames, resultResourceNames)
			}
		})
	}
}

// --- HandlerTestSuite suite methods ---

func (suite *HandlerTestSuite) createTestHandlerAndInitTestCache(targetPort int, initialCachedData *kube.IngressValue) (Handler, error) {
	testIngressCache := ingresscache.NewIngressCache(suite.logger)
	if initialCachedData != nil {
		if err := testIngressCache.Set(initialCachedData.Host, initialCachedData.Path, initialCachedData.Targets); err != nil {
			return Handler{}, err
		}
	}

	return NewHandler(
		suite.logger,
		suite.starter,
		suite.scaler,
		"X-Resource-Name",
		"X-Resource-Path",
		targetPort,
		scalertypes.MultiTargetStrategyPrimary,
		testIngressCache,
	)
}

func (suite *HandlerTestSuite) createTestHTTPRequest(
	reqHeaders map[string]string,
	reqHost string,
	reqPath string,
) *http.Request {
	req := httptest.NewRequest("GET", "/", nil)
	if reqHost != "" {
		req.Host = reqHost
	}
	if reqPath != "" {
		req.URL.Path = reqPath
	}
	for k, v := range reqHeaders {
		req.Header.Set(k, v)
	}
	return req
}

func TestHandlerTestSuite(t *testing.T) {
	suite.Run(t, new(HandlerTestSuite))
}
