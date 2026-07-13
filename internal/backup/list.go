package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// List returns completed manifest-backed backup directories under root.
func List(root string) ([]Manifest, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list backups: %w", err)
	}
	type item struct {
		path     string
		manifest Manifest
	}
	items := []item{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		manifest, err := ReadManifest(path)
		if err != nil {
			continue
		}
		items = append(items, item{path: path, manifest: manifest})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].manifest.CreatedAt.After(items[j].manifest.CreatedAt) })
	result := make([]Manifest, len(items))
	for i := range items {
		result[i] = items[i].manifest
	}
	return result, nil
}

// PrunePolicy removes older completed backups according to calendar buckets.
func PrunePolicy(root string, policy RetentionPolicy, now time.Time, dryRun bool) ([]string, error) {
	if policy.Daily < 0 || policy.Weekly < 0 || policy.Monthly < 0 {
		return nil, fmt.Errorf("backup retention cannot be negative")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list backups: %w", err)
	}
	type item struct {
		path      string
		createdAt time.Time
	}
	items := []item{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		manifest, err := ReadManifest(path)
		if err == nil {
			items = append(items, item{path: path, createdAt: manifest.CreatedAt})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].createdAt.After(items[j].createdAt) })
	if now.IsZero() {
		now = time.Now().UTC()
	}
	seen := map[string]int{}
	removed := []string{}
	for _, item := range items {
		if policy.KeepFor(item.createdAt, now, seen) {
			continue
		}
		removed = append(removed, item.path)
		if !dryRun {
			if err := os.RemoveAll(item.path); err != nil {
				return removed, fmt.Errorf("remove backup %s: %w", item.path, err)
			}
		}
	}
	return removed, nil
}

// Prune removes older completed backup directories while retaining keep sets.
func Prune(root string, keep int, dryRun bool) ([]string, error) {
	if keep < 0 {
		return nil, fmt.Errorf("backup retention cannot be negative")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list backups: %w", err)
	}
	type item struct {
		path      string
		createdAt int64
	}
	items := []item{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(root, entry.Name())
		manifest, err := ReadManifest(path)
		if err != nil {
			continue
		}
		items = append(items, item{path: path, createdAt: manifest.CreatedAt.UnixNano()})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].createdAt > items[j].createdAt })
	removed := []string{}
	for _, item := range items[keep:] {
		removed = append(removed, item.path)
		if !dryRun {
			if err := os.RemoveAll(item.path); err != nil {
				return removed, fmt.Errorf("remove backup %s: %w", item.path, err)
			}
		}
	}
	return removed, nil
}
