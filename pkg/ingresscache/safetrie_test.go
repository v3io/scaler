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
	"testing"

	"github.com/stretchr/testify/suite"
)

type SafeTrieTestSuite struct {
	suite.Suite
}

type safeTrieFunctionArgs struct {
	path      string
	functions []string
}

func (suite *SafeTrieTestSuite) TestPathTreeSet() {
	suite.T().Parallel()
	for _, testCase := range []struct {
		name           string
		args           []safeTrieFunctionArgs
		expectedResult map[string]FunctionTarget
		expectError    bool
		errorMessage   string
	}{
		{
			name: "simple set",
			args: []safeTrieFunctionArgs{
				{
					path:      "/path/to/functions",
					functions: []string{"test-functions"},
				},
			},
			expectedResult: map[string]FunctionTarget{"/path/to/functions": SingleTarget("test-functions")},
		}, {
			name: "idempotent test",
			args: []safeTrieFunctionArgs{
				{
					path:      "/path/to/functions",
					functions: []string{"test-functions"},
				}, {
					path:      "/path/to/functions",
					functions: []string{"test-functions"},
				},
			},
			expectedResult: map[string]FunctionTarget{"/path/to/functions": SingleTarget("test-functions")},
		}, {
			name: "set twice the same path with a different functions",
			args: []safeTrieFunctionArgs{
				{
					path:      "/path/to/functions",
					functions: []string{"test-functions"},
				}, {
					path:      "/path/to/functions",
					functions: []string{"test-function2"},
				},
			},
			expectedResult: map[string]FunctionTarget{"/path/to/functions": SingleTarget("test-function2")},
		}, {
			name: "set nested paths and different functions",
			args: []safeTrieFunctionArgs{
				{
					path:      "/path/to/functions",
					functions: []string{"test-functions"},
				}, {
					path:      "/path/to/functions/nested",
					functions: []string{"test-function2"},
				},
			},
			expectedResult: map[string]FunctionTarget{
				"/path/to/functions":        SingleTarget("test-functions"),
				"/path/to/functions/nested": SingleTarget("test-function2"),
			},
		}, {
			name: "set different paths and different functions",
			args: []safeTrieFunctionArgs{
				{
					path:      "/path/to/functions",
					functions: []string{"test-functions"},
				}, {
					path:      "/another/path/to/functions/",
					functions: []string{"test-function2"},
				},
			},
			expectedResult: map[string]FunctionTarget{
				"/path/to/functions":          SingleTarget("test-functions"),
				"/another/path/to/functions/": SingleTarget("test-function2"),
			},
		}, {
			name: "empty functions name",
			args: []safeTrieFunctionArgs{
				{
					path:      "/path/to/functions",
					functions: []string{},
				},
			},
			expectedResult: map[string]FunctionTarget{},
			expectError:    true,
			errorMessage:   "failed to create FunctionTarget",
		}, {
			name: "empty path",
			args: []safeTrieFunctionArgs{
				{
					path:      "",
					functions: []string{"test-functions"},
				},
			},
			expectedResult: map[string]FunctionTarget{},
			expectError:    true,
			errorMessage:   "path is empty",
		}, {
			name: "double slash in path",
			args: []safeTrieFunctionArgs{
				{
					path:      "///path/to/functions",
					functions: []string{"test-functions"},
				},
			},
			expectedResult: map[string]FunctionTarget{
				"///path/to/functions": SingleTarget("test-functions"),
			},
		}, {
			name: "path starts without slash",
			args: []safeTrieFunctionArgs{
				{
					path:      "path/to/functions",
					functions: []string{"test-functions"},
				},
			},
			expectedResult: map[string]FunctionTarget{
				"path/to/functions": SingleTarget("test-functions"),
			},
		}, {
			name:           "lots of paths and functions",
			args:           suite.generatePathsAndFunctions(200),
			expectedResult: suite.generateExpectedResultMap(200),
		}, {
			name:           "path ends with slash",
			args:           []safeTrieFunctionArgs{{path: "/path/to/functions/", functions: []string{"test-functions"}}},
			expectedResult: map[string]FunctionTarget{"/path/to/functions/": SingleTarget("test-functions")},
		}, {
			name:           "path with dots",
			args:           []safeTrieFunctionArgs{{path: "/path/./to/./functions/", functions: []string{"test-functions"}}},
			expectedResult: map[string]FunctionTarget{"/path/./to/./functions/": SingleTarget("test-functions")},
		}, {
			name:           "upper case path",
			args:           []safeTrieFunctionArgs{{path: "/PATH/TO/functions", functions: []string{"test-functions"}}},
			expectedResult: map[string]FunctionTarget{"/PATH/TO/functions": SingleTarget("test-functions")},
		}, {
			name: "upper case functions name",
			args: []safeTrieFunctionArgs{
				{path: "/path/to/functions", functions: []string{"test-functions"}},
				{path: "/path/to/functions", functions: []string{"test-functions", "test-FUNCTION"}},
			},
			expectedResult: map[string]FunctionTarget{"/path/to/functions": &CanaryTarget{[2]string{"test-functions", "test-FUNCTION"}}},
		}, {
			name: "path with numbers and hyphens",
			args: []safeTrieFunctionArgs{
				{path: "/api/v1/user-data/123", functions: []string{"test-functions"}},
			},
			expectedResult: map[string]FunctionTarget{"/api/v1/user-data/123": SingleTarget("test-functions")},
		},
	} {
		suite.Run(testCase.name, func() {
			testSafeTrie := suite.generateSafeTrieForTest([]safeTrieFunctionArgs{})
			for _, setArgs := range testCase.args {
				err := testSafeTrie.Set(setArgs.path, setArgs.functions)
				if testCase.expectError {
					suite.Require().Error(err)
					suite.Require().ErrorContains(err, testCase.errorMessage)
				} else {
					suite.Require().NoError(err)
				}
			}
			testResult, err := flattenSafeTrie(testSafeTrie)
			suite.Require().NoError(err)
			suite.Require().Equal(testCase.expectedResult, testResult)
		})
	}
}
func (suite *SafeTrieTestSuite) TestPathTreeGet() {
	suite.T().Parallel()
	initialStateGetTest := []safeTrieFunctionArgs{
		{"/", []string{"test-functions"}},
		{"/path/to/function1", []string{"test-function1"}},
		{"/path/to/function1/nested", []string{"test-function2"}},
		{"/path/./to/./functions/", []string{"test-function1"}},
		{"path//to//functions/", []string{"test-function1"}},
		{"/path/to/multiple/functions", []string{"test-function1", "test-function2"}},
	}
	for _, testCase := range []struct {
		name           string
		path           string
		expectedResult FunctionTarget
		expectError    bool
		errorMessage   string
	}{
		{
			name:           "get root path",
			path:           "/",
			expectedResult: SingleTarget("test-functions"),
		}, {
			name:           "get regular path",
			path:           "/path/to/function1",
			expectedResult: SingleTarget("test-function1"),
		}, {
			name:           "get nested path",
			path:           "/path/to/function1/nested",
			expectedResult: SingleTarget("test-function2"),
		}, {
			name:           "get closest match",
			path:           "/path/to/function1/nested/extra",
			expectedResult: SingleTarget("test-function2"),
		}, {
			name:         "get empty path",
			path:         "",
			expectError:  true,
			errorMessage: "path is empty",
		}, {
			name:           "get closest match with different suffix",
			path:           "/path/to/function1/something/else",
			expectedResult: SingleTarget("test-function1"),
		}, {
			name:           "get path with dots",
			path:           "/path/./to/./functions/",
			expectedResult: SingleTarget("test-function1"),
		}, {
			name:           "get path with slash",
			path:           "path//to//functions/",
			expectedResult: SingleTarget("test-function1"),
		}, {
			name:           "get multiple functions for the same path",
			path:           "/path/to/multiple/functions",
			expectedResult: &CanaryTarget{[2]string{"test-function1", "test-function2"}},
		},
	} {
		suite.Run(testCase.name, func() {
			testSafeTrie := suite.generateSafeTrieForTest(initialStateGetTest)
			result, err := testSafeTrie.Get(testCase.path)
			if testCase.expectError {
				suite.Require().Error(err)
				suite.Require().ErrorContains(err, testCase.errorMessage)
			} else {
				suite.Require().NoError(err)
				suite.Require().Equal(testCase.expectedResult, result)
			}
		})
	}
}
func (suite *SafeTrieTestSuite) TestPathTreeDelete() {
	suite.T().Parallel()
	for _, testCase := range []struct {
		initialState   []safeTrieFunctionArgs // initial state of the path tree before delete
		name           string
		deleteArgs     safeTrieFunctionArgs
		expectedResult map[string]FunctionTarget
		expectError    bool
		errorMessage   string
	}{
		{
			name: "delete a path and validate that nested path is still there",
			initialState: []safeTrieFunctionArgs{
				{"/path/to/function1", []string{"test-function1"}},
				{"/path/to/function1/nested", []string{"test-function2"}},
			},
			deleteArgs: safeTrieFunctionArgs{"/path/to/function1", []string{"test-function1"}},
			expectedResult: map[string]FunctionTarget{
				"/path/to/function1/nested": SingleTarget("test-function2"),
			},
		}, {
			name: "delete a functions from multiple values shouldn't do anything, validate that the functions is still there",
			initialState: []safeTrieFunctionArgs{
				{"/path/to/multiple/functions", []string{"test-function1", "test-function2"}},
			},
			deleteArgs: safeTrieFunctionArgs{"/path/to/multiple/functions", []string{"test-function1"}},
			expectedResult: map[string]FunctionTarget{
				"/path/to/multiple/functions": &CanaryTarget{[2]string{"test-function1", "test-function2"}},
			},
		}, {
			name: "delete functions that does not exist in the path",
			initialState: []safeTrieFunctionArgs{
				{"/path/to/function1", []string{"test-function1"}},
			},
			deleteArgs: safeTrieFunctionArgs{"/path/to/function1", []string{"test-function2"}},
			expectedResult: map[string]FunctionTarget{
				"/path/to/function1": SingleTarget("test-function1"),
			},
		}, {
			name: "delete functions that does not exist in multiple value path",
			initialState: []safeTrieFunctionArgs{
				{"/path/to/functions", []string{"test-function1", "test-function2"}},
			},
			deleteArgs: safeTrieFunctionArgs{"/path/to/functions", []string{"test-not-existing-functions"}},
			expectedResult: map[string]FunctionTarget{
				"/path/to/functions": &CanaryTarget{[2]string{"test-function1", "test-function2"}},
			},
		}, {
			name: "delete not exist path",
			initialState: []safeTrieFunctionArgs{
				{"/path/to/function1", []string{"test-function1"}},
			},
			deleteArgs: safeTrieFunctionArgs{"/path/to/function1/nested", []string{"test-function2"}},
			expectedResult: map[string]FunctionTarget{
				"/path/to/function1": SingleTarget("test-function1"),
			},
		}, {
			name: "delete path with suffix that does not exist",
			initialState: []safeTrieFunctionArgs{
				{"/path/to/function1", []string{"test-function1"}},
				{"/path/to/function1/nested", []string{"test-function2"}},
			},
			deleteArgs: safeTrieFunctionArgs{"/path/to/function1/path/suffix", []string{"test-function1"}},
			expectedResult: map[string]FunctionTarget{
				"/path/to/function1":        SingleTarget("test-function1"),
				"/path/to/function1/nested": SingleTarget("test-function2"),
			},
		},
	} {
		suite.Run(testCase.name, func() {
			testSafeTrie := suite.generateSafeTrieForTest(testCase.initialState)

			err := testSafeTrie.Delete(testCase.deleteArgs.path, testCase.deleteArgs.functions)
			if testCase.expectError {
				suite.Require().Error(err)
				suite.Require().ErrorContains(err, testCase.errorMessage)
			} else {
				suite.Require().NoError(err)
			}

			// After delete, check that the expected paths and functions are as expected
			flattenSafeTrie, err := flattenSafeTrie(testSafeTrie)
			suite.Require().NoError(err)
			suite.Require().Equal(testCase.expectedResult, flattenSafeTrie)
		})
	}
}
func (suite *SafeTrieTestSuite) TestPathTreeIsEmpty() {
	suite.T().Parallel()
	for _, testCase := range []struct {
		initialState   []safeTrieFunctionArgs // initial state of the path tree before delete
		name           string
		expectedResult bool
	}{
		{
			name:           "is empty with empty trie",
			expectedResult: true,
		}, {
			name:           "is empty with not empty trie",
			expectedResult: false,
			initialState: []safeTrieFunctionArgs{
				{"/test/path/", []string{"test-function1"}},
			},
		},
	} {
		suite.Run(testCase.name, func() {
			testSafeTrie := suite.generateSafeTrieForTest(testCase.initialState)

			result := testSafeTrie.IsEmpty()
			suite.Require().Equal(testCase.expectedResult, result)
		})
	}
}

