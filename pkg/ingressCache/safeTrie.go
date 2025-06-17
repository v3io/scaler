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
	"slices"
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

// SetFunctionName sets a function for a given path. If the path does not exist, it creates it
func (st *SafeTrie) SetFunctionName(path string, function string) error {
	if path == "" {
		return errors.New("path is empty")
	}

	if function == "" {
		return errors.New("function is empty")
	}

	st.rwMutex.Lock()
	defer st.rwMutex.Unlock()

	// get the exact path value in order to avoid creating a new path if it already exists
	pathValue := st.pathTrie.Get(path)
	if pathValue == nil {
		st.pathTrie.Put(path, []string{function})
		return nil
	}

	pathFunctionNames, ok := pathValue.([]string)
	if !ok {
		return errors.Errorf("value is not a []string, got: %T", pathValue)
	}

	if slices.Contains(pathFunctionNames, function) {
		// If the function already exists at this path, skip adding it to prevent duplicates
		return nil
	}

	pathFunctionNames = append(pathFunctionNames, function)
	st.pathTrie.Put(path, pathFunctionNames)

	return nil
}

// DeleteFunctionName removes a function from a path and also deletes the path if the function is the only one associated with that path
func (st *SafeTrie) DeleteFunctionName(path string, function string) error {
	st.rwMutex.Lock()
	defer st.rwMutex.Unlock()

	pathValue := st.pathTrie.Get(path)
	if pathValue == nil {
		// If pathValue is nil, the path does not exist, so nothing to delete
		return nil
	}

	pathFunctionNames, ok := pathValue.([]string)
	if !ok {
		return errors.Errorf("path value should be []string, got %T", pathValue)
	}

	// If the function to delete matches the current function name and it's the only value, delete the path
	if len(pathFunctionNames) == 1 {
		if pathFunctionNames[0] == function {
			st.pathTrie.Delete(path)
		}
		return nil
	}

	// TODO - will be removed once moving into efficient pathFunctionNames implementation (i.e. not using slices)
	pathFunctionNames = excludeElemFromSlice(pathFunctionNames, function)
	st.pathTrie.Put(path, pathFunctionNames)
	return nil
}

// GetFunctionName retrieve the closest prefix matching the path and returns the associated functions
func (st *SafeTrie) GetFunctionName(path string) ([]string, error) {
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

	functionNames, ok := walkPathResult.([]string)
	if !ok {
		return nil, errors.Errorf("value is not a []string, value: %v", walkPathResult)
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

// TODO - will be removed once moving into efficient pathFunctionNames implementation (i.e. not using slices)
func excludeElemFromSlice(slice []string, elem string) []string {
	// 'j' is the "write" index. It tracks where the next element to keep should be placed.
	j := 0

	// Iterate through the original slice using 'i' as the "read" index.
	for i := 0; i < len(slice); i++ {
		// If the current element (s[i]) is NOT the one we want to remove,
		// copy it to the current "write" position (s[j]).
		if slice[i] != elem {
			slice[j] = slice[i]
			j++ // Increment the write index, preparing for the next element to keep.
		}
	}

	slice[j] = "" // This helps the garbage collector reclaim memory for the string data

	// Return a re-sliced version of 's' up to the new length 'j'.
	return slice[:j]
}
