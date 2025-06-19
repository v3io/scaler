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
	path     string
	function string
}

func (suite *SafeTrieTestSuite) TestPathTreeSet() {
	suite.T().Parallel()
	for _, testCase := range []struct {
		name           string
		args           []safeTrieFunctionArgs
		expectedResult map[string][]string
		expectError    bool
		errorMessage   string
	}{
		{
			name: "simple set",
			args: []safeTrieFunctionArgs{
				{
					path:     "/path/to/function",
					function: "test-function",
				},
			},
			expectedResult: map[string][]string{"/path/to/function": {"test-function"}},
		}, {
			name: "idempotent test",
			args: []safeTrieFunctionArgs{
				{
					path:     "/path/to/function",
					function: "test-function",
				}, {
					path:     "/path/to/function",
					function: "test-function",
				},
			},
			expectedResult: map[string][]string{"/path/to/function": {"test-function"}},
		}, {
			name: "set twice the same path with a different function",
			args: []safeTrieFunctionArgs{
				{
					path:     "/path/to/function",
					function: "test-function",
				}, {
					path:     "/path/to/function",
					function: "test-function2",
				},
			},
			expectedResult: map[string][]string{"/path/to/function": {"test-function", "test-function2"}},
		}, {
			name: "set nested paths and different functions",
			args: []safeTrieFunctionArgs{
				{
					path:     "/path/to/function",
					function: "test-function",
				}, {
					path:     "/path/to/function/nested",
					function: "test-function2",
				},
			},
			expectedResult: map[string][]string{
				"/path/to/function":        {"test-function"},
				"/path/to/function/nested": {"test-function2"},
			},
		}, {
			name: "set different paths and different functions",
			args: []safeTrieFunctionArgs{
				{
					path:     "/path/to/function",
					function: "test-function",
				}, {
					path:     "/another/path/to/function/",
					function: "test-function2",
				},
			},
			expectedResult: map[string][]string{
				"/path/to/function":          {"test-function"},
				"/another/path/to/function/": {"test-function2"},
			},
		}, {
			name: "empty function name",
			args: []safeTrieFunctionArgs{
				{
					path:     "/path/to/function",
					function: "",
				},
			},
			expectedResult: map[string][]string{},
			expectError:    true,
			errorMessage:   "function is empty",
		}, {
			name: "empty path",
			args: []safeTrieFunctionArgs{
				{
					path:     "",
					function: "test-function",
				},
			},
			expectedResult: map[string][]string{},
			expectError:    true,
			errorMessage:   "path is empty",
		}, {
			name: "double slash in path",
			args: []safeTrieFunctionArgs{
				{
					path:     "///path/to/function",
					function: "test-function",
				},
			},
			expectedResult: map[string][]string{
				"///path/to/function": {"test-function"},
			},
		}, {
			name: "path starts without slash",
			args: []safeTrieFunctionArgs{
				{
					path:     "path/to/function",
					function: "test-function",
				},
			},
			expectedResult: map[string][]string{
				"path/to/function": {"test-function"},
			},
		}, {
			name:           "lots of paths and functions",
			args:           suite.generatePathsAndFunctions(200),
			expectedResult: suite.generateExpectedResultMap(200),
		}, {
			name:           "path ends with slash",
			args:           []safeTrieFunctionArgs{{path: "/path/to/function/", function: "test-function"}},
			expectedResult: map[string][]string{"/path/to/function/": {"test-function"}},
		}, {
			name:           "path with dots",
			args:           []safeTrieFunctionArgs{{path: "/path/./to/./function/", function: "test-function"}},
			expectedResult: map[string][]string{"/path/./to/./function/": {"test-function"}},
		}, {
			name:           "upper case path",
			args:           []safeTrieFunctionArgs{{path: "/PATH/TO/function", function: "test-function"}},
			expectedResult: map[string][]string{"/PATH/TO/function": {"test-function"}},
		}, {
			name: "upper case function name",
			args: []safeTrieFunctionArgs{
				{path: "/path/to/function", function: "test-function"},
				{path: "/path/to/function", function: "test-FUNCTION"},
			},
			expectedResult: map[string][]string{"/path/to/function": {"test-function", "test-FUNCTION"}},
		}, {
			name: "path with numbers and hyphens",
			args: []safeTrieFunctionArgs{
				{path: "/api/v1/user-data/123", function: "test-function"},
			},
			expectedResult: map[string][]string{"/api/v1/user-data/123": {"test-function"}},
		},
	} {
		suite.Run(testCase.name, func() {
			testSafeTrie := suite.generateSafeTrieForTest([]safeTrieFunctionArgs{})
			for _, setArgs := range testCase.args {
				err := testSafeTrie.SetFunctionName(setArgs.path, setArgs.function)
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
		{"/", "test-function"},
		{"/path/to/function1", "test-function1"},
		{"/path/to/function1/nested", "test-function2"},
		{"/path/./to/./function/", "test-function1"},
		{"path//to//function/", "test-function1"},
		{"/path/to/multiple/functions", "test-function1"},
		{"/path/to/multiple/functions", "test-function2"},
	}
	for _, testCase := range []struct {
		name           string
		arg            string
		expectedResult []string
		expectError    bool
		errorMessage   string
	}{
		{
			name:           "get root path",
			arg:            "/",
			expectedResult: []string{"test-function"},
		}, {
			name:           "get regular path",
			arg:            "/path/to/function1",
			expectedResult: []string{"test-function1"},
		}, {
			name:           "get nested path",
			arg:            "/path/to/function1/nested",
			expectedResult: []string{"test-function2"},
		}, {
			name:           "get closest match",
			arg:            "/path/to/function1/nested/extra",
			expectedResult: []string{"test-function2"},
		}, {
			name:         "get empty path",
			arg:          "",
			expectError:  true,
			errorMessage: "path is empty",
		}, {
			name:           "get closest match with different suffix",
			arg:            "/path/to/function1/something/else",
			expectedResult: []string{"test-function1"},
		}, {
			name:           "get path with dots",
			arg:            "/path/./to/./function/",
			expectedResult: []string{"test-function1"},
		}, {
			name:           "get path with slash",
			arg:            "path//to//function/",
			expectedResult: []string{"test-function1"},
		}, {
			name:           "get multiple functions for the same path",
			arg:            "/path/to/multiple/functions",
			expectedResult: []string{"test-function1", "test-function2"},
		},
	} {
		suite.Run(testCase.name, func() {
			testSafeTrie := suite.generateSafeTrieForTest(initialStateGetTest)
			result, err := testSafeTrie.GetFunctionNames(testCase.arg)
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
		expectedResult map[string][]string
		expectError    bool
		errorMessage   string
	}{
		{
			name: "delete a path and validate that nested path is still there",
			initialState: []safeTrieFunctionArgs{
				{"/path/to/function1", "test-function1"},
				{"/path/to/function1/nested", "test-function2"},
			},
			deleteArgs: safeTrieFunctionArgs{"/path/to/function1", "test-function1"},
			expectedResult: map[string][]string{
				"/path/to/function1/nested": {"test-function2"},
			},
		}, {
			name: "delete a function from multiple values and validate that the other function is still there",
			initialState: []safeTrieFunctionArgs{
				{"/path/to/multiple/functions", "test-function1"},
				{"/path/to/multiple/functions", "test-function2"},
			},
			deleteArgs: safeTrieFunctionArgs{"/path/to/multiple/functions", "test-function1"},
			expectedResult: map[string][]string{
				"/path/to/multiple/functions": {"test-function2"},
			},
		}, {
			name: "delete function that does not exist in the path",
			initialState: []safeTrieFunctionArgs{
				{"/path/to/function1", "test-function1"},
			},
			deleteArgs: safeTrieFunctionArgs{"/path/to/function1", "test-function2"},
			expectedResult: map[string][]string{
				"/path/to/function1": {"test-function1"},
			},
		}, {
			name: "delete function that does not exist in multiple value path",
			initialState: []safeTrieFunctionArgs{
				{"/path/to/functions", "test-function1"},
				{"/path/to/functions", "test-function2"},
			},
			deleteArgs: safeTrieFunctionArgs{"/path/to/functions", "test-not-existing-function"},
			expectedResult: map[string][]string{
				"/path/to/functions": {"test-function1", "test-function2"},
			},
		}, {
			name: "delete not exist path",
			initialState: []safeTrieFunctionArgs{
				{"/path/to/function1", "test-function1"},
			},
			deleteArgs: safeTrieFunctionArgs{"/path/to/function1/nested", "test-function2"},
			expectedResult: map[string][]string{
				"/path/to/function1": {"test-function1"},
			},
		}, {
			name: "delete path with suffix that does not exist",
			initialState: []safeTrieFunctionArgs{
				{"/path/to/function1", "test-function1"},
				{"/path/to/function1/nested", "test-function2"},
			},
			deleteArgs: safeTrieFunctionArgs{"/path/to/function1/path/suffix", "test-function1"},
			expectedResult: map[string][]string{
				"/path/to/function1":        {"test-function1"},
				"/path/to/function1/nested": {"test-function2"},
			},
		},
	} {
		suite.Run(testCase.name, func() {
			testSafeTrie := suite.generateSafeTrieForTest(testCase.initialState)

			err := testSafeTrie.DeleteFunctionName(testCase.deleteArgs.path, testCase.deleteArgs.function)
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
				{"/test/path/", "test-function1"},
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

// flattenSafeTrie converts a PathTrie into a map[string][]string
// This function is not part of the SafeTrieTestSuite because it is also in use in IngressCacheTestSuite
func flattenSafeTrie(st *SafeTrie) (map[string][]string, error) {
	resultMap := make(map[string][]string)
	err := st.pathTrie.Walk(func(key string, value interface{}) error {
		// The Walk function iterates over all nodes.
		// Only store key-value pairs where a non-nil value has been explicitly 'Put'.
		// If a node exists as an internal prefix (e.g., "/a" for "/a/b"), its 'value' will be nil.
		// We only care about the values that were actually stored.
		if value != nil {
			convertedValue, ok := value.([]string)
			if !ok {
				return fmt.Errorf("path value should be []string")
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
		path := fmt.Sprintf("/path/to/function/%d", i)
		function := fmt.Sprintf("function-%d", i)
		args[i] = safeTrieFunctionArgs{path: path, function: function}
	}
	return args
}

func (suite *SafeTrieTestSuite) generateExpectedResultMap(num int) map[string][]string {
	expectedResult := make(map[string][]string)
	args := suite.generatePathsAndFunctions(num)
	for i := 0; i < num; i++ {
		expectedResult[args[i].path] = []string{args[i].function}
	}
	return expectedResult
}

// generateSafeTrieForTest creates a SafeTrie instance and sets the provided initial state
func (suite *SafeTrieTestSuite) generateSafeTrieForTest(initialSafeTrieState []safeTrieFunctionArgs) *SafeTrie {
	var err error
	safeTrie := NewSafeTrie()

	// set path tree with the provided required state
	for _, args := range initialSafeTrieState {
		err = safeTrie.SetFunctionName(args.path, args.function)
		suite.Require().NoError(err)
	}

	return safeTrie
}

func TestSafeTrie(t *testing.T) {
	suite.Run(t, new(SafeTrieTestSuite))
}
