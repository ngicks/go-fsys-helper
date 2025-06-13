package overlay

import (
	"bufio"
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/ngicks/go-fsys-helper/vroot"
)

var (
	MetadataStoreSimpleTextWhiteout  = "whiteout"
	MetadataStoreSimpleTextTmpSuffix = ".tmp"
)

var _ MetadataStore = (*MetadataStoreSimpleText)(nil)

// MetadataStoreSimpleText stores metadata using simple new line delimited text file.
// It is optimized for relatively small number of files.
// It stores all records on memory and write to file only if there's change in the list.
//
// It only reads and writes "whiteout" and "whiteout.tmp".
// But future additions may use other names so the fsys should be dedicated for this usage.
//
// The file "whiteout" lists whited-out files.
// Each lines are quoted following convention strconv.Quote does.
// The line is always forward slash "/" separated and has no leading path separator ("./" or "../")
// where it can pass [fs.ValidPath].
type MetadataStoreSimpleText struct {
	mu   sync.RWMutex
	root *pathNode // Tree structure for efficient parent path checking
	fsys vroot.Rooted
}

// pathNode represents a node in the whiteout path tree
type pathNode struct {
	whited   bool
	children map[string]*pathNode
}

// NewMetadataStoreSimpleText creates a new MetadataStoreSimpleText
func NewMetadataStoreSimpleText(fsys vroot.Rooted) *MetadataStoreSimpleText {
	return &MetadataStoreSimpleText{
		fsys: fsys,
	}
}

// loadIfNeeded loads the whiteout file if not already loaded
func (m *MetadataStoreSimpleText) loadIfNeeded() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.root != nil {
		return nil
	}

	// Clean up any leftover temporary file
	tmpName := MetadataStoreSimpleTextWhiteout + MetadataStoreSimpleTextTmpSuffix
	_ = m.fsys.Remove(tmpName)

	// Try to open the whiteout file
	file, err := m.fsys.Open(MetadataStoreSimpleTextWhiteout)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// File doesn't exist, tree is already initialized as empty
			m.root = &pathNode{children: make(map[string]*pathNode)}
			return nil
		}
		return err
	}
	defer file.Close()

	m.root = &pathNode{children: make(map[string]*pathNode)}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			// Unquote the line to get the actual path
			unquoted, err := strconv.Unquote(line)
			if err != nil {
				// If unquoting fails, skip this line
				continue
			}

			// Validate path - skip invalid paths
			if !fs.ValidPath(unquoted) {
				continue
			}

			// Add path to tree
			m.addPathToTree(unquoted)
		}
	}

	if err := scanner.Err(); err != nil {
		// This is thought unlikely.
		// Not setting nil to m.root
		return err
	}

	return nil
}

// addPathToTree adds a path to the tree structure (must hold lock)
// Note: RecordWhiteout rejects "." so this should never be called with "."
func (m *MetadataStoreSimpleText) addPathToTree(path string) {
	if path == "" || path == "." {
		// This should not happen as RecordWhiteout rejects ".", but keep for safety
		m.root.whited = true
		return
	}

	parts := strings.Split(path, "/")
	current := m.root

	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}

		if current.children == nil {
			current.children = make(map[string]*pathNode)
		}

		if current.children[part] == nil {
			current.children[part] = &pathNode{children: make(map[string]*pathNode)}
		}

		current = current.children[part]
	}

	current.whited = true
}

// removePathFromTree removes a path from the tree structure (must hold lock)
func (m *MetadataStoreSimpleText) removePathFromTree(path string) {
	if path == "" || path == "." {
		m.root.whited = false
		return
	}

	parts := strings.Split(path, "/")
	current := m.root

	// Navigate to the parent of the target path
	for _, part := range parts[:len(parts)-1] {
		if part == "" || part == "." {
			continue
		}

		if current.children == nil || current.children[part] == nil {
			// Path doesn't exist, nothing to remove
			return
		}

		current = current.children[part]
	}

	// Get the final part
	finalPart := parts[len(parts)-1]
	if finalPart == "" || finalPart == "." {
		current.whited = false
		return
	}

	if current.children != nil && current.children[finalPart] != nil {
		// Mark as not whited out
		current.children[finalPart].whited = false

		// If the node has no children and is not whited, we can remove it
		// to keep the tree clean, but this is optional optimization
		if len(current.children[finalPart].children) == 0 && !current.children[finalPart].whited {
			delete(current.children, finalPart)
		}
	}
}

// stackItem represents an item on the iteration stack
type stackItem struct {
	node *pathNode
	path string
}

