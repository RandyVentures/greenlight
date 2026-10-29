// Package sourcepath identifies common non-shipping source inputs. It does not
// resolve a build graph; generated artifacts should be inspected separately.
package sourcepath

import (
	"path/filepath"
	"strings"
)

func SkipDir(name string) bool {
	switch name {
	case "node_modules", ".git", "Pods", "build", "dist", "out", ".expo", "DerivedData", ".next", "vendor", ".build", ".swiftpm", ".venv", ".cache":
		return true
	}
	return false
}
func IsTest(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	for _, suffix := range []string{".test.ts", ".test.tsx", ".test.js", ".test.jsx", ".spec.ts", ".spec.tsx", ".spec.js", ".spec.jsx", "tests.swift"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == "__tests__" || strings.HasSuffix(part, "Tests") || strings.HasSuffix(part, "UITests") {
			return true
		}
	}
	return false
}
