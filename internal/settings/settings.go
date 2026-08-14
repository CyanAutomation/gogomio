package settings

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Manager handles persistent settings storage.
type Manager struct {
	mu       sync.RWMutex
	filePath string
	data     map[string]interface{}
}

// NewManager creates a new settings manager with optional file path.
// If filepath is empty, a default location is used.
func NewManager(filepath string) *Manager {
	if filepath == "" {
		// Default: /tmp/gogomio/ directory
		filepath = "/tmp/gogomio/settings.json"
	}

	m := &Manager{
		filePath: filepath,
		data:     make(map[string]interface{}),
	}

	// Load existing settings if file exists
	_ = m.load()

	return m
}

// Set stores a key-value pair in memory and persists to file.
func (m *Manager) Set(key string, value interface{}) error {
	return m.SetMany(map[string]interface{}{key: value})
}

// SetMany merges multiple key-value pairs into the latest settings on disk and
// persists them as a single atomic batch. In-memory data is only updated after
// a successful persist.
func (m *Manager) SetMany(values map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	updated, err := m.mutateSettings(func(latest map[string]interface{}) {
		for key, value := range values {
			latest[key] = value
		}
	})
	if err != nil {
		return fmt.Errorf("batch persist failed: %w", err)
	}

	m.data = updated
	return nil
}

// Get retrieves a value by key. Returns nil if key doesn't exist.
func (m *Manager) Get(key string) interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return cloneJSONLikeValue(m.data[key])
}

// GetString retrieves a string value by key with default fallback.
func (m *Manager) GetString(key string, defaultValue string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	val, ok := m.data[key]
	if !ok {
		return defaultValue
	}

	str, ok := val.(string)
	if !ok {
		return defaultValue
	}

	return str
}

// GetInt retrieves an int value by key with default fallback.
func (m *Manager) GetInt(key string, defaultValue int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	val, ok := m.data[key]
	if !ok {
		return defaultValue
	}

	// Handle both float64 (JSON number unmarshals as float64) and int
	switch v := val.(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return defaultValue
	}
}

// GetAll returns a copy of all settings.
func (m *Manager) GetAll() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a deep copy to prevent external modification.
	copy := make(map[string]interface{})
	for k, v := range m.data {
		copy[k] = cloneJSONLikeValue(v)
	}
	return copy
}

// cloneJSONLikeValue deep-copies supported JSON-like values.
// Supported types: map[string]interface{}, []interface{}, and primitives.
func cloneJSONLikeValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		cloned := make(map[string]interface{}, len(typed))
		for key, val := range typed {
			cloned[key] = cloneJSONLikeValue(val)
		}
		return cloned
	case []interface{}:
		cloned := make([]interface{}, len(typed))
		for i, val := range typed {
			cloned[i] = cloneJSONLikeValue(val)
		}
		return cloned
	case nil, bool, string, float64, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return typed
	default:
		// Fallback for unsupported types: preserve existing behavior by returning as-is.
		return typed
	}
}

// Delete removes only the requested key from the latest settings on disk.
func (m *Manager) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	updated, err := m.mutateSettings(func(latest map[string]interface{}) {
		delete(latest, key)
	})
	if err != nil {
		return err
	}

	m.data = updated
	return nil
}

// Clear intentionally replaces all settings, including keys written by other
// managers since this manager last loaded the file.
func (m *Manager) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	updated, err := m.mutateSettings(func(latest map[string]interface{}) {
		for key := range latest {
			delete(latest, key)
		}
	})
	if err != nil {
		return err
	}

	m.data = updated
	return nil
}

// mutateSettings performs a complete read-modify-write transaction while the
// cross-process settings lock is held. The returned map is the exact state
// committed to disk.
func (m *Manager) mutateSettings(mutate func(map[string]interface{})) (map[string]interface{}, error) {
	if err := m.ensureSettingsDir(); err != nil {
		return nil, err
	}

	var updated map[string]interface{}
	err := m.withSettingsLock(func() error {
		latest := make(map[string]interface{})
		data, err := os.ReadFile(m.filePath)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to read settings file: %w", err)
		}
		if err == nil {
			if err := json.Unmarshal(data, &latest); err != nil {
				return fmt.Errorf("failed to decode settings file: %w", err)
			}
			if latest == nil {
				latest = make(map[string]interface{})
			}
		}

		mutate(latest)
		encoded, err := json.MarshalIndent(latest, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal settings: %w", err)
		}
		if err := m.writeSettingsUnlocked(encoded); err != nil {
			return err
		}
		updated = latest
		return nil
	})
	if err != nil {
		return nil, err
	}

	log.Printf("✓ Settings: persisted %d settings", len(updated))
	return updated, nil
}

// persist writes current settings to disk atomically with backup.
func (m *Manager) persist() error {
	return m.persistData(m.data)
}

// persistData writes the provided settings map to disk atomically with backup.
//
// Multi-process behavior:
//   - An advisory exclusive file lock is taken on "<settings>.lock" while backing up,
//     writing, and renaming the new settings file.
//   - Processes that do not honor this advisory lock may still race with writes.
func (m *Manager) persistData(settings map[string]interface{}) error {
	if err := m.ensureSettingsDir(); err != nil {
		return err
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		log.Printf("❌ Settings: failed to marshal settings: %v", err)
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	if err := m.withSettingsLock(func() error {
		return m.writeSettingsUnlocked(data)
	}); err != nil {
		return err
	}

	log.Printf("✓ Settings: persisted %d settings", len(settings))
	return nil
}

func (m *Manager) ensureSettingsDir() error {
	dir := filepath.Dir(m.filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("❌ Settings: failed to create settings directory: %v", err)
			return fmt.Errorf("failed to create settings directory: %w", err)
		}
	}
	return nil
}

