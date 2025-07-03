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
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/nuclio/logger"
	nucliozap "github.com/nuclio/zap"
	"github.com/stretchr/testify/suite"
)

type IngressCacheTestSuite struct {
	suite.Suite
	logger logger.Logger
}

type testIngressCacheArgs struct {
	host    string
	path    string
	targets []string
}

type ingressCacheTestInitialState testIngressCacheArgs

func (suite *IngressCacheTestSuite) SetupTest() {
	var err error

	suite.logger, err = nucliozap.NewNuclioZapTest("test")
	suite.Require().NoError(err)
}

func (suite *IngressCacheTestSuite) TestGet() {
	suite.T().Parallel()
	for _, testCase := range []struct {
		name           string
		initialState   []ingressCacheTestInitialState
		args           testIngressCacheArgs
		expectedResult []string
		expectError    bool
		errorMessage   string
	}{
		{
			name:           "Get two targetNames",
			args:           testIngressCacheArgs{"example.com", "/test/path", []string{}},
			expectedResult: []string{"test-target-name-1", "test-target-name-2"},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", []string{"test-target-name-1", "test-target-name-2"}},
			},
		}, {
			name:           "Get single targetNames",
			args:           testIngressCacheArgs{"example.com", "/test/path", []string{}},
			expectedResult: []string{"test-target-name-1"},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", []string{"test-target-name-1"}},
			},
		}, {
			name:           "Get with empty cache",
			args:           testIngressCacheArgs{"not.exist", "/test/path", []string{}},
			expectedResult: nil,
			expectError:    true,
			errorMessage:   "host does not exist",
		}, {
			name:           "Get with not existing host",
			args:           testIngressCacheArgs{"not.exist", "/test/path", []string{}},
			expectedResult: nil,
			expectError:    true,
			errorMessage:   "host does not exist",
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", []string{"test-target-name-1"}},
			},
		}, {
			name:           "Get with not existing path",
			args:           testIngressCacheArgs{"example.com", "/not/exists/test/path", []string{}},
			expectedResult: nil,
			expectError:    true,
			errorMessage:   "failed to get the targets from the ingress host tree",
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", []string{"test-target-name-1"}},
			},
		},
	} {
		suite.Run(testCase.name, func() {
			testIngressCache := suite.getTestIngressCache(testCase.initialState)

			resultTargetNames, err := testIngressCache.Get(testCase.args.host, testCase.args.path)
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

func (suite *IngressCacheTestSuite) TestSet() {
	suite.T().Parallel()
	for _, testCase := range []struct {
		name           string
		initialState   []ingressCacheTestInitialState
		args           testIngressCacheArgs
		expectError    bool
		errorMessage   string
		expectedResult map[string]map[string]Target
	}{
		{
			name: "Set new host",
			args: testIngressCacheArgs{"example.com", "/test/path", []string{"test-target-name-1"}},
			expectedResult: map[string]map[string]Target{
				"example.com": {"/test/path": SingleTarget("test-target-name-1")},
			},
		}, {
			name: "Set unique targetNames that will override the existing host and path value",
			args: testIngressCacheArgs{"example.com", "/test/path", []string{"test-target-name-2"}},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", []string{"test-target-name-1"}},
			},
			expectedResult: map[string]map[string]Target{
				"example.com": {"/test/path": SingleTarget("test-target-name-2")},
			},
		}, {
			name: "Set existing targetNames for existing host and path",
			args: testIngressCacheArgs{"example.com", "/test/path", []string{"test-target-name-1"}},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", []string{"test-target-name-1"}},
			},
			expectedResult: map[string]map[string]Target{
				"example.com": {"/test/path": SingleTarget("test-target-name-1")},
			},
		}, {
			name: "Set another host and path",
			args: testIngressCacheArgs{"google.com", "/test/path", []string{"test-target-name-1"}},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", []string{"test-target-name-1"}},
			},
			expectedResult: map[string]map[string]Target{
				"google.com":  {"/test/path": SingleTarget("test-target-name-1")},
				"example.com": {"/test/path": SingleTarget("test-target-name-1")},
			},
		},
	} {
		suite.Run(testCase.name, func() {
			testIngressCache := suite.getTestIngressCache(testCase.initialState)

			err := testIngressCache.Set(testCase.args.host, testCase.args.path, testCase.args.targets)
			if testCase.expectError {
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
		expectError    bool
		errorMessage   string
		initialState   []ingressCacheTestInitialState
		expectedResult map[string]map[string]Target
	}{
		{
			name:           "Delete when cache is empty",
			args:           testIngressCacheArgs{"example.com", "/test/path", []string{"test-target-name-1"}},
			expectedResult: map[string]map[string]Target{},
		}, {
			name: "Delete not existed host",
			args: testIngressCacheArgs{"google.com", "/test/path", []string{"test-target-name-1"}},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", []string{"test-target-name-1"}},
			},
			expectedResult: map[string]map[string]Target{
				"example.com": {"/test/path": SingleTarget("test-target-name-1")},
			},
		}, {
			name: "Delete last targets in host, validate host deletion",
			args: testIngressCacheArgs{"example.com", "/test/path", []string{"test-target-name-1"}},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", []string{"test-target-name-1"}},
				{"google.com", "/test/path", []string{"test-target-name-1"}},
			},
			expectedResult: map[string]map[string]Target{
				"google.com": {"/test/path": SingleTarget("test-target-name-1")},
			},
		}, {
			name: "Delete not existing url and validate host wasn't deleted",
			args: testIngressCacheArgs{"example.com", "/not/exists/test/path", []string{"test-target-name-2"}},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", []string{"test-target-name-1"}},
			},
			expectedResult: map[string]map[string]Target{
				"example.com": {"/test/path": SingleTarget("test-target-name-1")},
			},
		}, {
			name: "Delete one target from multiple targets and validate that nothing was deleted",
			args: testIngressCacheArgs{"example.com", "/test/path", []string{"test-targets-name-2"}},
			initialState: []ingressCacheTestInitialState{
				{"example.com", "/test/path", []string{"test-target-name-1", "test-target-name-2"}},
			},
			expectedResult: map[string]map[string]Target{
				"example.com": {"/test/path": PairTarget{"test-target-name-1", "test-target-name-2"}},
			},
		},
	} {
		suite.Run(testCase.name, func() {
			testIngressCache := suite.getTestIngressCache(testCase.initialState)

			err := testIngressCache.Delete(testCase.args.host, testCase.args.path, testCase.args.targets)
			if testCase.expectError {
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

// --- IngressCacheTestSuite flow tests ---

func (suite *IngressCacheTestSuite) TestAllThreeMainFunctionalitiesWithTheSameHostAndPath() {
	// This test verifies the flow of setting targets in an empty IngressCache, then getting it, and finally deleting it.
	// It ensures that the IngressCache behaves correctly when performing these operations sequentially.

	testIngressCache := suite.getTestIngressCache([]ingressCacheTestInitialState{})
	var err error
	var getResult []string

	// get when cache is empty
	getResult, err = testIngressCache.Get("example.com", "/test/path")
	suite.Require().Error(err)
	suite.Require().ErrorContains(err, "cache get failed: host does not exist")
	suite.Require().Nil(getResult, "Expected no targets for empty cache")

	// Set targets in an empty cache
	err = testIngressCache.Set("example.com", "/test/path", []string{"test-target-name-1"})
	suite.Require().NoError(err, "Expected no error when setting targets in an empty cache")
	getResult, err = testIngressCache.Get("example.com", "/test/path")
	suite.Require().NoError(err, "Expected no error when getting targets after setting")
	suite.Require().Equal([]string{"test-target-name-1"}, getResult, "Expected to get the targets that was just set")
	flattenTestResult := suite.flattenIngressCache(testIngressCache)
	suite.Require().Equal(flattenTestResult, map[string]map[string]Target{
		"example.com": {"/test/path": SingleTarget("test-target-name-1")},
	})

	// Set another targets for the same host and path
	err = testIngressCache.Set("example.com", "/test/path", []string{"test-target-name-1", "test-target-name-2"})
	suite.Require().NoError(err, "Expected no error when setting another targets for the same host and path")
	getResult, err = testIngressCache.Get("example.com", "/test/path")
	suite.Require().NoError(err, "Expected no error when getting targets after setting another targets")
	suite.Require().Equal([]string{"test-target-name-1", "test-target-name-2"}, getResult, "Expected to get the new targets that was just set")
	flattenTestResult = suite.flattenIngressCache(testIngressCache)
	suite.Require().Equal(flattenTestResult, map[string]map[string]Target{
		"example.com": {"/test/path": PairTarget{"test-target-name-1", "test-target-name-2"}},
	})

	// Delete only the first target name shouldn't delete anything
	err = testIngressCache.Delete("example.com", "/test/path", []string{"test-target-name-1"})
	suite.Require().NoError(err, "Expected no error when deleting the first targets name")
	getResult, err = testIngressCache.Get("example.com", "/test/path")
	suite.Require().NoError(err, "Expected no error when getting targets after deleting the first targets name")
	suite.Require().Equal(getResult, []string{"test-target-name-1", "test-target-name-2"}, "Expected to get the remaining targets after deletion")
	flattenTestResult = suite.flattenIngressCache(testIngressCache)
	suite.Require().Equal(flattenTestResult, map[string]map[string]Target{
		"example.com": {"/test/path": PairTarget{"test-target-name-1", "test-target-name-2"}},
	})

	// Delete the first and second target's names should delete the targetTarget, validate that the cache is empty
	err = testIngressCache.Delete("example.com", "/test/path", []string{"test-target-name-1", "test-target-name-2"})
	suite.Require().NoError(err, "Expected no error when deleting an existing target")
	getResult, err = testIngressCache.Get("example.com", "/test/path")
	suite.Require().Error(err)
	suite.Require().ErrorContains(err, "cache get failed: host does not exist")
	suite.Require().Nil(getResult, "Expected no targets for empty cache")
	flattenTestResult = suite.flattenIngressCache(testIngressCache)
	suite.Require().Equal(flattenTestResult, map[string]map[string]Target{})
}

func (suite *IngressCacheTestSuite) TestParallelSetForTheSameHostAndDifferentPath() {
	// This test simulates a scenario where multiple goroutines try to set the same host and different paths in the IngressCache concurrently.
	// The expected behavior is that the IngressCache should handle concurrent writes without any errors and end up with a target for each path.

	testIngressCache := suite.getTestIngressCache([]ingressCacheTestInitialState{})
	wg := sync.WaitGroup{}
	for i := range 20 {
		wg.Add(2)
		path := fmt.Sprintf("/test/path/%d", i)

		// first goroutine set targetTarget
		go func(ingressCache *IngressCache, wg *sync.WaitGroup, path string) {
			defer wg.Done()
			err := ingressCache.Set("example.com", path, []string{"test-target-name-1", "test-target-name-2"})
			suite.Require().NoError(err, "Expected no error when setting targets in an empty cache")
		}(testIngressCache, &wg, path)

		// second goroutine set targetTarget
		go func(ingressCache *IngressCache, wg *sync.WaitGroup, path string) {
			defer wg.Done()
			err := ingressCache.Set("example.com", path, []string{"test-target-name-1", "test-target-name-2"})
			suite.Require().NoError(err, "Expected no error when setting targets in an empty cache")
		}(testIngressCache, &wg, path)
	}
	wg.Wait()

	// After all goroutines finished, check that the expected result matches the IngressCache state
	flattenTestResult := suite.flattenIngressCache(testIngressCache)
	expectedResult := suite.generateExpectedResult(20, false)
	suite.compareIngressHostCache(expectedResult, flattenTestResult)
}

func (suite *IngressCacheTestSuite) TestParallelSetForDifferentHosts() {
	// This test simulates a scenario where multiple goroutines try to set different hosts and paths in the IngressCache concurrently.
	// The expected behavior is that the IngressCache should handle concurrent writes without any errors and end up with a target for each host and path.

	testIngressCache := suite.getTestIngressCache([]ingressCacheTestInitialState{})
	wg := sync.WaitGroup{}
	for i := range 200 {
		wg.Add(2)
		host := fmt.Sprintf("example-%d.com", i)
		path := fmt.Sprintf("/test/path/%d", i)

		// first goroutine set
		go func(ingressCache *IngressCache, wg *sync.WaitGroup, host, path string) {
			defer wg.Done()
			err := ingressCache.Set(host, path, []string{"test-target-name-1", "test-target-name-2"})
			suite.Require().NoError(err, "Expected no error when setting targets in an empty cache")
		}(testIngressCache, &wg, host, path)

		// second goroutine set
		go func(ingressCache *IngressCache, wg *sync.WaitGroup, host, path string) {
			defer wg.Done()
			err := ingressCache.Set(host, path, []string{"test-target-name-1", "test-target-name-2"})
			suite.Require().NoError(err, "Expected no error when setting targets in an empty cache")
		}(testIngressCache, &wg, host, path)
	}
	wg.Wait()

	// After all goroutines finished, check that the expected result matches the IngressCache state
	flattenTestResult := suite.flattenIngressCache(testIngressCache)
	expectedResult := suite.generateExpectedResult(200, true)
	suite.compareIngressHostCache(expectedResult, flattenTestResult)
}

func (suite *IngressCacheTestSuite) TestParallelSetForSameHostAndSamePath() {
	// This test simulates a scenario where multiple goroutines try to set the same host and path in the IngressCache concurrently.
	// The expected behavior is that the IngressCache should handle concurrent writes without any errors and end up with a single entry for the host and path

	testIngressCache := suite.getTestIngressCache([]ingressCacheTestInitialState{})
	wg := sync.WaitGroup{}
	for range 20 {
		wg.Add(2)

		// first goroutine set
		go func(ingressCache *IngressCache, wg *sync.WaitGroup) {
			defer wg.Done()
			err := ingressCache.Set("example.com", "/test/path", []string{"test-target-name-1", "test-target-name-2"})
			suite.Require().NoError(err, "Expected no error when setting targets in an empty cache")
		}(testIngressCache, &wg)

		// second goroutine set
		go func(ingressCache *IngressCache, wg *sync.WaitGroup) {
			defer wg.Done()
			err := ingressCache.Set("example.com", "/test/path", []string{"test-target-name-1", "test-target-name-2"})
			suite.Require().NoError(err, "Expected no error when setting targets in an empty cache")
		}(testIngressCache, &wg)
	}
	wg.Wait()

	// After all goroutines finished, check that the expected result matches the IngressCache state
	flattenTestResult := suite.flattenIngressCache(testIngressCache)
	expectedResult := map[string]map[string]Target{
		"example.com": {
			"/test/path": PairTarget{"test-target-name-1", "test-target-name-2"},
		},
	}
	suite.compareIngressHostCache(expectedResult, flattenTestResult)
}

// --- IngressCacheTestSuite suite methods ---

// getTestIngressCache creates an IngressCache instance and sets the provided initial state
func (suite *IngressCacheTestSuite) getTestIngressCache(initialState []ingressCacheTestInitialState) *IngressCache {
	var err error
	ingressCache := NewIngressCache(suite.logger)

	// Set the initial state in the IngressCache
	for _, args := range initialState {
		err = ingressCache.Set(args.host, args.path, args.targets)
		suite.Require().NoError(err)
	}

	return ingressCache
}

// flattenIngressCache flattens the IngressCache's syncMap into a map for easier comparison in tests
func (suite *IngressCacheTestSuite) flattenIngressCache(testIngressCache *IngressCache) map[string]map[string]Target {
	output := make(map[string]map[string]Target)
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

func (suite *IngressCacheTestSuite) generateExpectedResult(num int, differentHosts bool) map[string]map[string]Target {
	output := make(map[string]map[string]Target)
	for i := range num {
		path := fmt.Sprintf("/test/path/%d", i)
		host := "example.com"
		if differentHosts {
			host = fmt.Sprintf("example-%d.com", i)
		}

		if output[host] == nil {
			output[host] = map[string]Target{}
		}

		output[host][path] = PairTarget{"test-target-name-1", "test-target-name-2"}
	}

	return output
}

// compareIngressHostCache compares the expected result with the test result
func (suite *IngressCacheTestSuite) compareIngressHostCache(expectedResult, testResult map[string]map[string]Target) {
	suite.Require().Equal(len(expectedResult), len(testResult))

	// Because the values in the map are pointers, we need to compare the values
	for host, paths := range testResult {
		suite.Require().Contains(expectedResult, host, "Expected host %s to be in the result", host)
		for path, targetNames := range paths {
			suite.Require().Contains(expectedResult[host], path, "Expected path %s to be in the result for host %s", path, host)
			expectedTargetNames := expectedResult[host][path].ToSliceString()
			slices.Sort(expectedTargetNames)
			sortedTargetNames := targetNames.ToSliceString()
			slices.Sort(sortedTargetNames)
			suite.Require().Equal(expectedTargetNames, sortedTargetNames,
				"Expected targets for host %s and path %s to match", host, path)
		}
	}
}

func TestIngressCacheSuite(t *testing.T) {
	suite.Run(t, new(IngressCacheTestSuite))
}
