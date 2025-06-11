package ingresscache

import (
	"fmt"
	"github.com/dghubble/trie"
	"slices"

	"github.com/nuclio/errors"
)

type PathTree struct {
	t *trie.PathTrie
}

// NewPathTree creates a new PathTree instance
func NewPathTree() *PathTree {
	return &PathTree{trie.NewPathTrie()}
}

// SetFunctionName sets a function for a given path. If the path does not exist, it creates it
func (p *PathTree) SetFunctionName(path string, function string) error {
	if path == "" {
		return errors.New("path is empty")
	}

	if function == "" {
		return errors.New("function is empty")
	}

	// get the exact path value in order to avoid creating a new path if it already exists
	pathValue := p.t.Get(path)
	if pathValue == nil {
		p.t.Put(path, []string{function})
		return nil
	}

	pathFunctionNames, ok := pathValue.([]string)
	if !ok {
		return errors.New(fmt.Sprintf("value is not a []string, got: %T", pathValue))
	}

	if slices.Contains(pathFunctionNames, function) {
		return nil
	}

	pathFunctionNames = append(pathFunctionNames, function)
	p.t.Put(path, pathFunctionNames)

	return nil
}

// DeleteFunctionName removes a function from a path and also deletes the path if the function is the only one associated with that path
func (p *PathTree) DeleteFunctionName(path string, function string) error {
	pathValue := p.t.Get(path)
	if pathValue == nil {
		// If pathValue is nil, the path does not exist, so nothing to delete
		return nil
	}

	pathFunctionNames, ok := pathValue.([]string)
	if !ok {
		return errors.New(fmt.Sprintf("path value should be []string, got %T", pathValue))
	}

	// If the function is the only value, delete the path
	if len(pathFunctionNames) == 1 {
		if pathFunctionNames[0] == function {
			p.t.Delete(path)
			return nil
		}
		return errors.New(fmt.Sprintf("the function-name doesn't exists in path, skipping delete. function-name: %s, path: %s", function, path))
	}

	// TODO - will be removed once moving into efficient pathFunctionNames implementation (i.e. not using slices)
	pathFunctionNames = excludeElemFromSlice(pathFunctionNames, function)
	p.t.Put(path, pathFunctionNames)
	return nil
}

// GetFunctionName retrieve the closest prefix matching the path and returns the associated functions
func (p *PathTree) GetFunctionName(path string) ([]string, error) {
	var walkPathResult interface{}
	if path == "" {
		return nil, errors.New("path is empty")
	}

	if err := p.t.WalkPath(path, func(_ string, value interface{}) error {
		if value != nil {
			walkPathResult = value
		}

		return nil
	}); err != nil {
		return nil, errors.New(fmt.Sprintf("no value found for path: %s", path))
	}

	output, ok := walkPathResult.([]string)
	if !ok {
		return nil, errors.New(fmt.Sprintf("value is not a []string, value: %v", walkPathResult))
	}

	return output, nil
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
