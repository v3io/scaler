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
	"sync"

	"github.com/dghubble/trie"
	"github.com/nuclio/errors"
)

type SafeTrie struct {
	pathTrie *trie.PathTrie
	rwMutex  sync.RWMutex
}

// NewSafeTrie creates a new SafeTrie instance
func NewSafeTrie() *SafeTrie {
	return &SafeTrie{
		pathTrie: trie.NewPathTrie(),
		rwMutex:  sync.RWMutex{},
	}
}

// Set adds a functions for a given path. If the path does not exist, it creates it
func (st *SafeTrie) Set(path string, functions []string) error {
	if path == "" {
		return errors.New("path is empty")
	}

	functionTarget, err := st.NewFunctionTarget(functions)
	if err != nil {
		return errors.Wrap(err, "failed to create FunctionTarget")
	}

	st.rwMutex.Lock()
	defer st.rwMutex.Unlock()

	st.pathTrie.Put(path, functionTarget)
	return nil
}

// Delete removes a functions from a path and cleans up the longest suffix of the path only used by that functions
func (st *SafeTrie) Delete(path string, functions []string) error {
	st.rwMutex.Lock()
	defer st.rwMutex.Unlock()

	pathValue := st.pathTrie.Get(path)
	if pathValue == nil {
		// If pathValue is nil, the path does not exist, so nothing to delete
		return nil
	}

	currentFunctionNames, ok := pathValue.(FunctionTarget)
	if !ok {
		return errors.Errorf("path value should be FunctionTarget, got %T", pathValue)
	}

	requestFunctionNames, err := st.NewFunctionTarget(functions)
	if err != nil {
		return errors.Wrap(err, "failed to create FunctionTarget for functions")
	}

	// If the functionTargets do not match, nothing to delete
	if !currentFunctionNames.Equal(requestFunctionNames) {
		return nil
	}

	st.pathTrie.Delete(path)
	return nil
}

// Get retrieve the closest prefix matching the path and returns the associated functions
func (st *SafeTrie) Get(path string) (FunctionTarget, error) {
	var walkPathResult interface{}
	if path == "" {
		return nil, errors.New("path is empty")
	}

	st.rwMutex.RLock()
	defer st.rwMutex.RUnlock()

	if err := st.pathTrie.WalkPath(path, func(_ string, value interface{}) error {
		if value != nil {
			walkPathResult = value
		}

		return nil
	}); err != nil {
		return nil, errors.Errorf("no value found for path: %s", path)
	}

	functionNames, ok := walkPathResult.(FunctionTarget)
	if !ok {
		return nil, errors.Errorf("walkPathResult value should be FunctionTarget, got %v", walkPathResult)
	}

	return functionNames, nil
}

// IsEmpty return true if the SafeTrie is empty
func (st *SafeTrie) IsEmpty() bool {
	walkResult := st.pathTrie.Walk(func(_ string, value interface{}) error {
		if value != nil {
			return errors.New("trie is not empty")
		}
		return nil
	})
	return walkResult == nil
}

// NewFunctionTarget returns a FunctionTarget based on the length of the input slice
func (st *SafeTrie) NewFunctionTarget(inputs []string) (FunctionTarget, error) {
	switch len(inputs) {
	case 1:
		return SingleTarget(inputs[0]), nil
	case 2:
		return &CanaryTarget{[2]string{inputs[0], inputs[1]}}, nil
	default:
		return nil, errors.New("unexpected input length")
	}
}

// ----- implementations for FunctionTarget interface -----

type SingleTarget string

func (s SingleTarget) Equal(functionTarget FunctionTarget) bool {
	singleFunctionTarget, ok := functionTarget.(SingleTarget)
	if !ok {
		return false
	}

	return string(s) == string(singleFunctionTarget)
}

func (s SingleTarget) ToSliceString() []string {
	return []string{string(s)}
}

type CanaryTarget struct {
	functionNames [2]string
}

func (c CanaryTarget) Equal(functionTarget FunctionTarget) bool {
	canaryTarget, ok := functionTarget.(*CanaryTarget)
	if !ok {
		return false
	}

	if c.functionNames[0] == canaryTarget.functionNames[0] && c.functionNames[1] == canaryTarget.functionNames[1] {
		return true
	}

	if c.functionNames[0] == canaryTarget.functionNames[1] && c.functionNames[1] == canaryTarget.functionNames[0] {
		return true
	}

	return false
}

func (c CanaryTarget) ToSliceString() []string {
	return []string{c.functionNames[0], c.functionNames[1]}
}
