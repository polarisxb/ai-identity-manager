package identity

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type LogFile struct {
	Path    string
	Kind    string
	Size    int64
	ModTime time.Time
}

func DiscoverLogFiles(clashDir string) ([]LogFile, error) {
	candidates := []struct {
		kind string
		path string
	}{
		{"service-latest", filepath.Join(clashDir, "logs", "service", "service_latest.log")},
		{"latest", filepath.Join(clashDir, "logs", "latest.log")},
		{"service-latest-flat", filepath.Join(clashDir, "logs", "service_latest.log")},
		{"root-latest", filepath.Join(clashDir, "latest.log")},
		{"root-service-latest", filepath.Join(clashDir, "service_latest.log")},
	}
	files := make([]LogFile, 0, len(candidates))
	seen := map[string]bool{}
	for _, candidate := range candidates {
		addLogFile(&files, seen, candidate.kind, candidate.path)
	}

	serviceDir := filepath.Join(clashDir, "logs", "service")
	matches, _ := filepath.Glob(filepath.Join(serviceDir, "*.log"))
	sort.Slice(matches, func(i, j int) bool {
		left, _ := os.Stat(matches[i])
		right, _ := os.Stat(matches[j])
		if left == nil || right == nil {
			return matches[i] < matches[j]
		}
		return left.ModTime().After(right.ModTime())
	})
	for i, match := range matches {
		if i >= 5 {
			break
		}
		addLogFile(&files, seen, "service-rotated", match)
	}

	return files, nil
}

func addLogFile(files *[]LogFile, seen map[string]bool, kind, path string) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || seen[path] {
		return
	}
	seen[path] = true
	*files = append(*files, LogFile{
		Path:    path,
		Kind:    kind,
		Size:    info.Size(),
		ModTime: info.ModTime(),
	})
}

func ReadLogFiles(files []LogFile, maxBytesPerFile int64) (string, []string, error) {
	var out []byte
	sources := []string{}
	for _, file := range files {
		data, err := readTail(file.Path, maxBytesPerFile)
		if err != nil {
			continue
		}
		out = append(out, data...)
		out = append(out, '\n')
		sources = append(sources, file.Path)
	}
	return string(out), sources, nil
}

func readTail(path string, maxBytes int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if maxBytes <= 0 || info.Size() <= maxBytes {
		return io.ReadAll(file)
	}
	if _, err := file.Seek(-maxBytes, io.SeekEnd); err != nil {
		return nil, err
	}
	return io.ReadAll(file)
}
