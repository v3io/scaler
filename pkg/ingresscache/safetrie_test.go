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

type safeTrieTestArgs struct {
	path    string
	targets []string
}

func (suite *SafeTrieTestSuite) TestPathTreeSet() {
	suite.T().Parallel()
	for _, testCase := range []struct {
		name           string
		args           []safeTrieTestArgs
		expectedResult map[string]Target
		expectError    bool
		errorMessage   string
	}{
		{
			name: "simple set",
			args: []safeTrieTestArgs{
				{
					path:    "/path/to/targets",
					targets: []string{"test-target"},
				},
			},
			expectedResult: map[string]Target{"/path/to/targets": SingleTarget("test-target")},
		}, {
			name: "idempotent test",
			args: []safeTrieTestArgs{
				{
					path:    "/path/to/targets",
					targets: []string{"test-target"},
				}, {
					path:    "/path/to/targets",
					targets: []string{"test-target"},
				},
			},
			expectedResult: map[string]Target{"/path/to/targets": SingleTarget("test-target")},
		}, {
			name: "set twice the same path with a different targets",
			args: []safeTrieTestArgs{
				{
					path:    "/path/to/targets",
					targets: []string{"test-target"},
				}, {
					path:    "/path/to/targets",
					targets: []string{"test-target2"},
				},
			},
			expectedResult: map[string]Target{"/path/to/targets": SingleTarget("test-target2")},
		}, {
			name: "set nested paths and different targets",
			args: []safeTrieTestArgs{
				{
					path:    "/path/to/targets",
					targets: []string{"test-target"},
				}, {
					path:    "/path/to/targets/nested",
					targets: []string{"test-target2"},
				},
			},
			expectedResult: map[string]Target{
				"/path/to/targets":        SingleTarget("test-target"),
				"/path/to/targets/nested": SingleTarget("test-target2"),
			},
		}, {
			name: "set different paths and different targets",
			args: []safeTrieTestArgs{
				{
					path:    "/path/to/targets",
					targets: []string{"test-target"},
				}, {
					path:    "/another/path/to/targets/",
					targets: []string{"test-target2"},
				},
			},
			expectedResult: map[string]Target{
				"/path/to/targets":          SingleTarget("test-target"),
				"/another/path/to/targets/": SingleTarget("test-target2"),
			},
		}, {
			name: "empty targets name",
			args: []safeTrieTestArgs{
				{
					path:    "/path/to/targets",
					targets: []string{},
				},
			},
			expectedResult: map[string]Target{},
			expectError:    true,
			errorMessage:   "failed to create Target",
		}, {
			name: "empty path",
			args: []safeTrieTestArgs{
				{
					path:    "",
					targets: []string{"test-target"},
				},
			},
			expectedResult: map[string]Target{},
			expectError:    true,
			errorMessage:   "path is empty",
		}, {
			name: "double slash in path",
			args: []safeTrieTestArgs{
				{
					path:    "///path/to/targets",
					targets: []string{"test-target"},
				},
			},
			expectedResult: map[string]Target{
				"///path/to/targets": SingleTarget("test-target"),
			},
		}, {
			name: "path starts without slash",
			args: []safeTrieTestArgs{
				{
					path:    "path/to/targets",
					targets: []string{"test-target"},
				},
			},
			expectedResult: map[string]Target{
				"path/to/targets": SingleTarget("test-target"),
			},
		}, {
			name:           "lots of paths and targets",
			args:           suite.generatePathsAndTargets(200),
			expectedResult: suite.generateExpectedResultMap(200),
		}, {
			name:           "path ends with slash",
			args:           []safeTrieTestArgs{{path: "/path/to/targets/", targets: []string{"test-target"}}},
			expectedResult: map[string]Target{"/path/to/targets/": SingleTarget("test-target")},
		}, {
			name:           "path with dots",
			args:           []safeTrieTestArgs{{path: "/path/./to/./targets/", targets: []string{"test-target"}}},
			expectedResult: map[string]Target{"/path/./to/./targets/": SingleTarget("test-target")},
		}, {
			name:           "upper case path",
			args:           []safeTrieTestArgs{{path: "/PATH/TO/targets", targets: []string{"test-target"}}},
			expectedResult: map[string]Target{"/PATH/TO/targets": SingleTarget("test-target")},
		}, {
			name: "upper case targets name",
			args: []safeTrieTestArgs{
				{path: "/path/to/targets", targets: []string{"test-target"}},
				{path: "/path/to/targets", targets: []string{"test-target", "test-TARGET"}},
			},
			expectedResult: map[string]Target{"/path/to/targets": PairTarget{"test-target", "test-TARGET"}},
		}, {
			name: "path with numbers and hyphens",
			args: []safeTrieTestArgs{
				{path: "/api/v1/user-data/123", targets: []string{"test-target"}},
			},
			expectedResult: map[string]Target{"/api/v1/user-data/123": SingleTarget("test-target")},
		},
	} {
		suite.Run(testCase.name, func() {
			testSafeTrie := suite.generateSafeTrieForTest([]safeTrieTestArgs{})
			for _, setArgs := range testCase.args {
				err := testSafeTrie.Set(setArgs.path, setArgs.targets)
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
	initialStateGetTest := []safeTrieTestArgs{
		{"/", []string{"test-target"}},
		{"/path/to/target1", []string{"test-target1"}},
		{"/path/to/target1/nested", []string{"test-target2"}},
		{"/path/./to/./targets/", []string{"test-target1"}},
		{"path//to//targets/", []string{"test-target1"}},
		{"/path/to/multiple/targets", []string{"test-target1", "test-target2"}},
	}
	for _, testCase := range []struct {
		name           string
		path           string
		expectedResult Target
		expectError    bool
		errorMessage   string
	}{
		{
			name:           "get root path",
			path:           "/",
			expectedResult: SingleTarget("test-target"),
		}, {
			name:           "get regular path",
			path:           "/path/to/target1",
			expectedResult: SingleTarget("test-target1"),
		}, {
			name:           "get nested path",
			path:           "/path/to/target1/nested",
			expectedResult: SingleTarget("test-target2"),
		}, {
			name:           "get closest match",
			path:           "/path/to/target1/nested/extra",
			expectedResult: SingleTarget("test-target2"),
		}, {
			name:         "get empty path",
			path:         "",
			expectError:  true,
			errorMessage: "path is empty",
		}, {
			name:           "get closest match with different suffix",
			path:           "/path/to/target1/something/else",
			expectedResult: SingleTarget("test-target1"),
		}, {
			name:           "get path with dots",
			path:           "/path/./to/./targets/",
			expectedResult: SingleTarget("test-target1"),
		}, {
			name:           "get path with slash",
			path:           "path//to//targets/",
			expectedResult: SingleTarget("test-target1"),
		}, {
			name:           "get multiple targets for the same path",
			path:           "/path/to/multiple/targets",
			expectedResult: PairTarget{"test-target1", "test-target2"},
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
		initialState   []safeTrieTestArgs // initial state of the path tree before delete
		name           string
		deleteArgs     safeTrieTestArgs
		expectedResult map[string]Target
		expectError    bool
		errorMessage   string
	}{
		{
			name: "delete a path and validate that nested path is still there",
			initialState: []safeTrieTestArgs{
				{"/path/to/target1", []string{"test-target1"}},
				{"/path/to/target1/nested", []string{"test-target2"}},
			},
			deleteArgs: safeTrieTestArgs{"/path/to/target1", []string{"test-target1"}},
			expectedResult: map[string]Target{
				"/path/to/target1/nested": SingleTarget("test-target2"),
			},
		}, {
			name: "delete a targets from multiple values shouldn't do anything, validate that the targets is still there",
			initialState: []safeTrieTestArgs{
				{"/path/to/multiple/targets", []string{"test-target1", "test-target2"}},
			},
			deleteArgs: safeTrieTestArgs{"/path/to/multiple/targets", []string{"test-target1"}},
			expectedResult: map[string]Target{
				"/path/to/multiple/targets": PairTarget{"test-target1", "test-target2"},
			},
		}, {
			name: "delete targets that does not exist in the path",
			initialState: []safeTrieTestArgs{
				{"/path/to/target1", []string{"test-target1"}},
			},
			deleteArgs: safeTrieTestArgs{"/path/to/target1", []string{"test-target2"}},
			expectedResult: map[string]Target{
				"/path/to/target1": SingleTarget("test-target1"),
			},
		}, {
			name: "delete targets that does not exist in multiple value path",
			initialState: []safeTrieTestArgs{
				{"/path/to/targets", []string{"test-target1", "test-target2"}},
			},
			deleteArgs: safeTrieTestArgs{"/path/to/targets", []string{"test-not-existing-targets"}},
			expectedResult: map[string]Target{
				"/path/to/targets": PairTarget{"test-target1", "test-target2"},
			},
		}, {
			name: "delete not exist path",
			initialState: []safeTrieTestArgs{
				{"/path/to/target1", []string{"test-target1"}},
			},
			deleteArgs: safeTrieTestArgs{"/path/to/target1/nested", []string{"test-target2"}},
			expectedResult: map[string]Target{
				"/path/to/target1": SingleTarget("test-target1"),
			},
		}, {
			name: "delete path with suffix that does not exist",
			initialState: []safeTrieTestArgs{
				{"/path/to/target1", []string{"test-target1"}},
				{"/path/to/target1/nested", []string{"test-target2"}},
			},
			deleteArgs: safeTrieTestArgs{"/path/to/target1/path/suffix", []string{"test-target1"}},
			expectedResult: map[string]Target{
				"/path/to/target1":        SingleTarget("test-target1"),
				"/path/to/target1/nested": SingleTarget("test-target2"),
			},
		},
	} {
		suite.Run(testCase.name, func() {
			testSafeTrie := suite.generateSafeTrieForTest(testCase.initialState)

			err := testSafeTrie.Delete(testCase.deleteArgs.path, testCase.deleteArgs.targets)
			if testCase.expectError {
				suite.Require().Error(err)
				suite.Require().ErrorContains(err, testCase.errorMessage)
			} else {
				suite.Require().NoError(err)
			}

			// After delete, check that the expected paths and targets are as expected
			flattenSafeTrie, err := flattenSafeTrie(testSafeTrie)
			suite.Require().NoError(err)
			suite.Require().Equal(testCase.expectedResult, flattenSafeTrie)
		})
	}
}
func (suite *SafeTrieTestSuite) TestPathTreeIsEmpty() {
	suite.T().Parallel()
	for _, testCase := range []struct {
		initialState   []safeTrieTestArgs // initial state of the path tree before delete
		name           string
		expectedResult bool
	}{
		{
			name:           "is empty with empty trie",
			expectedResult: true,
		}, {
			name:           "is empty with not empty trie",
			expectedResult: false,
			initialState: []safeTrieTestArgs{
				{"/test/path/", []string{"test-target1"}},
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

// flattenSafeTrie converts a PathTrie into a map[string]Target
// This targets is not part of the SafeTrieTestSuite because it is also in use in IngressCacheTestSuite
func flattenSafeTrie(st *SafeTrie) (map[string]Target, error) {
	resultMap := make(map[string]Target)
	err := st.pathTrie.Walk(func(key string, value interface{}) error {
		// The Walk targets iterates over all nodes.
		// Only store key-value pairs where a non-nil value has been explicitly 'Put'.
		// If a node exists as an internal prefix (e.g., "/a" for "/a/b"), its 'value' will be nil.
		// We only care about the values that were actually stored.
		if value != nil {
			convertedValue, ok := value.(Target)
			if !ok {
				return fmt.Errorf("path value should be Target")
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

func (suite *SafeTrieTestSuite) generatePathsAndTargets(num int) []safeTrieTestArgs {
	args := make([]safeTrieTestArgs, num)
	for i := 0; i < num; i++ {
		path := fmt.Sprintf("/path/to/target/%d", i)
		target := []string{fmt.Sprintf("target-%d", i)}
		args[i] = safeTrieTestArgs{path: path, targets: target}
	}
	return args
}

func (suite *SafeTrieTestSuite) generateExpectedResultMap(num int) map[string]Target {
	expectedResult := make(map[string]Target)
	args := suite.generatePathsAndTargets(num)
	for i := 0; i < num; i++ {
		expectedResult[args[i].path] = SingleTarget(args[i].targets[0])
	}
	return expectedResult
}

// generateSafeTrieForTest creates a SafeTrie instance and sets the provided initial state
func (suite *SafeTrieTestSuite) generateSafeTrieForTest(initialSafeTrieState []safeTrieTestArgs) *SafeTrie {
	var err error
	safeTrie := NewSafeTrie()

	// set path tree with the provided required state
	for _, args := range initialSafeTrieState {
		err = safeTrie.Set(args.path, args.targets)
		suite.Require().NoError(err)
	}

	return safeTrie
}

func TestSafeTrieSuite(t *testing.T) {
	suite.Run(t, new(SafeTrieTestSuite))
}

// --- SingleTargetTestSuite ---
type SingleTargetTestSuite struct {
	suite.Suite
}

func (suite *SingleTargetTestSuite) TestEqual() {
	testCases := []struct {
		name           string
		target         Target
		expectedResult bool
	}{
		{
			name:           "Equal exact match",
			target:         SingleTarget("test-target1"),
			expectedResult: true,
		}, {
			name:           "Equal no match",
			target:         SingleTarget("test-target2"),
			expectedResult: false,
		}, {
			name:           "Equal empty target name no match",
			target:         SingleTarget(""),
			expectedResult: false,
		}, {
			name:           "Equal canary no match",
			target:         PairTarget{"test-target1", "test-target2"},
			expectedResult: false,
		},
	}

	for _, testCase := range testCases {
		suite.Run(testCase.name, func() {
			testSingleTarget := SingleTarget("test-target1")
			result := testSingleTarget.Equal(testCase.target)
			suite.Equal(testCase.expectedResult, result)
		})
	}
}

func (suite *SingleTargetTestSuite) TestToSliceString() {
	testCases := []struct {
		name             string
		singleTargetName string
		expectedResult   []string
	}{
		{
			name:             "ToSliceStringWithTarget",
			singleTargetName: "toSliceStringTarget",
			expectedResult:   []string{"toSliceStringTarget"},
		}, {
			name:             "ToSliceStringWithSpecialChars",
			singleTargetName: "my-target_123",
			expectedResult:   []string{"my-target_123"},
		},
	}

	for _, testCase := range testCases {
		suite.Run(testCase.name, func() {
			testSingleTarget := SingleTarget(testCase.singleTargetName)
			result := testSingleTarget.ToSliceString()
			suite.Equal(testCase.expectedResult, result)
			suite.Len(result, 1)
		})
	}
}

// TestSingleTargetTestSuite runs the test suite
func TestSingleTargetTestSuite(t *testing.T) {
	suite.Run(t, new(SingleTargetTestSuite))
}

// --- PairTargetTestSuite ---
type PairTargetTestSuite struct {
	suite.Suite
}

func (suite *PairTargetTestSuite) TestEqual() {
	testCases := []struct {
		name           string
		target         Target
		expectedResult bool
	}{
		{
			name:           "Equal match",
			target:         PairTarget{"test-target1", "test-target2"},
			expectedResult: true,
		}, {
			name:           "Equal no match",
			target:         PairTarget{"test-target1", "test-target3"},
			expectedResult: false,
		}, {
			name:           "Equal empty PairTarget no match",
			target:         PairTarget{},
			expectedResult: false,
		}, {
			name:           "Equal case sensitive",
			target:         PairTarget{"TEST-target1", "test-target2"},
			expectedResult: false,
		}, {
			name:           "Equal with single targets",
			target:         SingleTarget("test-target1"),
			expectedResult: false,
		},
	}

	for _, testCase := range testCases {
		suite.Run(testCase.name, func() {
			testTargets := PairTarget{"test-target1", "test-target2"}
			result := testTargets.Equal(testCase.target)
			suite.Equal(testCase.expectedResult, result)
		})
	}
}

func (suite *PairTargetTestSuite) TestToSliceString() {
	testCases := []struct {
		name           string
		target         [2]string
		expectedResult []string
	}{
		{
			name:           "ToSliceStringWithTargets",
			target:         [2]string{"test-target1", "test-target2"},
			expectedResult: []string{"test-target1", "test-target2"},
		}, {
			name:           "ToSliceStringWithSpecialChars",
			target:         [2]string{"my-target_123", "test-target2"},
			expectedResult: []string{"my-target_123", "test-target2"},
		},
	}

	for _, testCase := range testCases {
		suite.Run(testCase.name, func() {
			testTargets := PairTarget{testCase.target[0], testCase.target[1]}
			result := testTargets.ToSliceString()
			suite.Equal(testCase.expectedResult, result)
		})
	}
}

// TestTargetsTestSuite runs the test suite
func TestTargetsTestSuite(t *testing.T) {
	suite.Run(t, new(PairTargetTestSuite))
}
