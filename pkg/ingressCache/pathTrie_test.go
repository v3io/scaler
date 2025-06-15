/*
Copyright 2019 Iguazio Systems Ltd.

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
	"github.com/stretchr/testify/suite"
	"testing"
)

type PathTreeTest struct {
	suite.Suite
	pathTree *PathTree
}

type pathTreeFunctionArgs struct {
	path     string
	function string
}

func (suite *PathTreeTest) SetupTest() {
	suite.pathTree = NewPathTree()
}

func (suite *PathTreeTest) SetupSubTest(pathTreeState []pathTreeFunctionArgs) {
	suite.pathTree = NewPathTree()

	// set path tree with the provided required state
	for _, args := range pathTreeState {
		err := suite.pathTree.SetFunctionName(args.path, args.function)
		suite.Require().NoError(err)
	}
}

func (suite *PathTreeTest) TestPathTreeSet() {
	testFunctionName := "test-function"
	testFunctionName2 := "test-function-2"
	testFunctionPath := "/path/to/function"
	testFunctionPathNested := "/path/to/function/nested"
	testFunctionPathEndsWithSlash := "/path/to/function/"
	testFunctionPathWithDots := "/path/./to/./function/"
	testFunctionPathUpperCase := "/PATH/TO/function"
	testFunctionNameUpperCase := "test-FUNCTION"
	testApiPath := "/api/v1/user-data/123"
	for _, testCase := range []struct {
		name           string
		args           []pathTreeFunctionArgs
		expectedResult map[string][]string
		shouldFail     bool
		errorMessage   string
	}{
		{
			name: "simple set",
			args: []pathTreeFunctionArgs{
				{
					path:     testFunctionPath,
					function: testFunctionName,
				},
			},
			expectedResult: map[string][]string{testFunctionPath: {testFunctionName}},
		}, {
			name: "idempotent test",
			args: []pathTreeFunctionArgs{
				{
					path:     testFunctionPath,
					function: testFunctionName,
				}, {
					path:     testFunctionPath,
					function: testFunctionName,
				},
			},
			expectedResult: map[string][]string{testFunctionPath: {testFunctionName}},
		}, {
			name: "set twice the same path with a different function",
			args: []pathTreeFunctionArgs{
				{
					path:     testFunctionPath,
					function: testFunctionName,
				}, {
					path:     testFunctionPath,
					function: testFunctionName2,
				},
			},
			expectedResult: map[string][]string{testFunctionPath: {testFunctionName, testFunctionName2}},
		}, {
			name: "set different paths and functions",
			args: []pathTreeFunctionArgs{
				{
					path:     testFunctionPath,
					function: testFunctionName,
				}, {
					path:     testFunctionPathNested,
					function: testFunctionName2,
				},
			},
			expectedResult: map[string][]string{
				testFunctionPath:       {testFunctionName},
				testFunctionPathNested: {testFunctionName2},
			},
		}, {
			name: "empty function name",
			args: []pathTreeFunctionArgs{
				{
					path:     testFunctionPath,
					function: "",
				},
			},
			expectedResult: map[string][]string{},
			shouldFail:     true,
			errorMessage:   "function is empty",
		}, {
			name: "empty path",
			args: []pathTreeFunctionArgs{
				{
					path:     "",
					function: testFunctionName,
				},
			},
			expectedResult: map[string][]string{},
			shouldFail:     true,
			errorMessage:   "path is empty",
		}, {
			name: "double slash in path",
			args: []pathTreeFunctionArgs{
				{
					path:     "//" + testFunctionPath,
					function: testFunctionName,
				},
			},
			expectedResult: map[string][]string{
				"//" + testFunctionPath: {testFunctionName},
			},
		}, {
			name: "path starts without slash",
			args: []pathTreeFunctionArgs{
				{
					path:     "path/to/function",
					function: testFunctionName,
				},
			},
			expectedResult: map[string][]string{
				"path/to/function": {testFunctionName},
			},
		}, {
			name:           "lots of paths and functions",
			args:           suite.generateLotsOfPathsAndFunctions(200),
			expectedResult: suite.generateExpectedResultMap(200),
		}, {
			name:           "path ends with slash",
			args:           []pathTreeFunctionArgs{{path: testFunctionPathEndsWithSlash, function: testFunctionName}},
			expectedResult: map[string][]string{testFunctionPathEndsWithSlash: {testFunctionName}},
		}, {
			name:           "path with dots",
			args:           []pathTreeFunctionArgs{{path: testFunctionPathWithDots, function: testFunctionName}},
			expectedResult: map[string][]string{testFunctionPathWithDots: {testFunctionName}},
		}, {
			name:           "upper case path",
			args:           []pathTreeFunctionArgs{{path: testFunctionPathUpperCase, function: testFunctionName}},
			expectedResult: map[string][]string{testFunctionPathUpperCase: {testFunctionName}},
		}, {
			name: "upper case function name",
			args: []pathTreeFunctionArgs{
				{path: testFunctionPath, function: testFunctionName},
				{path: testFunctionPath, function: testFunctionNameUpperCase},
			},
			expectedResult: map[string][]string{testFunctionPath: {testFunctionName, testFunctionNameUpperCase}},
		}, {
			name: "path with numbers and hyphens",
			args: []pathTreeFunctionArgs{
				{path: testApiPath, function: testFunctionName},
			},
			expectedResult: map[string][]string{testApiPath: {testFunctionName}},
		},
	} {
		suite.Run(testCase.name, func() {
			suite.SetupSubTest(nil)
			for _, setArgs := range testCase.args {
				err := suite.pathTree.SetFunctionName(setArgs.path, setArgs.function)
				if testCase.shouldFail {
					suite.Require().Error(err)
					suite.Require().Equal(err.Error(), testCase.errorMessage)
				} else {
					suite.Require().NoError(err)
				}
			}
			suitePathTree, err := suite.pathTreeToMap(suite.pathTree)
			suite.Require().NoError(err)
			suite.Require().Equal(testCase.expectedResult, suitePathTree)
		})
	}
}
func (suite *PathTreeTest) TestPathTreeGet() {
	testPathRoot := "/"
	testPath1 := "/path/to/function1"
	testPath2 := testPath1 + "/nested"
	testFunctionName := "test-function"
	testFunctionName1 := "test-function1"
	testFunctionName2 := "test-function2"
	testFunctionPathWithDots := "/path/./to/./function/"
	testFunctionPathWithDoubleSlash := "path//to//function/"
	testPathWithMultipleFunctions := "/path/to/multiple/functions"

	for _, testCase := range []struct {
		name           string
		arg            string
		expectedResult []string
		shouldFail     bool
		errorMessage   string
	}{
		{
			name:           "get root path",
			arg:            testPathRoot,
			expectedResult: []string{testFunctionName},
		}, {
			name:           "get regular path",
			arg:            testPath1,
			expectedResult: []string{testFunctionName1},
		}, {
			name:           "get nested path",
			arg:            testPath2,
			expectedResult: []string{testFunctionName2},
		}, {
			name:           "get closest match",
			arg:            "/path/to/function1/nested/extra",
			expectedResult: []string{testFunctionName2},
		}, {
			name:         "get empty path",
			arg:          "",
			shouldFail:   true,
			errorMessage: "path is empty",
		}, {
			name:           "get closest match with different sufix",
			arg:            "/path/to/function1/something/else",
			expectedResult: []string{testFunctionName1},
		}, {
			name:           "get path with dots",
			arg:            testFunctionPathWithDots,
			expectedResult: []string{testFunctionName1},
		}, {
			name:           "get path with slash",
			arg:            testFunctionPathWithDoubleSlash,
			expectedResult: []string{testFunctionName1},
		}, {
			name:           "get multiple functions for the same path",
			arg:            testPathWithMultipleFunctions,
			expectedResult: []string{testFunctionName1, testFunctionName2},
		},
	} {
		suite.SetupSubTest([]pathTreeFunctionArgs{
			{testPathRoot, testFunctionName},
			{testPath1, testFunctionName1},
			{testPath2, testFunctionName2},
			{testFunctionPathWithDots, testFunctionName1},
			{testFunctionPathWithDoubleSlash, testFunctionName1},
			{testPathWithMultipleFunctions, testFunctionName1},
			{testPathWithMultipleFunctions, testFunctionName2},
		})
		suite.Run(testCase.name, func() {
			result, err := suite.pathTree.GetFunctionName(testCase.arg)
			if testCase.shouldFail {
				suite.Require().Error(err)
				suite.Require().Contains(err.Error(), testCase.errorMessage)
			} else {
				suite.Require().NoError(err)
				suite.Require().Equal(testCase.expectedResult, result)
			}
		})
	}
}
func (suite *PathTreeTest) TestPathTreeDelete() {
	testPath1 := "/path/to/function1"
	testPath2 := testPath1 + "/nested"
	testFunctionName1 := "test-function1"
	testFunctionName2 := "test-function2"
	testPathWithMultipleFunctions := "/path/to/multiple/functions"

	type getFunctionAfterDeleteArgs struct { //this struct enables multiple get tests after delete
		path           string
		expectedResult []string
		shouldFail     bool
		errorMessage   string
	}

	for _, testCase := range []struct {
		initialState               []pathTreeFunctionArgs // initial state of the path tree before delete
		name                       string
		deleteArgs                 pathTreeFunctionArgs
		getFunctionAfterDeleteArgs []getFunctionAfterDeleteArgs
		shouldFail                 bool
		errorMessage               string
	}{
		{
			name: "delete a path and validate that nested path is still there",
			initialState: []pathTreeFunctionArgs{
				{testPath1, testFunctionName1},
				{testPath2, testFunctionName2},
			},
			deleteArgs: pathTreeFunctionArgs{testPath1, testFunctionName1},
			getFunctionAfterDeleteArgs: []getFunctionAfterDeleteArgs{
				{
					path:           testPath2,
					expectedResult: []string{testFunctionName2},
				}, {
					path:         testPath1,
					shouldFail:   true,
					errorMessage: "",
				},
			},
		}, {
			name: "delete a function from multiple values and validate that the other function is still there",
			initialState: []pathTreeFunctionArgs{
				{testPathWithMultipleFunctions, testFunctionName1},
				{testPathWithMultipleFunctions, testFunctionName2},
			},
			deleteArgs: pathTreeFunctionArgs{testPathWithMultipleFunctions, testFunctionName1},
			getFunctionAfterDeleteArgs: []getFunctionAfterDeleteArgs{
				{
					path:           testPathWithMultipleFunctions,
					expectedResult: []string{testFunctionName2},
				},
			},
		}, {
			name: "delete function that does not exist in the path",
			initialState: []pathTreeFunctionArgs{
				{testPath1, testFunctionName1},
			},
			deleteArgs: pathTreeFunctionArgs{testPath1, testFunctionName2},
			getFunctionAfterDeleteArgs: []getFunctionAfterDeleteArgs{
				{
					path:           testPath1,
					expectedResult: []string{testFunctionName1},
				},
			},
			shouldFail:   true,
			errorMessage: "",
		}, {
			name: "delete not exist path",
			initialState: []pathTreeFunctionArgs{
				{testPath1, testFunctionName1},
			},
			deleteArgs: pathTreeFunctionArgs{testPath2, testFunctionName2},
			getFunctionAfterDeleteArgs: []getFunctionAfterDeleteArgs{
				{
					path:           testPath1,
					expectedResult: []string{testFunctionName1},
				},
			},
		}, {
			name: "delete path with suffix that does not exist",
			initialState: []pathTreeFunctionArgs{
				{testPath1, testFunctionName1},
				{testPath2, testFunctionName2},
			},
			deleteArgs: pathTreeFunctionArgs{testPath1 + "/path/suffix", testFunctionName1},
			getFunctionAfterDeleteArgs: []getFunctionAfterDeleteArgs{
				{
					path:           testPath1,
					expectedResult: []string{testFunctionName1},
				}, {
					path:           testPath2,
					expectedResult: []string{testFunctionName2},
				},
			},
		},
	} {
		suite.Run(testCase.name, func() {
			suite.SetupSubTest(testCase.initialState)

			err := suite.pathTree.DeleteFunctionName(testCase.deleteArgs.path, testCase.deleteArgs.function)
			if testCase.shouldFail {
				suite.Require().Error(err)
				suite.Require().Contains(err.Error(), testCase.errorMessage)
			} else {
				suite.Require().NoError(err)
			}

			// After delete, check that the expected paths and functions are still there
			for _, getAfterDeleteArgs := range testCase.getFunctionAfterDeleteArgs {
				result, err := suite.pathTree.GetFunctionName(getAfterDeleteArgs.path)
				if getAfterDeleteArgs.shouldFail {
					suite.Require().Error(err)
					suite.Require().Contains(err.Error(), getAfterDeleteArgs.errorMessage)
				} else {
					suite.Require().NoError(err)
					suite.Require().Equal(getAfterDeleteArgs.expectedResult, result)
				}
			}
		})
	}
}

// --- PathTreeTest suite methods ---

// pathTreeToMap converts a PathTrie into a map[string][]string
func (suite *PathTreeTest) pathTreeToMap(pt *PathTree) (map[string][]string, error) {
	resultMap := make(map[string][]string)
	err := pt.t.Walk(func(key string, value interface{}) error {
		// The Walk function iterates over all nodes.
		// Only store key-value pairs where a non-nil value has been explicitly 'Put'.
		// If a node exists as an internal prefix (e.g., "/a" for "/a/b"), its 'value' will be nil.
		// We only care about the values that were actually stored.
		if value != nil {
			convertedValue, ok := value.([]string)
			if !ok {
				return fmt.Errorf("value is not a []string")
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

func (suite *PathTreeTest) generateLotsOfPathsAndFunctions(num int) []pathTreeFunctionArgs {
	args := make([]pathTreeFunctionArgs, num)
	for i := 0; i < num; i++ {
		path := fmt.Sprintf("/path/to/function/%d", i)
		function := fmt.Sprintf("function-%d", i)
		args[i] = pathTreeFunctionArgs{path: path, function: function}
	}
	return args
}

func (suite *PathTreeTest) generateExpectedResultMap(num int) map[string][]string {
	expectedResult := make(map[string][]string)
	args := suite.generateLotsOfPathsAndFunctions(num)
	for i := 0; i < num; i++ {
		expectedResult[args[i].path] = []string{args[i].function}
	}
	return expectedResult
}

func TestPathTree(t *testing.T) {
	suite.Run(t, new(PathTreeTest))
}
