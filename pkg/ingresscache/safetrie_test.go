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
		expectedResult map[string]FunctionTarget
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
			expectedResult: map[string]FunctionTarget{"/path/to/function": &SingleTarget{"test-function"}},
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
			expectedResult: map[string]FunctionTarget{"/path/to/function": &SingleTarget{"test-function"}},
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
			expectedResult: map[string]FunctionTarget{"/path/to/function": &CanaryTarget{[2]string{"test-function", "test-function2"}}},
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
			expectedResult: map[string]FunctionTarget{
				"/path/to/function":        &SingleTarget{"test-function"},
				"/path/to/function/nested": &SingleTarget{"test-function2"},
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
			expectedResult: map[string]FunctionTarget{
				"/path/to/function":          &SingleTarget{"test-function"},
				"/another/path/to/function/": &SingleTarget{"test-function2"},
			},
		}, {
			name: "empty function name",
			args: []safeTrieFunctionArgs{
				{
					path:     "/path/to/function",
					function: "",
				},
			},
			expectedResult: map[string]FunctionTarget{},
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
			expectedResult: map[string]FunctionTarget{},
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
			expectedResult: map[string]FunctionTarget{
				"///path/to/function": &SingleTarget{"test-function"},
			},
		}, {
			name: "path starts without slash",
			args: []safeTrieFunctionArgs{
				{
					path:     "path/to/function",
					function: "test-function",
				},
			},
			expectedResult: map[string]FunctionTarget{
				"path/to/function": &SingleTarget{"test-function"},
			},
		}, {
			name:           "lots of paths and functions",
			args:           suite.generatePathsAndFunctions(200),
			expectedResult: suite.generateExpectedResultMap(200),
		}, {
			name:           "path ends with slash",
			args:           []safeTrieFunctionArgs{{path: "/path/to/function/", function: "test-function"}},
			expectedResult: map[string]FunctionTarget{"/path/to/function/": &SingleTarget{"test-function"}},
		}, {
			name:           "path with dots",
			args:           []safeTrieFunctionArgs{{path: "/path/./to/./function/", function: "test-function"}},
			expectedResult: map[string]FunctionTarget{"/path/./to/./function/": &SingleTarget{"test-function"}},
		}, {
			name:           "upper case path",
			args:           []safeTrieFunctionArgs{{path: "/PATH/TO/function", function: "test-function"}},
			expectedResult: map[string]FunctionTarget{"/PATH/TO/function": &SingleTarget{"test-function"}},
		}, {
			name: "upper case function name",
			args: []safeTrieFunctionArgs{
				{path: "/path/to/function", function: "test-function"},
				{path: "/path/to/function", function: "test-FUNCTION"},
			},
			expectedResult: map[string]FunctionTarget{"/path/to/function": &CanaryTarget{[2]string{"test-function", "test-FUNCTION"}}},
		}, {
			name: "path with numbers and hyphens",
			args: []safeTrieFunctionArgs{
				{path: "/api/v1/user-data/123", function: "test-function"},
			},
			expectedResult: map[string]FunctionTarget{"/api/v1/user-data/123": &SingleTarget{"test-function"}},
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
		expectedResult map[string]FunctionTarget
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
			expectedResult: map[string]FunctionTarget{
				"/path/to/function1/nested": &SingleTarget{"test-function2"},
			},
		}, {
			name: "delete a function from multiple values and validate that the other function is still there",
			initialState: []safeTrieFunctionArgs{
				{"/path/to/multiple/functions", "test-function1"},
				{"/path/to/multiple/functions", "test-function2"},
			},
			deleteArgs: safeTrieFunctionArgs{"/path/to/multiple/functions", "test-function1"},
			expectedResult: map[string]FunctionTarget{
				"/path/to/multiple/functions": &SingleTarget{"test-function2"},
			},
		}, {
			name: "delete function that does not exist in the path",
			initialState: []safeTrieFunctionArgs{
				{"/path/to/function1", "test-function1"},
			},
			deleteArgs: safeTrieFunctionArgs{"/path/to/function1", "test-function2"},
			expectedResult: map[string]FunctionTarget{
				"/path/to/function1": &SingleTarget{"test-function1"},
			},
		}, {
			name: "delete function that does not exist in multiple value path",
			initialState: []safeTrieFunctionArgs{
				{"/path/to/functions", "test-function1"},
				{"/path/to/functions", "test-function2"},
			},
			deleteArgs: safeTrieFunctionArgs{"/path/to/functions", "test-not-existing-function"},
			expectedResult: map[string]FunctionTarget{
				"/path/to/functions": &CanaryTarget{[2]string{"test-function1", "test-function2"}},
			},
		}, {
			name: "delete not exist path",
			initialState: []safeTrieFunctionArgs{
				{"/path/to/function1", "test-function1"},
			},
			deleteArgs: safeTrieFunctionArgs{"/path/to/function1/nested", "test-function2"},
			expectedResult: map[string]FunctionTarget{
				"/path/to/function1": &SingleTarget{"test-function1"},
			},
		}, {
			name: "delete path with suffix that does not exist",
			initialState: []safeTrieFunctionArgs{
				{"/path/to/function1", "test-function1"},
				{"/path/to/function1/nested", "test-function2"},
			},
			deleteArgs: safeTrieFunctionArgs{"/path/to/function1/path/suffix", "test-function1"},
			expectedResult: map[string]FunctionTarget{
				"/path/to/function1":        &SingleTarget{"test-function1"},
				"/path/to/function1/nested": &SingleTarget{"test-function2"},
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

// flattenSafeTrie converts a PathTrie into a map[string]FunctionTarget
// This function is not part of the SafeTrieTestSuite because it is also in use in IngressCacheTestSuite
func flattenSafeTrie(st *SafeTrie) (map[string]FunctionTarget, error) {
	resultMap := make(map[string]FunctionTarget)
	err := st.pathTrie.Walk(func(key string, value interface{}) error {
		// The Walk function iterates over all nodes.
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
		path := fmt.Sprintf("/path/to/function/%d", i)
		function := fmt.Sprintf("function-%d", i)
		args[i] = safeTrieFunctionArgs{path: path, function: function}
	}
	return args
}

func (suite *SafeTrieTestSuite) generateExpectedResultMap(num int) map[string]FunctionTarget {
	expectedResult := make(map[string]FunctionTarget)
	args := suite.generatePathsAndFunctions(num)
	for i := 0; i < num; i++ {
		expectedResult[args[i].path] = &SingleTarget{args[i].function}
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

// --- SingleTargetTestSuite ---
type SingleTargetTestSuite struct {
	suite.Suite
}

func (suite *SingleTargetTestSuite) TestContains() {
	testCases := []struct {
		name           string
		functionName   string
		expectedResult bool
	}{
		{
			name:           "Contains exact match",
			functionName:   "myFunction",
			expectedResult: true,
		}, {
			name:           "Contains no match",
			functionName:   "otherFunction",
			expectedResult: false,
		}, {
			name:           "Contains empty function name no match",
			functionName:   "",
			expectedResult: false,
		}, {
			name:           "Contains case sensitive",
			functionName:   "MYFUNCTION",
			expectedResult: false,
		},
	}

	for _, testCase := range testCases {
		suite.Run(testCase.name, func() {
			testSingleFunctionName := &SingleTarget{functionName: "myFunction"}
			result := testSingleFunctionName.Contains(testCase.functionName)
			suite.Equal(testCase.expectedResult, result)
		})
	}
}

func (suite *SingleTargetTestSuite) TestRemoveFunctionName() {
	testCases := []struct {
		name           string
		functionName   string
		expectedResult FunctionTarget
		expectError    bool
		errorMessage   string
	}{
		{
			name:           "RemoveExistingFunction",
			functionName:   "test-function1",
			expectedResult: nil,
			expectError:    true,
			errorMessage:   "cannot remove function name from SingleTarget, it only contains one function name",
		},
		{
			name:           "RemoveNonExistingFunction",
			functionName:   "otherFunction",
			expectedResult: &SingleTarget{functionName: "test-function1"},
		},
	}

	for _, testCase := range testCases {
		suite.Run(testCase.name, func() {
			testSingleFunctionName := &SingleTarget{functionName: "test-function1"}
			result, err := testSingleFunctionName.Delete(testCase.functionName)
			if testCase.expectError {
				suite.Require().Error(err)
				suite.Require().ErrorContains(err, testCase.errorMessage)
				suite.Nil(result)
			} else {
				suite.Equal(testCase.expectedResult, result)
			}
		})
	}
}

func (suite *SingleTargetTestSuite) TestAddFunctionName() {
	testCases := []struct {
		name           string
		functionName   string
		expectedResult FunctionTarget
	}{
		{
			name:           "Add same function name",
			functionName:   "test-function1",
			expectedResult: &SingleTarget{functionName: "test-function1"},
		}, {
			name:           "Add function name",
			functionName:   "test-function2",
			expectedResult: &CanaryTarget{[2]string{"test-function1", "test-function2"}},
		},
	}

	for _, testCase := range testCases {
		suite.Run(testCase.name, func() {
			testSingleFunctionName := &SingleTarget{functionName: "test-function1"}
			result, err := testSingleFunctionName.Add(testCase.functionName)
			suite.Require().NoError(err)
			suite.Require().NotNil(result)
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
			testSingleFunctionName := &SingleTarget{functionName: testCase.singleFunctionName}
			result := testSingleFunctionName.ToSliceString()
			suite.Equal(testCase.expectedResult, result)
			suite.Len(result, 1)
		})
	}
}

func (suite *SingleTargetTestSuite) TestIsSingleFunctionName() {
	testCases := []struct {
		name               string
		singleFunctionName *SingleTarget
	}{
		{
			name:               "IsSingleFunctionNameTrue",
			singleFunctionName: &SingleTarget{functionName: "isSingleFunctionNameFunction"},
		},
	}

	for _, testCase := range testCases {
		suite.Run(testCase.name, func() {
			result := testCase.singleFunctionName.IsSingle()
			suite.Require().True(result)
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

func (suite *CanaryTargetTestSuite) TestContains() {
	testCases := []struct {
		name           string
		functionName   string
		expectedResult bool
	}{
		{
			name:           "Contains match",
			functionName:   "test-function1",
			expectedResult: true,
		}, {
			name:           "Contains no match",
			functionName:   "test-function3",
			expectedResult: false,
		}, {
			name:           "Contains empty function name",
			functionName:   "",
			expectedResult: false,
		}, {
			name:           "Contains case sensitive",
			functionName:   "TEST-function1",
			expectedResult: false,
		},
	}

	for _, testCase := range testCases {
		suite.Run(testCase.name, func() {
			testCanaryFunctionNames := &CanaryTarget{[2]string{"test-function1", "test-function2"}}
			result := testCanaryFunctionNames.Contains(testCase.functionName)
			suite.Equal(testCase.expectedResult, result)
		})
	}
}

func (suite *CanaryTargetTestSuite) TestRemoveFunctionName() {
	testCases := []struct {
		name           string
		functionName   string
		expectedResult FunctionTarget
	}{
		{
			name:           "RemoveExistingFunction",
			functionName:   "test-function1",
			expectedResult: &SingleTarget{functionName: "test-function2"},
		}, {
			name:           "RemoveNotExistingFunction",
			functionName:   "test-function3",
			expectedResult: &CanaryTarget{[2]string{"test-function1", "test-function2"}},
		},
	}

	for _, testCase := range testCases {
		suite.Run(testCase.name, func() {
			testCanaryFunctionNames := &CanaryTarget{[2]string{"test-function1", "test-function2"}}
			result, err := testCanaryFunctionNames.Delete(testCase.functionName)
			suite.Require().NoError(err)
			suite.Require().Equal(testCase.expectedResult, result)
		})
	}
}

func (suite *CanaryTargetTestSuite) TestAddFunctionName() {
	testCases := []struct {
		name           string
		functionName   string
		expectedResult FunctionTarget
		expectError    bool
		errorMessage   string
	}{
		{
			name:           "Add same function name",
			functionName:   "test-function1",
			expectedResult: &CanaryTarget{[2]string{"test-function1", "test-function2"}},
		}, {
			name:           "Add distinct function name to a CanaryTarget",
			functionName:   "test-function3",
			expectedResult: &CanaryTarget{[2]string{"test-function1", "test-function2"}},
			expectError:    true,
			errorMessage:   "cannot add function name to CanaryTarget, it already contains two function names",
		},
	}

	for _, testCase := range testCases {
		suite.Run(testCase.name, func() {
			testCanaryFunctionNames := &CanaryTarget{[2]string{"test-function1", "test-function2"}}
			result, err := testCanaryFunctionNames.Add(testCase.functionName)
			if testCase.expectError {
				suite.Require().Error(err)
				suite.Require().ErrorContains(err, testCase.errorMessage)
				suite.Equal(testCase.expectedResult, result)
			} else {
				suite.Require().NoError(err)
				suite.Equal(testCase.expectedResult, result)
			}
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

func (suite *CanaryTargetTestSuite) TestIsSingleFunctionName() {
	testCases := []struct {
		name string
	}{
		{
			name: "IsSingleFunctionNameTrue",
		},
	}

	for _, testCase := range testCases {
		suite.Run(testCase.name, func() {
			testCanaryFunctionNames := &CanaryTarget{[2]string{"test-function1", "test-function2"}}
			result := testCanaryFunctionNames.IsSingle()
			suite.Require().False(result)
		})
	}
}

// TestCanaryFunctionNamesTestSuite runs the test suite
func TestCanaryFunctionNamesTestSuite(t *testing.T) {
	suite.Run(t, new(CanaryTargetTestSuite))
}