// --- SafeTrieTestSuite suite methods ---

// flattenSafeTrie converts a PathTrie into a map[string]FunctionTarget
// This functions is not part of the SafeTrieTestSuite because it is also in use in IngressCacheTestSuite
func flattenSafeTrie(st *SafeTrie) (map[string]FunctionTarget, error) {
	resultMap := make(map[string]FunctionTarget)
	err := st.pathTrie.Walk(func(key string, value interface{}) error {
		// The Walk functions iterates over all nodes.
		// Only store key-value pairs where a non-nil value has been explicitly 'Put'.
		// If a node exists as an internal prefix (e.g., "/a" for "/a/b"), its 'value' will be nil.
		// We only care about the values that were actually stored.
		if value != nil {
			convertedValue, ok := value.(FunctionTarget)
			if !ok {
				return fmt.Errorf("path value should be FunctionTarget")
			}
			resultMap[key] = convertedValue
		}
		return nil // Continue the walk
	})
	if err != nil {
		return nil, fmt.Errorf("error walking trie: %w", err)
	}
	return resultMap, nil
}

func (suite *SafeTrieTestSuite) generatePathsAndFunctions(num int) []safeTrieFunctionArgs {
	args := make([]safeTrieFunctionArgs, num)
	for i := 0; i < num; i++ {
		path := fmt.Sprintf("/path/to/functions/%d", i)
		functions := []string{fmt.Sprintf("functions-%d", i)}
		args[i] = safeTrieFunctionArgs{path: path, functions: functions}
	}
	return args
}

