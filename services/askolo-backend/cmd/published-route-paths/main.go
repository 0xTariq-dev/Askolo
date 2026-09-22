package main

import (
	"fmt"
	"strings"

	"askolo/backend/internal/httpapi"
)

func main() {
	paths := httpapi.PublishedRoutePaths()
	quotedPaths := make([]string, len(paths))
	for i, path := range paths {
		quotedPaths[i] = fmt.Sprintf("%q", path)
	}
	fmt.Printf("paths = [%s]\n", strings.Join(quotedPaths, ", "))
}
