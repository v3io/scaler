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

// Set adds targets for a given path. If the path does not exist, it creates it
func (st *SafeTrie) Set(path string, targets []string) error {
	if path == "" {
		return errors.New("path is empty")
	}

	newTarget, err := st.NewTarget(targets)
	if err != nil {
		return errors.Wrap(err, "failed to create Target")
	}

	st.rwMutex.Lock()
	defer st.rwMutex.Unlock()

	st.pathTrie.Put(path, newTarget)
	return nil
}

// Delete removes the targets from a path and cleans up the longest suffix of the path only used by these targets
func (st *SafeTrie) Delete(path string, targets []string) error {
	st.rwMutex.Lock()
	defer st.rwMutex.Unlock()

	pathValue := st.pathTrie.Get(path)
	if pathValue == nil {
		// If pathValue is nil, the path does not exist, so nothing to delete
		return nil
	}

	currentTarget, ok := pathValue.(Target)
	if !ok {
		return errors.Errorf("path value should be Target, got %T", pathValue)
	}

	targetToDelete, err := st.NewTarget(targets)
	if err != nil {
		return errors.Wrap(err, "failed to create Target for targets")
	}

	// If the Target instances do not match, nothing to delete
	if !currentTarget.Equal(targetToDelete) {
		return nil
	}

	st.pathTrie.Delete(path)
	return nil
}

// Get retrieve the closest prefix matching the path and returns the associated targets
func (st *SafeTrie) Get(path string) (Target, error) {
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

	target, ok := walkPathResult.(Target)
	if !ok {
		return nil, errors.Errorf("walkPathResult value should be Target, got %v", walkPathResult)
	}

	return target, nil
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

// NewTarget returns a Target based on the length of the input slice
func (st *SafeTrie) NewTarget(inputs []string) (Target, error) {
	switch len(inputs) {
	case 1:
		return SingleTarget(inputs[0]), nil
	case 2:
		return PairTarget{inputs[0], inputs[1]}, nil
	default:
		return nil, errors.New("unexpected input length")
	}
}

// ----- implementations for Target interface -----

type SingleTarget string

func (s SingleTarget) Equal(otherTarget Target) bool {
	otherSingleTarget, ok := otherTarget.(SingleTarget)
	if !ok {
		return false
	}

	return string(s) == string(otherSingleTarget)
}

func (s SingleTarget) ToSliceString() []string {
	return []string{string(s)}
}

type PairTarget [2]string

func (p PairTarget) Equal(otherTarget Target) bool {
	target, ok := otherTarget.(PairTarget)
	if !ok {
		return false
	}

	if p[0] == target[0] && p[1] == target[1] {
		return true
	}

	if p[0] == target[1] && p[1] == target[0] {
		return true
	}

	return false
}

func (p PairTarget) ToSliceString() []string {
	return []string{p[0], p[1]}
}