func (suite *SafeTrieTestSuite) generateExpectedResultMap(num int) map[string]FunctionTarget {
	expectedResult := make(map[string]FunctionTarget)
	args := suite.generatePathsAndFunctions(num)
	for i := 0; i < num; i++ {
		expectedResult[args[i].path] = SingleTarget(args[i].functions[0])
	}
	return expectedResult
}

// generateSafeTrieForTest creates a SafeTrie instance and sets the provided initial state
func (suite *SafeTrieTestSuite) generateSafeTrieForTest(initialSafeTrieState []safeTrieFunctionArgs) *SafeTrie {
	var err error
	safeTrie := NewSafeTrie()

	// set path tree with the provided required state
	for _, args := range initialSafeTrieState {
		err = safeTrie.Set(args.path, args.functions)
		suite.Require().NoError(err)
	}

	return safeTrie
}

func TestSafeTrie(t *testing.T) {
	suite.Run(t, new(SafeTrieTestSuite))
}

// --- SingleTargetTestSuite ---
type SingleTargetTestSuite struct {
	suite.Suite
}

func (suite *SingleTargetTestSuite) TestEqual() {
	testCases := []struct {
		name           string
		functionTarget FunctionTarget
		expectedResult bool
	}{
		{
			name:           "Equal exact match",
			functionTarget: SingleTarget("myFunction"),
			expectedResult: true,
		}, {
			name:           "Equal no match",
			functionTarget: SingleTarget("otherFunction"),
			expectedResult: false,
		}, {
			name:           "Equal empty functions name no match",
			functionTarget: SingleTarget(""),
			expectedResult: false,
		}, {
			name:           "Equal canary no match",
			functionTarget: CanaryTarget{functionNames: [2]string{}},
			expectedResult: false,
		},
	}

	for _, testCase := range testCases {
		suite.Run(testCase.name, func() {
			testSingleFunctionName := SingleTarget("myFunction")
			result := testSingleFunctionName.Equal(testCase.functionTarget)
			suite.Equal(testCase.expectedResult, result)
		})
	}
}