// writeSettingsUnlocked backs up and atomically replaces the settings file.
// The caller must hold the settings file lock.
func (m *Manager) writeSettingsUnlocked(data []byte) error {
	dir := filepath.Dir(m.filePath)
	// Create backup of existing file before writing new one
	if fileInfo, err := os.Stat(m.filePath); err == nil && fileInfo.Size() > 0 {
		backupFile := m.filePath + ".bak"
		if err := m.copyFile(m.filePath, backupFile); err != nil {
			log.Printf("⚠️  Settings: could not create backup, proceeding anyway: %v", err)
			// Don't fail here - backup is nice-to-have, not critical
		}
	}

	// Atomic write: write to uniquely named temp file in same directory then rename.
	tmp, err := os.CreateTemp(dir, filepath.Base(m.filePath)+".*.tmp")
	if err != nil {
		log.Printf("❌ Settings: failed to create temp settings file: %v", err)
		return fmt.Errorf("failed to create temp settings file: %w", err)
	}
	tempFile := tmp.Name()
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempFile)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		log.Printf("❌ Settings: failed to write temp settings file: %v", err)
		return fmt.Errorf("failed to write temp settings file: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		log.Printf("❌ Settings: failed to sync temp settings file: %v", err)
		return fmt.Errorf("failed to sync temp settings file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		log.Printf("❌ Settings: failed to close temp settings file: %v", err)
		return fmt.Errorf("failed to close temp settings file: %w", err)
	}

	if err := os.Rename(tempFile, m.filePath); err != nil {
		log.Printf("❌ Settings: failed to rename settings file: %v", err)
		return fmt.Errorf("failed to rename settings file: %w", err)
	}

	cleanupTemp = false
	return nil
}

func (m *Manager) withSettingsLock(fn func() error) error {
	lockPath := m.filePath + ".lock"
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		log.Printf("❌ Settings: failed to open lock file: %v", err)
		return fmt.Errorf("failed to open lock file: %w", err)
	}

	if err := lockFileExclusive(lockFile); err != nil {
		_ = lockFile.Close()
		log.Printf("❌ Settings: failed to lock settings file: %v", err)
		return fmt.Errorf("failed to lock settings file: %w", err)
	}
	defer func() {
		if err := unlockFileExclusive(lockFile); err != nil {
			log.Printf("⚠️  Settings: failed to unlock settings file: %v", err)
		}
		if err := lockFile.Close(); err != nil {
			log.Printf("⚠️  Settings: failed to close lock file: %v", err)
		}
		// Keep the lock file on disk so all processes continue to lock the same inode.
		// Deleting and recreating lockfiles can split lock domains across processes.
	}()

	return fn()
}

// load reads settings from disk with error recovery.
func (m *Manager) load() error {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, that's ok
			log.Printf("ℹ️  Settings: no existing settings file at %s (will be created on first save)", m.filePath)
			return nil
		}
		log.Printf("❌ Settings: failed to read settings file: %v", err)
		return fmt.Errorf("failed to read settings file: %w", err)
	}

	var loaded map[string]interface{}
	if err := json.Unmarshal(data, &loaded); err != nil {
		log.Printf("❌ Settings: corrupted JSON in settings file, attempting backup recovery...")

		// Try to recover from backup file
		backupFile := m.filePath + ".bak"
		backupData, backupErr := os.ReadFile(backupFile)
		if backupErr == nil {
			if backupErr := json.Unmarshal(backupData, &loaded); backupErr == nil {
				log.Printf("✓ Settings: recovered from backup file")
				// Attempt to restore backup over corrupted file
				if restoreErr := m.withSettingsLock(func() error {
					return os.WriteFile(m.filePath, backupData, 0600)
				}); restoreErr != nil {
					log.Printf("⚠️  Settings: could not restore from backup: %v", restoreErr)
				} else {
					log.Printf("✓ Settings: restored from backup")
				}
				m.mu.Lock()
				m.data = loaded
				m.mu.Unlock()
				return nil
			}
		}

		// No backup available or backup also corrupted
		log.Printf("❌ Settings: backup also corrupted or unavailable, starting with clean state")
		loaded = make(map[string]interface{})

		// Move corrupted file to timestamped archive
		archiveFile := m.filePath + ".corrupted." + time.Now().Format("20060102_150405")
		if err := m.withSettingsLock(func() error {
			return os.Rename(m.filePath, archiveFile)
		}); err != nil {
			log.Printf("⚠️  Settings: could not archive corrupted file: %v", err)
		} else {
			log.Printf("✓ Settings: archived corrupted file to %s", archiveFile)
		}
	} else {
		log.Printf("✓ Settings: loaded from file (%d entries)", len(loaded))
	}

	if loaded == nil {
		loaded = make(map[string]interface{})
	}

	m.mu.Lock()
	m.data = loaded
	m.mu.Unlock()
	return nil
}

// copyFile is a helper to copy source file to destination.
func (m *Manager) copyFile(src, dst string) error {
	srcData, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, srcData, 0600)
}