// whitedPaths returns an iterator over all whited-out paths in the tree
func (m *MetadataStoreSimpleText) whitedPaths() func(func(string) bool) {
	return func(yield func(string) bool) {
		if m.root == nil {
			return
		}

		// Use iterative depth-first traversal with explicit stack to avoid recursion
		stack := []stackItem{{node: m.root, path: ""}}

		for len(stack) > 0 {
			// Pop from stack
			item := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			node, currentPath := item.node, item.path

			// If this node is whited out, yield the path
			// Note: root node (currentPath == "") should never be whited since RecordWhiteout rejects "."
			if node.whited {
				var path string
				if currentPath == "" {
					path = "." // This should not happen, but kept for safety
				} else {
					path = currentPath
				}
				if !yield(path) {
					return
				}
			}

			// Push children onto stack (in reverse order to maintain consistent ordering)
			var childNames []string
			for name := range node.children {
				childNames = append(childNames, name)
			}

			// Sort for consistent ordering
			slices.Sort(childNames)
			for i := len(childNames) - 1; i >= 0; i-- {
				name := childNames[i]
				child := node.children[name]

				var childPath string
				if currentPath == "" {
					childPath = name
				} else {
					childPath = currentPath + "/" + name
				}

				stack = append(stack, stackItem{node: child, path: childPath})
			}
		}
	}
}

// save writes the current state to the whiteout file
func (m *MetadataStoreSimpleText) save() error {
	tmpName := MetadataStoreSimpleTextWhiteout + MetadataStoreSimpleTextTmpSuffix

	// Create temporary file
	tmpFile, err := m.fsys.Create(tmpName)
	if err != nil {
		return err
	}

	defer func() {
		tmpFile.Close()
		// Clean up temp file if something goes wrong
		if err != nil {
			m.fsys.Remove(tmpName)
		}
	}()

	// Write all whited-out paths to temp file using iterator
	for path := range m.whitedPaths() {
		// Quote the line for safe storage
		quoted := strconv.Quote(path)
		if _, err := tmpFile.WriteString(quoted + "\n"); err != nil {
			return err
		}
	}

	// Sync the file
	if err := tmpFile.Sync(); err != nil {
		return err
	}

	// Close the temp file
	if err := tmpFile.Close(); err != nil {
		return err
	}

	// Atomically rename temp file to final name
	if err := m.fsys.Rename(tmpName, MetadataStoreSimpleTextWhiteout); err != nil {
		return err
	}

	return nil
}

// QueryWhiteout checks if a path or any of its parents is whited out
func (m *MetadataStoreSimpleText) QueryWhiteout(name string) (has bool, err error) {
	if err := m.loadIfNeeded(); err != nil {
		return false, err
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Normalize the path
	name = filepath.ToSlash(filepath.Clean(name))

	// Check if root is whited out
	if m.root.whited {
		return true, nil
	}

	// Handle empty path or "."
	if name == "" || name == "." {
		return m.root.whited, nil
	}

	// Split path into parts
	parts := strings.Split(name, "/")
	current := m.root

	// Walk through the path, checking if any parent is whited out
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}

		// Check if current level is whited out (parent of remaining path)
		if current.whited {
			return true, nil
		}

		// Move to next level
		if current.children == nil || current.children[part] == nil {
			// Path doesn't exist in tree, so not whited out
			return false, nil
		}

		current = current.children[part]
	}

	// Check if the final path component itself is whited out
	return current.whited, nil
}

// RecordWhiteout adds a path to the whiteout list
func (m *MetadataStoreSimpleText) RecordWhiteout(name string) error {
	if err := m.loadIfNeeded(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Normalize the path
	name = filepath.ToSlash(filepath.Clean(name))

	// Reject root path as per interface requirement
	if name == "." {
		return &fs.PathError{Op: "record_whiteout", Path: name, Err: errors.New("cannot whiteout root path")}
	}

	// Validate path - return error for invalid paths
	if !fs.ValidPath(name) {
		return &fs.PathError{Op: "record_whiteout", Path: name, Err: errors.New("invalid path")}
	}

	// Add path to tree
	m.addPathToTree(name)

	// Save immediately
	return m.save()
}

// RemoveWhiteout removes a path from the whiteout list
func (m *MetadataStoreSimpleText) RemoveWhiteout(name string) error {
	if err := m.loadIfNeeded(); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Normalize the path
	name = filepath.ToSlash(filepath.Clean(name))

	// Remove path from tree
	m.removePathFromTree(name)

	// Save immediately
	return m.save()
}
