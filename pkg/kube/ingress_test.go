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

	"github.com/v3io/scaler/pkg/scalertypes"
	"github.com/v3io/scaler/pkg/scalertypes"

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

// expectedResult is a struct for representing the expected result of a test case
type expectedResult struct {
	host           string
	path           string
	targets        []string
	expectError    bool
	expectErrorMsg string
}

func (suite *IngressWatcherTestSuite) SetupSuite() {
	suite.kubeClientSet = fake.NewSimpleClientset()
}

func (suite *IngressWatcherTestSuite) SetupTest() {
	var err error

	suite.logger, err = nucliozap.NewNuclioZapTest("test")
	suite.Require().NoError(err)
}

func (suite *IngressWatcherTestSuite) TestAddHandler() {
	for _, testCase := range []struct {
		name              string
		testArgs          ingressValue
		expectedResult    []string
		initialCachedData *ingressValue
		errorMessage      string
		expectError       bool
	}{
		{
			name: "Add PairTarget",
			testArgs: ingressValue{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			expectedResult: []string{"test-targets-name-1", "test-targets-name-2"},
		}, {
			name: "Add SingleTarget",
			testArgs: ingressValue{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1"},
			},
			expectedResult: []string{"test-targets-name-1"},
		}, {
			name: "Add SingleTarget with different name to the same host and path",
			testArgs: ingressValue{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-2"},
			},
			expectedResult: []string{"test-targets-name-2"},
			initialCachedData: &ingressValue{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1"},
			},
		}, {
			name: "bad input- should fail",
			testArgs: ingressValue{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-2"},
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

			testObj = suite.createDummyIngress(testCase.testArgs.host, testCase.testArgs.path, "1", testCase.testArgs.targets)

			if testCase.expectError {
				testObj = &networkingv1.IngressSpec{}
			}

			if testCase.initialCachedData != nil {
				err = testIngressWatcher.cache.Set(testCase.initialCachedData.host, testCase.initialCachedData.path, testCase.initialCachedData.targets)
				suite.Require().NoError(err)
			}

			testIngressWatcher.AddHandler(testObj)
			// get the targets from the cache and compare values
			resultTargetNames, err := testIngressWatcher.cache.Get(testCase.testArgs.host, testCase.testArgs.path)
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

func (suite *IngressWatcherTestSuite) TestUpdateHandler() {
	for _, testCase := range []struct {
		name              string
		expectedResults   []expectedResult
		initialCachedData *ingressValue
		testOldObj        ingressValue
		testNewObj        ingressValue
		OldObjVersion     string
		newObjVersion     string
	}{
		{
			name: "Update PairTarget - same host and path, different targets",
			expectedResults: []expectedResult{
				{
					host:    "www.example.com",
					path:    "/test/path",
					targets: []string{"test-targets-name-1", "test-targets-name-3"},
				},
			},
			testOldObj: ingressValue{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			OldObjVersion: "1",
			testNewObj: ingressValue{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-3"},
			},
			newObjVersion: "2",
			initialCachedData: &ingressValue{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
		}, {
			name: "Update PairTarget - same ResourceVersion, no change in targets",
			expectedResults: []expectedResult{
				{
					host:    "www.example.com",
					path:    "/test/path",
					targets: []string{"test-targets-name-1", "test-targets-name-2"},
				},
			},
			testOldObj: ingressValue{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			testNewObj: ingressValue{
				host:    "www.example.com",
				path:    "/test/path",
				version: "1",
				targets: []string{"test-targets-name-1", "test-targets-name-3"},
			},
			OldObjVersion: "1",
			newObjVersion: "1",
			initialCachedData: &ingressValue{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
		}, {
			name: "Update PairTarget - different path- should Delete old targets",
			initialCachedData: &ingressValue{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			testOldObj: ingressValue{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			OldObjVersion: "1",
			testNewObj: ingressValue{
				host:    "www.example.com",
				path:    "/another/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			newObjVersion: "2",
			expectedResults: []expectedResult{
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
			name: "Update PairTarget - different host- should Delete old targets",
			initialCachedData: &ingressValue{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			testOldObj: ingressValue{
				host:    "www.example.com",
				path:    "/test/path",
				version: "1",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			OldObjVersion: "1",
			testNewObj: ingressValue{
				host:    "www.google.com",
				path:    "/test/path",
				version: "2",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			newObjVersion: "2",
			expectedResults: []expectedResult{
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

			testOldObj := suite.createDummyIngress(testCase.testOldObj.host, testCase.testOldObj.path, testCase.OldObjVersion, testCase.testOldObj.targets)
			testNewObj := suite.createDummyIngress(testCase.testNewObj.host, testCase.testNewObj.path, testCase.newObjVersion, testCase.testNewObj.targets)

			if testCase.initialCachedData != nil {
				err = testIngressWatcher.cache.Set(testCase.initialCachedData.host, testCase.initialCachedData.path, testCase.initialCachedData.targets)
				suite.Require().NoError(err)
			}

			testIngressWatcher.UpdateHandler(testOldObj, testNewObj)

			// iterate over the expectedResults to check the results for each host and path
			for _, expectedResult := range testCase.expectedResults {
				resultTargetNames, err := testIngressWatcher.cache.Get(expectedResult.host, expectedResult.path)
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

func (suite *IngressWatcherTestSuite) TestDeleteHandler() {
	for _, testCase := range []struct {
		name              string
		testArgs          ingressValue
		expectedResult    expectedResult
		initialCachedData *ingressValue
		errorMessage      string
		expectError       bool
	}{
		{
			name: "Delete PairTarget",
			initialCachedData: &ingressValue{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1", "test-targets-name-2"},
			},
			testArgs: ingressValue{
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
			initialCachedData: &ingressValue{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1"},
			},
			testArgs: ingressValue{
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
			initialCachedData: &ingressValue{
				host:    "www.example.com",
				path:    "/test/path",
				targets: []string{"test-targets-name-1"},
			},
			testArgs: ingressValue{
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

			testObj = suite.createDummyIngress(testCase.testArgs.host, testCase.testArgs.path, "1", testCase.testArgs.targets)

			if testCase.expectError {
				testObj = &networkingv1.IngressSpec{}
			}

			if testCase.initialCachedData != nil {
				err = testIngressWatcher.cache.Set(testCase.initialCachedData.host, testCase.initialCachedData.path, testCase.initialCachedData.targets)
				suite.Require().NoError(err)
			}

			testIngressWatcher.DeleteHandler(testObj)
			// get the targets from the cache and compare values
			resultTargetNames, err := testIngressWatcher.cache.Get(testCase.expectedResult.host, testCase.expectedResult.path)
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
			ingress:  suite.createDummyIngress("host", "/test", "1", []string{"target"}),
			expected: "/test",
		},
		{
			name:        "Nil ingress",
			ingress:     nil,
			expectError: true,
			errorMsg:    "Failed to get first rule from ingress",
		},
		{
			name:        "No rules",
			ingress:     &networkingv1.Ingress{},
			expectError: true,
			errorMsg:    "Failed to get first rule from ingress",
		},
		{
			name: "Nil HTTP",
			ingress: &networkingv1.Ingress{
				Spec: networkingv1.IngressSpec{
					Rules: []networkingv1.IngressRule{{}},
				},
			},
			expectError: true,
			errorMsg:    "No HTTP configuration found in ingress rule",
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
			errorMsg:    "No paths found in ingress HTTP path",
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
			errorMsg:    "Path is empty in the first ingress HTTP path",
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
			ingress:  suite.createDummyIngress("test-host", "/test", "1", []string{"target"}),
			expected: "test-host",
		},
		{
			name:        "Nil ingress",
			ingress:     nil,
			expectError: true,
			errorMsg:    "Failed to get first rule from ingress",
		},
		{
			name:        "No rules",
			ingress:     &networkingv1.Ingress{},
			expectError: true,
			errorMsg:    "Failed to get first rule from ingress",
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
			errorMsg:    "Host is empty in ingress rule",
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
		suite.createMockResolveFunc(),
		scalertypes.Duration{},
		"test-namespace",
		"test-labels-filter",
	)
}

func (suite *IngressWatcherTestSuite) createMockResolveFunc() scalertypes.ResolveTargetsFromIngressCallback {
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
func (suite *IngressWatcherTestSuite) createDummyIngress(host, path, version string, targets []string) *networkingv1.Ingress {
	target := strings.Join(targets, ",")
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-ingress",
			Namespace:       "test-namespace",
			ResourceVersion: version,
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

// TestIngressWatcherTestSuite runs the test suite
func TestIngressWatcherTestSuite(t *testing.T) {
	suite.Run(t, new(IngressWatcherTestSuite))
}
