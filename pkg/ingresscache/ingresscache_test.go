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

package ingresscache

import (
	"testing"

	nucliozap "github.com/nuclio/zap"
	"github.com/stretchr/testify/suite"
)

type IngressCacheTestSuite struct {
	suite.Suite
}

type testIngressCacheArgs struct {
	host     string
	path     string
	function string
}

type ingressCacheTestInitialState testIngressCacheArgs

func (suite *IngressCacheTestSuite) TestGet() {
	suite.T().Parallel()
	for _, testCase := range []struct {
		name           string
		initialState   []ingressCacheTestInitialState
		args           testIngressCacheArgs
		expectedResult []string
		shouldFail     bool
		errorMessage   string
	}{
		{
			name:           "Get two functionName",
			args:           testIngressCacheArgs{"example.com", "/test/path", ""},
			expectedResult: []string{"test-function-name-1", "test-function-name-2"},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", "test-function-name-1"},
				{"example.com", "/test/path", "test-function-name-2"},
			},
		}, {
			name:           "Get single functionName",
			args:           testIngressCacheArgs{"example.com", "/test/path", ""},
			expectedResult: []string{"test-function-name-1"},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", "test-function-name-1"},
			},
		}, {
			name:           "Get multiple functionName",
			args:           testIngressCacheArgs{"example.com", "/test/path", ""},
			expectedResult: []string{"test-function-name-1", "test-function-name-2"},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", "test-function-name-1"},
				{"example.com", "/test/path", "test-function-name-2"},
			},
		}, {
			name:           "Get with empty cache",
			args:           testIngressCacheArgs{"not.exist", "/test/path", ""},
			expectedResult: nil,
			shouldFail:     true,
			errorMessage:   "host does not exist",
		}, {
			name:           "Get with not existing host",
			args:           testIngressCacheArgs{"not.exist", "/test/path", ""},
			expectedResult: nil,
			shouldFail:     true,
			errorMessage:   "host does not exist",
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", "test-function-name-1"},
			},
		}, {
			name:           "Get with not existing path",
			args:           testIngressCacheArgs{"example.com", "/not/exists/test/path", ""},
			expectedResult: nil,
			shouldFail:     true,
			errorMessage:   "failed to get the function name from the ingress host tree",
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", "test-function-name-1"},
			},
		},
	} {
		suite.Run(testCase.name, func() {
			testIngressCache := suite.getTestIngressCache(testCase.initialState)

			resultFunctionNames, err := testIngressCache.Get(testCase.args.host, testCase.args.path)
			if testCase.shouldFail {
				suite.Require().Error(err)
				suite.Require().ErrorContains(err, testCase.errorMessage)
				suite.Require().Nil(resultFunctionNames)
			} else {
				suite.Require().NoError(err)
				suite.Require().Equal(testCase.expectedResult, resultFunctionNames)
			}
		})
	}
}

func (suite *IngressCacheTestSuite) TestSet() {
	suite.T().Parallel()
	for _, testCase := range []struct {
		name           string
		initialState   []ingressCacheTestInitialState
		args           testIngressCacheArgs
		shouldFail     bool
		errorMessage   string
		expectedResult map[string]map[string][]string
	}{
		{
			name: "Set new host",
			args: testIngressCacheArgs{"example.com", "/test/path", "test-function-name-1"},
			expectedResult: map[string]map[string][]string{
				"example.com": {"/test/path": {"test-function-name-1"}},
			},
		}, {
			name: "Set another functionName for existing host",
			args: testIngressCacheArgs{"example.com", "/test/path", "test-function-name-2"},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", "test-function-name-1"},
			},
			expectedResult: map[string]map[string][]string{
				"example.com": {"/test/path": {"test-function-name-1", "test-function-name-2"}},
			},
		}, {
			name: "Set existing functionName for existing host and path",
			args: testIngressCacheArgs{"example.com", "/test/path", "test-function-name-1"},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", "test-function-name-1"},
			},
			expectedResult: map[string]map[string][]string{
				"example.com": {"/test/path": {"test-function-name-1"}},
			},
		}, {
			name: "Set another host and path",
			args: testIngressCacheArgs{"google.com", "/test/path", "test-function-name-1"},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", "test-function-name-1"},
			},
			expectedResult: map[string]map[string][]string{
				"google.com":  {"/test/path": {"test-function-name-1"}},
				"example.com": {"/test/path": {"test-function-name-1"}},
			},
		},
	} {
		suite.Run(testCase.name, func() {
			testIngressCache := suite.getTestIngressCache(testCase.initialState)

			err := testIngressCache.Set(testCase.args.host, testCase.args.path, testCase.args.function)
			if testCase.shouldFail {
				suite.Require().Error(err)
				suite.Require().ErrorContains(err, testCase.errorMessage)
			} else {
				suite.Require().NoError(err)
			}

			// After delete, check that the expected result matches the IngressCache state
			testResult := suite.flattenIngressCache(testIngressCache)
			suite.Require().NoError(err)
			suite.Require().Equal(testCase.expectedResult, testResult)
		})
	}
}

