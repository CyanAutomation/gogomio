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

// SetMany stores multiple key-value pairs and persists them as a single atomic batch.
// In-memory data is only updated after a successful persist.
func (m *Manager) SetMany(values map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.data == nil {
		m.data = make(map[string]interface{})
	}

	updated := make(map[string]interface{}, len(m.data)+len(values))
	for key, value := range m.data {
		updated[key] = value
	}
	for key, value := range values {
		updated[key] = value
	}

	if err := m.persistData(updated); err != nil {
		return fmt.Errorf("batch persist failed: %w", err)
	}

	m.data = updated
	return nil
}

// Get retrieves a value by key. Returns nil if key doesn't exist.
func (m *Manager) Get(key string) interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.data[key]
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

	// Return a copy to prevent external modification
	copy := make(map[string]interface{})
	for k, v := range m.data {
		copy[k] = v
	}
	return copy
}

// Delete removes a key from settings.
func (m *Manager) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, key)
	return m.persist()
}

// Clear removes all settings.
func (m *Manager) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data = make(map[string]interface{})
	return m.persist()
}

// persist writes current settings to disk atomically with backup.
func (m *Manager) persist() error {
	return m.persistData(m.data)
}

// persistData writes the provided settings map to disk atomically with backup.
func (m *Manager) persistData(settings map[string]interface{}) error {
	// Create directory if needed
	dir := filepath.Dir(m.filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("❌ Settings: failed to create settings directory: %v", err)
			return fmt.Errorf("failed to create settings directory: %w", err)
		}
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		log.Printf("❌ Settings: failed to marshal settings: %v", err)
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	// Create backup of existing file before writing new one
	if fileInfo, err := os.Stat(m.filePath); err == nil && fileInfo.Size() > 0 {
		backupFile := m.filePath + ".bak"
		if err := m.copyFile(m.filePath, backupFile); err != nil {
			log.Printf("⚠️  Settings: could not create backup, proceeding anyway: %v", err)
			// Don't fail here - backup is nice-to-have, not critical
		}
	}

	// Atomic write: write to temp file then rename
	tempFile := m.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		log.Printf("❌ Settings: failed to write temp settings file: %v", err)
		return fmt.Errorf("failed to write temp settings file: %w", err)
	}

	if err := os.Rename(tempFile, m.filePath); err != nil {
		// Cleanup temp file
		_ = os.Remove(tempFile)
		log.Printf("❌ Settings: failed to rename settings file: %v", err)
		return fmt.Errorf("failed to rename settings file: %w", err)
	}

	log.Printf("✓ Settings: persisted %d settings", len(settings))
	return nil
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
				m.data = loaded
				// Attempt to restore backup over corrupted file
				if restoreErr := os.WriteFile(m.filePath, backupData, 0644); restoreErr != nil {
					log.Printf("⚠️  Settings: could not restore from backup: %v", restoreErr)
				} else {
					log.Printf("✓ Settings: restored from backup")
				}
				return nil
			}
		}

		// No backup available or backup also corrupted
		log.Printf("❌ Settings: backup also corrupted or unavailable, starting with clean state")
		loaded = make(map[string]interface{})

		// Move corrupted file to timestamped archive
		archiveFile := m.filePath + ".corrupted." + time.Now().Format("20060102_150405")
		if err := os.Rename(m.filePath, archiveFile); err != nil {
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

	m.data = loaded
	return nil
}

// copyFile is a helper to copy source file to destination.
func (m *Manager) copyFile(src, dst string) error {
	srcData, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, srcData, 0644)
}
