package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// LoadManifests reads explicit YAML files and recursively discovers YAML files
// in directories. Files are ordered deterministically before concatenation.
func LoadManifests(ctx context.Context, paths []string) ([]byte, error) {
	files := make([]string, 0)
	seen := map[string]bool{}
	addFile := func(path string) {
		path = filepath.Clean(path)
		if !seen[path] {
			seen[path] = true
			files = append(files, path)
		}
	}
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("inspect Kubernetes manifest path %s: %w", path, err)
		}
		if !info.IsDir() {
			addFile(path)
			continue
		}
		err = filepath.WalkDir(path, func(candidate string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			extension := strings.ToLower(filepath.Ext(candidate))
			if extension == ".yaml" || extension == ".yml" {
				addFile(candidate)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk Kubernetes manifest directory %s: %w", path, err)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no Kubernetes YAML manifests found")
	}
	sort.Strings(files)
	var manifest bytes.Buffer
	for index, path := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read Kubernetes manifest %s: %w", path, err)
		}
		if index > 0 {
			manifest.WriteString("\n---\n")
		}
		// Preserve the originating filename when several manifest files are
		// combined into the byte stream consumed by the extractor. Quoting keeps
		// unusual but valid filenames from creating ambiguous marker lines.
		manifest.WriteString("# Fleck-Source: ")
		manifest.WriteString(strconv.Quote(filepath.Clean(path)))
		manifest.WriteByte('\n')
		manifest.Write(contents)
	}
	return manifest.Bytes(), nil
}