func (suite *SingleTargetTestSuite) TestToSliceString() {
	testCases := []struct {
		name               string
		singleFunctionName string
		expectedResult     []string
	}{
		{
			name:               "ToSliceStringWithFunction",
			singleFunctionName: "toSliceStringFunction",
			expectedResult:     []string{"toSliceStringFunction"},
		}, {
			name:               "ToSliceStringWithSpecialChars",
			singleFunctionName: "my-function_123",
			expectedResult:     []string{"my-function_123"},
		},
	}

	for _, testCase := range testCases {
		suite.Run(testCase.name, func() {
			testSingleFunctionName := SingleTarget(testCase.singleFunctionName)
			result := testSingleFunctionName.ToSliceString()
			suite.Equal(testCase.expectedResult, result)
			suite.Len(result, 1)
		})
	}
}

// TestSingleFunctionNameTestSuite runs the test suite
func TestSingleFunctionNameTestSuite(t *testing.T) {
	suite.Run(t, new(SingleTargetTestSuite))
}

// --- CanaryTargetTestSuite ---
type CanaryTargetTestSuite struct {
	suite.Suite
}

func (suite *CanaryTargetTestSuite) TestEqual() {
	testCases := []struct {
		name           string
		functionTarget FunctionTarget
		expectedResult bool
	}{
		{
			name:           "Equal match",
			functionTarget: &CanaryTarget{[2]string{"test-function1", "test-function2"}},
			expectedResult: true,
		}, {
			name:           "Equal no match",
			functionTarget: &CanaryTarget{[2]string{"test-function1", "test-function3"}},
			expectedResult: false,
		}, {
			name:           "Equal empty functions name",
			functionTarget: &CanaryTarget{[2]string{}},
			expectedResult: false,
		}, {
			name:           "Equal case sensitive",
			functionTarget: &CanaryTarget{[2]string{"TEST-function1", "test-function2"}},
			expectedResult: false,
		}, {
			name:           "Equal with single functions",
			functionTarget: SingleTarget("test-function1"),
			expectedResult: false,
		},
	}

	for _, testCase := range testCases {
		suite.Run(testCase.name, func() {
			testCanaryFunctionNames := &CanaryTarget{[2]string{"test-function1", "test-function2"}}
			result := testCanaryFunctionNames.Equal(testCase.functionTarget)
			suite.Equal(testCase.expectedResult, result)
		})
	}
}

func (suite *CanaryTargetTestSuite) TestToSliceString() {
	testCases := []struct {
		name           string
		canaryTarget   [2]string
		expectedResult []string
	}{
		{
			name:           "ToSliceStringWithFunction",
			canaryTarget:   [2]string{"test-function1", "test-function2"},
			expectedResult: []string{"test-function1", "test-function2"},
		}, {
			name:           "ToSliceStringWithSpecialChars",
			canaryTarget:   [2]string{"my-function_123", "test-function2"},
			expectedResult: []string{"my-function_123", "test-function2"},
		},
	}

	for _, testCase := range testCases {
		suite.Run(testCase.name, func() {
			testCanaryFunctionNames := &CanaryTarget{testCase.canaryTarget}
			result := testCanaryFunctionNames.ToSliceString()
			suite.Equal(testCase.expectedResult, result)
		})
	}
}

// TestCanaryFunctionNamesTestSuite runs the test suite
func TestCanaryFunctionNamesTestSuite(t *testing.T) {
	suite.Run(t, new(CanaryTargetTestSuite))
}
