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

	"github.com/v3io/scaler/pkg/ingresscache/mock"

	"github.com/nuclio/errors"
	"github.com/nuclio/logger"
	nucliozap "github.com/nuclio/zap"
	"github.com/stretchr/testify/suite"
)

type IngressCacheTestSuite struct {
	suite.Suite
	logger       logger.Logger
	ingressCache *IngressCache
}

type testIngressCacheArgs struct {
	host     string
	path     string
	function string
}

// used to mock the IngressHostsTree interface per test
type mockFunction func() *mock.SafeTrie

const (
	testPath          = "/test/path"
	testHost          = "example.com"
	testFunctionName1 = "testFunction1"
	testFunctionName2 = "testFunction2"
)

func (suite *IngressCacheTestSuite) SetupTest() {
	var err error

	suite.logger, err = nucliozap.NewNuclioZapTest("test")
	suite.Require().NoError(err)
	suite.ingressCache = NewIngressCache(suite.logger)
}

func (suite *IngressCacheTestSuite) SetupSubTest(testHost string, testMocks mockFunction) {
	suite.ingressCache = NewIngressCache(suite.logger)

	if m := testMocks(); m != nil {
		// mock==nil is used to check for non-existing host
		suite.ingressCache.syncMap.Store(testHost, m)
	}
}

func (suite *IngressCacheTestSuite) TestGet() {
	for _, testCase := range []struct {
		name           string
		args           testIngressCacheArgs
		expectedResult []string
		shouldFail     bool
		errorMessage   string
		testMocks      mockFunction
	}{
		{
			name:           "Get two functionName",
			args:           testIngressCacheArgs{testHost, testPath, ""},
			expectedResult: []string{testFunctionName1, testFunctionName2},
			testMocks: func() *mock.SafeTrie {
				m := &mock.SafeTrie{}
				m.On("GetFunctionNames", testPath).Return([]string{testFunctionName1, testFunctionName2}, nil)
				return m
			},
		}, {
			name:           "Get single functionName",
			args:           testIngressCacheArgs{testHost, testPath, ""},
			expectedResult: []string{testFunctionName1},
			testMocks: func() *mock.SafeTrie {
				m := &mock.SafeTrie{}
				m.On("GetFunctionNames", testPath).Return([]string{testFunctionName1}, nil)
				return m
			},
		}, {
			name:           "Get with not existing host",
			args:           testIngressCacheArgs{"not.exist", testPath, ""},
			expectedResult: nil,
			testMocks: func() *mock.SafeTrie {
				return nil
			},
			shouldFail:   true,
			errorMessage: "host does not exist",
		},
	} {
		suite.Run(testCase.name, func() {
			suite.SetupSubTest(testCase.args.host, testCase.testMocks)

			resultFunctionNames, err := suite.ingressCache.Get(testCase.args.host, testCase.args.path)
			if testCase.shouldFail {
				suite.Require().NotNil(err)
				suite.Require().Contains(err.Error(), testCase.errorMessage)
				suite.Require().Nil(resultFunctionNames)
			} else {
				suite.Require().NoError(err)
				suite.Require().Equal(testCase.expectedResult, resultFunctionNames)
			}
		})
	}
}

func (suite *IngressCacheTestSuite) TestSet() {
	for _, testCase := range []struct {
		name         string
		args         testIngressCacheArgs
		shouldFail   bool
		errorMessage string
		testMocks    mockFunction
	}{
		{
			name: "Set new host",
			args: testIngressCacheArgs{testHost, testPath, testFunctionName1},
			testMocks: func() *mock.SafeTrie {
				return nil
			}, // nil is used to check for non-existing host
		}, {
			name: "Set another functionName for existing host",
			args: testIngressCacheArgs{testHost, testPath, testFunctionName2},
			testMocks: func() *mock.SafeTrie {
				m := &mock.SafeTrie{}
				m.On("GetFunctionNames", testPath).Return([]string{testFunctionName1}, nil).Once()
				m.On("SetFunctionName", testPath, testFunctionName2).Return(nil).Once()
				return m
			},
		}, {
			name: "Set existing functionName for existing host and path",
			args: testIngressCacheArgs{testHost, testPath, testFunctionName1},
			testMocks: func() *mock.SafeTrie {
				m := &mock.SafeTrie{}
				m.On("SetFunctionName", testPath, testFunctionName1).Return(nil).Once()
				return m
			},
		},
	} {
		suite.Run(testCase.name, func() {
			suite.SetupSubTest(testCase.args.host, testCase.testMocks)

			err := suite.ingressCache.Set(testCase.args.host, testCase.args.path, testCase.args.function)
			if testCase.shouldFail {
				suite.Require().NotNil(err)
				suite.Require().Contains(err.Error(), testCase.errorMessage)
			} else {
				suite.Require().NoError(err)
			}
		})
	}
}

