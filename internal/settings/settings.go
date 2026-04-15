package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.data == nil {
		m.data = make(map[string]interface{})
	}

	m.data[key] = value
	return m.persist()
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

// persist writes current settings to disk atomically.
func (m *Manager) persist() error {
	// Create directory if needed
	dir := filepath.Dir(m.filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create settings directory: %w", err)
		}
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(m.data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings: %w", err)
	}

	// Atomic write: write to temp file then rename
	tempFile := m.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp settings file: %w", err)
	}

	if err := os.Rename(tempFile, m.filePath); err != nil {
		// Cleanup temp file
		_ = os.Remove(tempFile)
		return fmt.Errorf("failed to rename settings file: %w", err)
	}

	return nil
}

// load reads settings from disk.
func (m *Manager) load() error {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, that's ok
			return nil
		}
		return fmt.Errorf("failed to read settings file: %w", err)
	}

	var loaded map[string]interface{}
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("failed to unmarshal settings: %w", err)
	}

	if loaded == nil {
		loaded = make(map[string]interface{})
	}

	m.data = loaded
	return nil
}
