// Package pathutil provides shared path validation helpers.
package pathutil

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ValidateFilePath validates a file path for path traversal and invalid characters.
// Uses segment-based detection so that "scripts/../etc/passwd" is rejected before
// cleaning (cleaned path would be "etc/passwd" and could bypass a simple ".." check).
// Returns an error if the path is empty, contains null bytes, or has ".." in any segment.
func ValidateFilePath(filePath string) error {
	if filePath == "" {
		return fmt.Errorf("file path cannot be empty")
	}
	if strings.Contains(filePath, "\x00") {
		return fmt.Errorf("file path contains invalid characters")
	}

	normalized := filepath.ToSlash(filePath)
	segments := strings.Split(normalized, "/")
	for _, segment := range segments {
		if segment == ".." {
			return fmt.Errorf("file path contains path traversal: %q", filePath)
		}
	}
	if strings.HasPrefix(normalized, "../") || normalized == ".." {
		return fmt.Errorf("file path contains path traversal: %q", filePath)
	}
	return nil
}
