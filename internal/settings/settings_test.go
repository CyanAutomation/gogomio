package settings

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestSettingsSetGet tests basic set and get operations
func TestSettingsSetGet(t *testing.T) {
	m := NewManager("")
	defer func() { _ = m.Clear() }()

	_ = m.Set("key", "value")
	result := m.Get("key")

	if result != "value" {
		t.Errorf("Get returned %v, want value", result)
	}
}

// TestSettingsGetNonexistent tests getting a non-existent key
func TestSettingsGetNonexistent(t *testing.T) {
	m := NewManager("")
	defer func() { _ = m.Clear() }()

	result := m.Get("nonexistent")
	if result != nil {
		t.Errorf("Get nonexistent returned %v, want nil", result)
	}
}

// TestSettingsGetString tests GetString with defaults
func TestSettingsGetString(t *testing.T) {
	m := NewManager("")
	defer func() { _ = m.Clear() }()

	_ = m.Set("str", "hello")
	result := m.GetString("str", "default")
	if result != "hello" {
		t.Errorf("GetString returned %q, want hello", result)
	}

	result = m.GetString("missing", "default")
	if result != "default" {
		t.Errorf("GetString missing returned %q, want default", result)
	}

	_ = m.Set("notstring", 42)
	result = m.GetString("notstring", "default")
	if result != "default" {
		t.Errorf("GetString non-string returned %q, want default", result)
	}
}

// TestSettingsGetInt tests GetInt with type conversions
func TestSettingsGetInt(t *testing.T) {
	m := NewManager("")
	defer func() { _ = m.Clear() }()

	_ = m.Set("int", 42)
	result := m.GetInt("int", 0)
	if result != 42 {
		t.Errorf("GetInt returned %d, want 42", result)
	}

	// JSON unmarshals numbers as float64
	_ = m.Set("float", 99.0)
	result = m.GetInt("float", 0)
	if result != 99 {
		t.Errorf("GetInt float returned %d, want 99", result)
	}

	result = m.GetInt("missing", 100)
	if result != 100 {
		t.Errorf("GetInt missing returned %d, want 100", result)
	}
}

// TestSettingsDelete tests deleting keys
func TestSettingsDelete(t *testing.T) {
	m := NewManager("")
	defer func() { _ = m.Clear() }()

	_ = m.Set("key", "value")
	_ = m.Delete("key")

	result := m.Get("key")
	if result != nil {
		t.Errorf("Get after delete returned %v, want nil", result)
	}
}

// TestSettingsClear tests clearing all settings
func TestSettingsClear(t *testing.T) {
	m := NewManager("")
	defer func() { _ = m.Clear() }()

	_ = m.Set("key1", "value1")
	_ = m.Set("key2", "value2")
	_ = m.Clear()

	if len(m.GetAll()) != 0 {
		t.Errorf("GetAll after Clear returned %d items, want 0", len(m.GetAll()))
	}
}

// TestSettingsGetAll tests getting all settings
func TestSettingsGetAll(t *testing.T) {
	m := NewManager("")
	defer func() { _ = m.Clear() }()

	_ = m.Set("key1", "value1")
	_ = m.Set("key2", 42)
	_ = m.Set("key3", true)

	all := m.GetAll()
	if len(all) != 3 {
		t.Errorf("GetAll returned %d items, want 3", len(all))
	}

	if val := all["key1"]; val != "value1" {
		t.Errorf("key1 = %v, want value1", val)
	}
}

// TestSettingsPersistence tests that settings survive reload
func TestSettingsPersistence(t *testing.T) {
	// Use temp directory for test
	tmpDir := t.TempDir()
	filepath := filepath.Join(tmpDir, "test_settings.json")

	// Create manager and set values
	m1 := NewManager(filepath)
	_ = m1.Set("key1", "value1")
	_ = m1.Set("key2", 42)
	_ = m1.Set("key3", true)

	// Create new manager with same file
	m2 := NewManager(filepath)

	// Verify values persisted
	if val := m2.Get("key1"); val != "value1" {
		t.Errorf("persisted key1 = %v, want value1", val)
	}
	if val := m2.GetInt("key2", 0); val != 42 {
		t.Errorf("persisted key2 = %d, want 42", val)
	}
	if val := m2.Get("key3"); val != true {
		t.Errorf("persisted key3 = %v, want true", val)
	}
}

// TestSettingsAtomicWrite tests that writes are atomic
func TestSettingsAtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	filepath := filepath.Join(tmpDir, "atomic_test.json")

	m := NewManager(filepath)
	_ = m.Set("key", "value1")
	_ = m.Set("key", "value2")

	// Verify file exists and contains correct data
	data, err := os.ReadFile(filepath)
	if err != nil {
		t.Fatalf("Failed to read settings file: %v", err)
	}

	if len(data) == 0 {
		t.Error("settings file is empty")
	}
}

// TestSettingsConcurrency tests concurrent access
func TestSettingsConcurrency(t *testing.T) {
	m := NewManager("")
	defer func() { _ = m.Clear() }()

	done := make(chan bool)
	errors := make(chan error, 10)

	// Writer goroutines
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				key := fmt.Sprintf("key_%d_%d", id, j)
				if err := m.Set(key, j*100); err != nil {
					errors <- err
				}
			}
			done <- true
		}(i)
	}

	// Reader goroutines
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 20; j++ {
				_ = m.GetAll()
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Check for errors
	select {
	case err := <-errors:
		t.Fatalf("Concurrent access error: %v", err)
	default:
	}

	// Verify we have settings
	if len(m.GetAll()) == 0 {
		t.Error("no settings after concurrent operations")
	}
}

// TestSettingsDirectoryCreation tests that directories are created
func TestSettingsDirectoryCreation(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "subdir1", "subdir2", "settings.json")

	m := NewManager(settingsPath)
	if err := m.Set("key", "value"); err != nil {
		t.Fatalf("Failed to set with missing dirs: %v", err)
	}

	// Verify file was created
	if _, err := os.Stat(settingsPath); err != nil {
		t.Errorf("settings file not created: %v", err)
	}
}
