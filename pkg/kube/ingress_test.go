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
	"strings"
	"testing"

	"github.com/v3io/scaler/pkg/ingresscache"

	"github.com/nuclio/logger"
	nucliozap "github.com/nuclio/zap"
	"github.com/stretchr/testify/suite"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

type IngressWatcherTestSuite struct {
	suite.Suite
	logger        logger.Logger
	kubeClientSet *fake.Clientset
}

func (suite *IngressWatcherTestSuite) SetupSuite() {
	suite.kubeClientSet = fake.NewSimpleClientset()
}

func (suite *IngressWatcherTestSuite) SetupTest() {
	var err error

	suite.logger, err = nucliozap.NewNuclioZapTest("test")
	suite.Require().NoError(err)
}

// TestIngressWatcherTestSuite runs the test suite
func TestIngressWatcherTestSuite(t *testing.T) {
	suite.Run(t, new(IngressWatcherTestSuite))
}

func (suite *IngressWatcherTestSuite) TestIngressHandlerAddFunc() {
	type ingressArgs struct {
		host   string
		path   string
		target []string
	}
	for _, testCase := range []struct {
		name              string
		testArgs          ingressArgs
		expectedResult    []string
		initialStateCache *ingressArgs
		errorMessage      string
		expectError       bool
	}{
		{
			name: "Add PairTarget",
			testArgs: ingressArgs{
				host:   "www.example.com",
				path:   "/test/path",
				target: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			expectedResult: []string{"test-targets-name-1", "test-targets-name-2"},
		}, {
			name: "Add SingleTarget",
			testArgs: ingressArgs{
				host:   "www.example.com",
				path:   "/test/path",
				target: []string{"test-targets-name-1"},
			},
			expectedResult: []string{"test-targets-name-1"},
		}, {
			name: "Add SingleTarget with different name to the same host and path",
			testArgs: ingressArgs{
				host:   "www.example.com",
				path:   "/test/path",
				target: []string{"test-targets-name-2"},
			},
			expectedResult: []string{"test-targets-name-2"},
			initialStateCache: &ingressArgs{
				host:   "www.example.com",
				path:   "/test/path",
				target: []string{"test-targets-name-1"},
			},
		}, {
			name: "bad input- should fail",
			testArgs: ingressArgs{
				host:   "www.example.com",
				path:   "/test/path",
				target: []string{"test-targets-name-2"},
			},
			expectedResult: []string{},
			expectError:    true,
			errorMessage:   "host does not exist",
		},
	} {
		suite.Run(testCase.name, func() {
			var testObj interface{}
			testIngressWatcher, err := suite.createTestIngressWatcher()
			suite.Require().NoError(err)

			testObj = suite.createDummyIngress(testCase.testArgs.host, testCase.testArgs.path, testCase.testArgs.target)

			if testCase.expectError {
				testObj = &networkingv1.IngressSpec{}
			}

			if testCase.initialStateCache != nil {
				err = testIngressWatcher.ingressCache.Set(testCase.initialStateCache.host, testCase.initialStateCache.path, testCase.initialStateCache.target)
				suite.Require().NoError(err)
			}

			testIngressWatcher.IngressHandlerAddFunc(testObj)
			// get the targets from the cache and compare values
			resultTargetNames, err := testIngressWatcher.ingressCache.Get(testCase.testArgs.host, testCase.testArgs.path)
			if testCase.expectError {
				suite.Require().Error(err)
				suite.Require().ErrorContains(err, testCase.errorMessage)
				suite.Require().Nil(resultTargetNames)
			} else {
				suite.Require().NoError(err)
				suite.Require().Equal(testCase.expectedResult, resultTargetNames)
			}
		})
	}
}

func (suite *IngressWatcherTestSuite) TestIngressHandlerUpdateFunc() {
	type ingressArgs struct {
		host    string
		path    string
		targets []string
	}
	type expectedResults struct {
		host           string
		path           string
		targets        []string
		expectError    bool
		expectErrorMsg string
	}
	for _, testCase := range []struct {
		name              string
		expectedResults   []expectedResults
		initialStateCache *ingressArgs
		testOldObj        ingressArgs
		testNewObj        ingressArgs
	}{
		{
			name: "Update PairTarget - same host and path, different Targets",
			expectedResults: []expectedResults{
				{
					host:    "www.example.com",
					path:    "/test/path",
					targets: []string{"test-targets-name-1", "test-targets-name-3"},
				},
			},
			testOldObj: ingressArgs{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			testNewObj: ingressArgs{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-3"},
			},
			initialStateCache: &ingressArgs{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
		}, {
			name: "Update PairTarget - different path- should Delete old Targets",
			initialStateCache: &ingressArgs{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			testOldObj: ingressArgs{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			testNewObj: ingressArgs{
				host:    "www.example.com",
				path:    "/another/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			expectedResults: []expectedResults{
				{
					host:           "www.example.com",
					path:           "/test/path",
					expectError:    true,
					expectErrorMsg: "failed to get the targets from the ingress host tree",
				}, {
					host:    "www.example.com",
					path:    "/another/path",
					targets: []string{"test-targets-name-1", "test-targets-name-2"},
				},
			},
		}, {
			name: "Update PairTarget - different host- should Delete old Targets",
			initialStateCache: &ingressArgs{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			testOldObj: ingressArgs{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			testNewObj: ingressArgs{
				host:    "www.google.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			expectedResults: []expectedResults{
				{
					host:           "www.example.com",
					path:           "/test/path",
					expectError:    true,
					expectErrorMsg: "host does not exist",
				}, {
					host:    "www.google.com",
					path:    "/test/path",
					targets: []string{"test-targets-name-1", "test-targets-name-2"},
				},
			},
		},
	} {
		suite.Run(testCase.name, func() {
			testIngressWatcher, err := suite.createTestIngressWatcher()
			suite.Require().NoError(err)

			testOldObj := suite.createDummyIngress(testCase.testOldObj.host, testCase.testOldObj.path, testCase.testOldObj.targets)
			testNewObj := suite.createDummyIngress(testCase.testNewObj.host, testCase.testNewObj.path, testCase.testNewObj.targets)

			if testCase.initialStateCache != nil {
				err = testIngressWatcher.ingressCache.Set(testCase.initialStateCache.host, testCase.initialStateCache.path, testCase.initialStateCache.targets)
				suite.Require().NoError(err)
			}

			testIngressWatcher.IngressHandlerUpdateFunc(testOldObj, testNewObj)

			// iterate over the expectedResults to check the results for each host and path
			for _, expectedResult := range testCase.expectedResults {
				resultTargetNames, err := testIngressWatcher.ingressCache.Get(expectedResult.host, expectedResult.path)
				if expectedResult.expectError {
					suite.Require().Error(err)
					suite.Require().ErrorContains(err, expectedResult.expectErrorMsg)
					suite.Require().Nil(resultTargetNames)
				} else {
					suite.Require().NoError(err)
					suite.Require().Equal(expectedResult.targets, resultTargetNames)
				}
			}
		})
	}
}

func (suite *IngressWatcherTestSuite) TestIngressHandlerDeleteFunc() {
	type ingressArgs struct {
		host    string
		path    string
		targets []string
	}
	type expectedResult struct {
		host           string
		path           string
		targets        []string
		expectError    bool
		expectErrorMsg string
	}
	for _, testCase := range []struct {
		name              string
		testArgs          ingressArgs
		expectedResult    expectedResult
		initialStateCache *ingressArgs
		errorMessage      string
		expectError       bool
	}{
		{
			name: "Delete PairTarget",
			initialStateCache: &ingressArgs{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			testArgs: ingressArgs{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			expectedResult: expectedResult{
				host:           "www.example.com",
				path:           "/test/path",
				targets:        nil,
				expectError:    true,
				expectErrorMsg: "host does not exist",
			},
		}, {
			name: "Delete SingleTarget",
			initialStateCache: &ingressArgs{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1"},
			},
			testArgs: ingressArgs{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1"},
			},
			expectedResult: expectedResult{
				host:           "www.example.com",
				path:           "/test/path",
				targets:        nil,
				expectError:    true,
				expectErrorMsg: "host does not exist",
			},
		}, {
			name: "bad input- should fail and keep the cache as is",
			initialStateCache: &ingressArgs{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1"},
			},
			testArgs: ingressArgs{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-2"},
			},
			expectedResult: expectedResult{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1"},
			},
			expectError:  true,
			errorMessage: "host does not exist",
		},
	} {
		suite.Run(testCase.name, func() {
			var testObj interface{}
			testIngressWatcher, err := suite.createTestIngressWatcher()
			suite.Require().NoError(err)

			testObj = suite.createDummyIngress(testCase.testArgs.host, testCase.testArgs.path, testCase.testArgs.targets)

			if testCase.expectError {
				testObj = &networkingv1.IngressSpec{}
			}

			if testCase.initialStateCache != nil {
				err = testIngressWatcher.ingressCache.Set(testCase.initialStateCache.host, testCase.initialStateCache.path, testCase.initialStateCache.targets)
				suite.Require().NoError(err)
			}

			testIngressWatcher.IngressHandlerDeleteFunc(testObj)
			// get the targets from the cache and compare values
			resultTargetNames, err := testIngressWatcher.ingressCache.Get(testCase.expectedResult.host, testCase.expectedResult.path)
			if testCase.expectedResult.expectError {
				suite.Require().Error(err)
				suite.Require().ErrorContains(err, testCase.expectedResult.expectErrorMsg)
				suite.Require().Nil(resultTargetNames)
			} else {
				suite.Require().NoError(err)
				suite.Require().Equal(testCase.expectedResult.targets, resultTargetNames)
			}
		})
	}
}

func (suite *IngressWatcherTestSuite) TestGetPathFromIngress() {
	type testCase struct {
		name        string
		ingress     *networkingv1.Ingress
		expected    string
		expectError bool
		errorMsg    string
	}
	tests := []testCase{
		{
			name:     "Valid ingress with path",
			ingress:  suite.createDummyIngress("host", "/test", []string{"target"}),
			expected: "/test",
		},
		{
			name:        "Nil ingress",
			ingress:     nil,
			expectError: true,
			errorMsg:    "ingress is nil",
		},
		{
			name:        "No rules",
			ingress:     &networkingv1.Ingress{},
			expectError: true,
			errorMsg:    "no rules found in ingress",
		},
		{
			name: "Nil HTTP",
			ingress: &networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{{}},
				},
			},
			expectError: true,
			errorMsg:    "no HTTP configuration found in ingress rule",
		},
		{
			name: "No paths",
			ingress: &networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "no paths found in ingress HTTP rule",
		},
		{
			name: "Empty path",
			ingress: &networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{Path: ""},
									},
								},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "path is empty in ingress HTTP rule",
		},
	}

	watcher, _ := suite.createTestIngressWatcher()
	for _, tc := range tests {
		suite.Run(tc.name, func() {
			path, err := watcher.getPathFromIngress(tc.ingress)
			if tc.expectError {
				suite.Require().Error(err)
				suite.Require().ErrorContains(err, tc.errorMsg)
				suite.Require().Empty(path)
			} else {
				suite.Require().NoError(err)
				suite.Require().Equal(tc.expected, path)
			}
		})
	}
}

func (suite *IngressWatcherTestSuite) TestGetHostFromIngress() {
	for _, testCase := range []struct {
		name        string
		ingress     *networkingv1.Ingress
		expected    string
		expectError bool
		errorMsg    string
	}{
		{
			name:     "Valid ingress with host",
			ingress:  suite.createDummyIngress("test-host", "/test", []string{"target"}),
			expected: "test-host",
		},
		{
			name:        "Nil ingress",
			ingress:     nil,
			expectError: true,
			errorMsg:    "ingress is nil",
		},
		{
			name:        "No rules",
			ingress:     &networkingv1.Ingress{},
			expectError: true,
			errorMsg:    "no rules found in ingress",
		},
		{
			name: "Empty host",
			ingress: &networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{
						{
							IngressRuleValue: networkingv1.IngressRuleValue{
								HTTP: &networkingv1.HTTPIngressRuleValue{
									Paths: []networkingv1.HTTPIngressPath{
										{Path: "/test"},
									},
								},
							},
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "host is empty in ingress rule",
		},
	} {
		testIngressWatcher, err := suite.createTestIngressWatcher()
		suite.Require().NoError(err)

		suite.Run(testCase.name, func() {
			host, err := testIngressWatcher.getHostFromIngress(testCase.ingress)
			if testCase.expectError {
				suite.Require().Error(err)
				suite.Require().ErrorContains(err, testCase.errorMsg)
				suite.Require().Empty(host)
			} else {
				suite.Require().NoError(err)
				suite.Require().Equal(testCase.expected, host)
			}
		})

	}
}

// --- IngressWatcherTestSuite suite methods ---

// Create a dummy IngressWatcher for testing
func (suite *IngressWatcherTestSuite) createTestIngressWatcher() (*IngressWatcher, error) {

	ctx := context.Background()

	return NewIngressWatcher(ctx,
		suite.logger,
		suite.kubeClientSet,
		ingresscache.NewIngressCache(suite.logger),
		suite.createMockResolveFunc(),
		"test-namespace",
		"test-labels-filter",
	)
}

func (suite *IngressWatcherTestSuite) createMockResolveFunc() ResolveTargetsFromIngressCallback {
	return func(ingress *networkingv1.Ingress) ([]string, error) {
		// Extract targets from ingress - matches createDummyIngress structure (1 rule, 1 path)
		if len(ingress.Spec.Rules) != 1 {
			return []string{}, nil
		}

		rule := ingress.Spec.Rules[0]
		if rule.HTTP == nil {
			return []string{}, nil
		}

		if len(rule.HTTP.Paths) != 1 {
			return []string{}, nil
		}

		path := rule.HTTP.Paths[0]
		if path.Backend.Service == nil {
			return []string{}, nil
		}

		target := path.Backend.Service.Name
		// targets might have ',' as a delimiter in the annotation case
		return strings.Split(target, ","), nil
	}
}

// createDummyIngress Creates a dummy Ingress object for testing
func (suite *IngressWatcherTestSuite) createDummyIngress(host, path string, targets []string) *networkingv1.Ingress {
	target := strings.Join(targets, ",")
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ingress",
			Namespace: "test-namespace",
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					Host: host,
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path: path,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: target,
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
