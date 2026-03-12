package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/wellspring-cli/wellspring/internal/config"
)

const (
	// registryBaseURL is the base URL for the remote source registry.
	registryBaseURL = "https://raw.githubusercontent.com/wellspring-cli/registry/main"
	// catalogURL is the full URL to the catalog JSON file.
	catalogURL = registryBaseURL + "/catalog.json"
	// syncInterval is how long before a forced sync is required.
	syncInterval = 24 * time.Hour
	// refreshAfter is how long before a background refresh is triggered.
	refreshAfter = 12 * time.Hour
)

// SyncStatus tracks the state of registry synchronization.
type SyncStatus struct {
	LastSync  time.Time `json:"last_sync"`
	ETag      string    `json:"etag"`
	CacheDir  string    `json:"-"`
}

// NeedsSync returns true if the registry should be synced.
func (s *SyncStatus) NeedsSync() bool {
	return time.Since(s.LastSync) > syncInterval
}

// NeedsBackgroundRefresh returns true if a background refresh should be kicked off.
func (s *SyncStatus) NeedsBackgroundRefresh() bool {
	return time.Since(s.LastSync) > refreshAfter
}

// LoadSyncStatus loads the sync status from the cache directory.
func LoadSyncStatus() *SyncStatus {
	cacheDir := config.DefaultCacheDir()
	status := &SyncStatus{CacheDir: cacheDir}

	// Read last sync time.
	if data, err := os.ReadFile(filepath.Join(cacheDir, "registry.updated_at")); err == nil {
		if t, err := time.Parse(time.RFC3339, string(data)); err == nil {
			status.LastSync = t
		}
	}

	// Read ETag.
	if data, err := os.ReadFile(filepath.Join(cacheDir, "registry.etag")); err == nil {
		status.ETag = string(data)
	}

	return status
}

// SaveSyncStatus saves the sync status to the cache directory.
func (s *SyncStatus) Save() error {
	cacheDir := config.DefaultCacheDir()
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}

	if err := os.WriteFile(
		filepath.Join(cacheDir, "registry.updated_at"),
		[]byte(s.LastSync.Format(time.RFC3339)),
		0o644,
	); err != nil {
		return err
	}

	if s.ETag != "" {
		if err := os.WriteFile(
			filepath.Join(cacheDir, "registry.etag"),
			[]byte(s.ETag),
			0o644,
		); err != nil {
			return err
		}
	}

	return nil
}

// SyncCatalog fetches the latest catalog from the remote registry.
// It uses ETag-based caching to minimize bandwidth.
func SyncCatalog(reg *Registry, debug bool) error {
	status := LoadSyncStatus()

	client := &http.Client{Timeout: 10 * time.Second}
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, "GET", catalogURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("User-Agent", "wellspring-cli/0.1")
	if status.ETag != "" {
		req.Header.Set("If-None-Match", status.ETag)
	}

	if debug {
		fmt.Fprintf(os.Stderr, "[debug] syncing catalog from %s\n", catalogURL)
		if status.ETag != "" {
			fmt.Fprintf(os.Stderr, "[debug] using ETag: %s\n", status.ETag)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("fetching catalog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		if debug {
			fmt.Fprintln(os.Stderr, "[debug] catalog not modified (304)")
		}
		status.LastSync = time.Now()
		return status.Save()
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading catalog: %w", err)
	}

	var entries []CatalogEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return fmt.Errorf("parsing catalog: %w", err)
	}

	// Save catalog.
	if err := reg.SaveCatalog(entries); err != nil {
		return fmt.Errorf("saving catalog: %w", err)
	}

	// Update registry.
	reg.SetCatalog(entries)

	// Save sync status.
	status.LastSync = time.Now()
	if etag := resp.Header.Get("ETag"); etag != "" {
		status.ETag = etag
	}

	if debug {
		fmt.Fprintf(os.Stderr, "[debug] catalog synced: %d entries\n", len(entries))
	}

	return status.Save()
}

// BackgroundSync starts a non-blocking sync in a goroutine.
func BackgroundSync(reg *Registry, debug bool) {
	go func() {
		if err := SyncCatalog(reg, debug); err != nil {
			if debug {
				fmt.Fprintf(os.Stderr, "[debug] background sync failed: %v\n", err)
			}
		}
	}()
}

// ForceSync forces an immediate catalog sync regardless of cache state.
func ForceSync(reg *Registry, debug bool) error {
	// Clear ETag to force a full download.
	cacheDir := config.DefaultCacheDir()
	etagPath := filepath.Join(cacheDir, "registry.etag")
	if err := os.Remove(etagPath); err != nil && !os.IsNotExist(err) {
		if debug {
			fmt.Fprintf(os.Stderr, "[debug] failed to remove etag file: %v\n", err)
		}
	}
	return SyncCatalog(reg, debug)
}