func (suite *IngressCacheTestSuite) TestDelete() {
	suite.T().Parallel()
	for _, testCase := range []struct {
		name           string
		args           testIngressCacheArgs
		shouldFail     bool
		errorMessage   string
		initialState   []ingressCacheTestInitialState
		expectedResult map[string]map[string][]string
	}{
		{
			name:           "Delete when cache is empty",
			args:           testIngressCacheArgs{"example.com", "/test/path", "test-function-name-1"},
			expectedResult: map[string]map[string][]string{},
		}, {
			name: "Delete not existed host",
			args: testIngressCacheArgs{"google.com", "/test/path", "test-function-name-1"},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", "test-function-name-1"},
			},
			expectedResult: map[string]map[string][]string{
				"example.com": {"/test/path": {"test-function-name-1"}},
			},
		}, {
			name: "Delete last function in host, validate host deletion",
			args: testIngressCacheArgs{"example.com", "/test/path", "test-function-name-1"},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", "test-function-name-1"},
				{"google.com", "/test/path", "test-function-name-1"},
			},
			expectedResult: map[string]map[string][]string{
				"google.com": {"/test/path": {"test-function-name-1"}},
			},
		}, {
			name: "Delete not existing url and validate host wasn't deleted",
			args: testIngressCacheArgs{"example.com", "/not/exists/test/path", "test-function-name-2"},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", "test-function-name-1"},
			},
			expectedResult: map[string]map[string][]string{
				"example.com": {"/test/path": {"test-function-name-1"}},
			},
		}, {
			name: "Delete not last function in path and validate host wasn't deleted",
			args: testIngressCacheArgs{"example.com", "/test/path", "test-function-name-2"},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", "test-function-name-1"},
				{"example.com", "/test/path", "test-function-name-2"},
			},
			expectedResult: map[string]map[string][]string{
				"example.com": {"/test/path": {"test-function-name-1"}},
			},
		},
	} {
		suite.Run(testCase.name, func() {
			testIngressCache := suite.getTestIngressCache(testCase.initialState)

			err := testIngressCache.Delete(testCase.args.host, testCase.args.path, testCase.args.function)
			if testCase.shouldFail {
				suite.Require().Error(err)
				suite.Require().ErrorContains(err, testCase.errorMessage)
			} else {
				suite.Require().NoError(err)
			}

			// After delete, check that the expected result matches the IngressCache state
			testResult := suite.flattenIngressCache(testIngressCache)
			suite.Require().NoError(err)
			suite.Require().Equal(testCase.expectedResult, testResult)
		})
	}
}

// --- IngressCacheTestSuite suite methods ---

// getTestIngressCache creates a IngressCache instance and sets the provided initial state
func (suite *IngressCacheTestSuite) getTestIngressCache(initialState []ingressCacheTestInitialState) *IngressCache {
	testLogger, err := nucliozap.NewNuclioZapTest("test")
	suite.Require().NoError(err)
	ingressCache := NewIngressCache(testLogger)

	// Set the initial state in the IngressCache
	for _, args := range initialState {
		err = ingressCache.Set(args.host, args.path, args.function)
		suite.Require().NoError(err)
	}

	return ingressCache
}

// flattenIngressCache flattens the IngressCache's syncMap into a map for easier comparison in tests
func (suite *IngressCacheTestSuite) flattenIngressCache(testIngressCache *IngressCache) map[string]map[string][]string {
	output := make(map[string]map[string][]string)
	testIngressCache.syncMap.Range(func(key, value interface{}) bool {
		safeTrie, ok := value.(*SafeTrie)
		suite.Require().True(ok)
		flatSafeTrie, err := flattenSafeTrie(safeTrie)
		suite.Require().NoError(err)
		output[key.(string)] = flatSafeTrie
		return true
	})

	return output
}

func TestIngressCache(t *testing.T) {
	suite.Run(t, new(IngressCacheTestSuite))
}