func (suite *IngressCacheTestSuite) TestDelete() {
	type getFunctionAfterDeleteArgs struct { // this struct enables multiple get tests after delete
		args           testIngressCacheArgs
		expectedResult []string
		shouldFail     bool
		errorMessage   string
	}

	for _, testCase := range []struct {
		name               string
		args               testIngressCacheArgs
		shouldFail         bool
		errorMessage       string
		testMocks          mockFunction
		getAfterDeleteArgs []getFunctionAfterDeleteArgs
	}{
		{
			name: "Delete not existed host",
			args: testIngressCacheArgs{testHost, testPath, testFunctionName1},
			testMocks: func() *mock.SafeTrie {
				return nil
			}, // nil is used to check for non-existing host
		}, {
			name: "Delete last function in host, validate host deletion",
			args: testIngressCacheArgs{testHost, testPath, testFunctionName2},
			testMocks: func() *mock.SafeTrie {
				m := &mock.SafeTrie{}
				m.On("DeleteFunctionName", testPath, testFunctionName2).Return(nil).Once()
				m.On("IsEmpty").Return(true).Once()
				return m
			},
			getAfterDeleteArgs: []getFunctionAfterDeleteArgs{
				{
					args:           testIngressCacheArgs{testHost, testPath, testFunctionName1},
					expectedResult: nil,
					shouldFail:     true,
					errorMessage:   "host does not exist",
				},
			},
		}, {
			name: "Fail to delete and validate host wasn't deleted",
			args: testIngressCacheArgs{testHost, testPath, testFunctionName2},
			testMocks: func() *mock.SafeTrie {
				m := &mock.SafeTrie{}
				m.On("DeleteFunctionName", testPath, testFunctionName2).Return(errors.New("mock error")).Once()
				m.On("GetFunctionNames", testPath).Return([]string{testFunctionName2}, nil).Once()
				return m
			},
			getAfterDeleteArgs: []getFunctionAfterDeleteArgs{
				{
					args:           testIngressCacheArgs{testHost, testPath, testFunctionName2},
					expectedResult: []string{testFunctionName2},
				},
			},
			shouldFail:   true,
			errorMessage: "failed to delete function name from the ingress host tree",
		}, {
			name: "Delete not last function in path and validate host wasn't deleted",
			args: testIngressCacheArgs{testHost, testPath, testFunctionName2},
			testMocks: func() *mock.SafeTrie {
				m := &mock.SafeTrie{}
				m.On("DeleteFunctionName", testPath, testFunctionName2).Return(nil).Once()
				m.On("IsEmpty").Return(false).Once()
				m.On("GetFunctionNames", testPath).Return([]string{testFunctionName1}, nil).Once()
				return m
			},
			getAfterDeleteArgs: []getFunctionAfterDeleteArgs{
				{
					args:           testIngressCacheArgs{testHost, testPath, testFunctionName1},
					expectedResult: []string{testFunctionName1},
				},
			},
		},
	} {
		suite.Run(testCase.name, func() {
			suite.SetupSubTest(testCase.args.host, testCase.testMocks)

			err := suite.ingressCache.Delete(testCase.args.host, testCase.args.path, testCase.args.function)
			if testCase.shouldFail {
				suite.Require().NotNil(err)
				suite.Require().Contains(err.Error(), testCase.errorMessage)
			} else {
				suite.Require().NoError(err)
			}

			// After delete, check that the expected paths and functions are still there
			for _, getAfterDeleteArgs := range testCase.getAfterDeleteArgs {
				result, err := suite.ingressCache.Get(getAfterDeleteArgs.args.host, getAfterDeleteArgs.args.path)
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

func TestIngressCache(t *testing.T) {
	suite.Run(t, new(IngressCacheTestSuite))
}
