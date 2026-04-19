package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
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
	tempPath := filepath + ".tmp"

	m := NewManager(filepath)

	readAndValidate := func(expectedValue string) {
		t.Helper()

		data, err := os.ReadFile(filepath)
		if err != nil {
			t.Fatalf("Failed to read settings file: %v", err)
		}
		if len(data) == 0 {
			t.Fatal("settings file is empty")
		}

		var payload map[string]interface{}
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatalf("settings file contains invalid JSON: %v; payload=%q", err, string(data))
		}

		if got, ok := payload["key"].(string); !ok || got != expectedValue {
			t.Fatalf("settings file has key=%v, want %q", payload["key"], expectedValue)
		}
	}

	// Repeated writes should always leave valid JSON and no dangling temp file.
	for i := 0; i < 20; i++ {
		value := fmt.Sprintf("value-%d", i)
		if err := m.Set("key", value); err != nil {
			t.Fatalf("Set failed for %q: %v", value, err)
		}
		readAndValidate(value)

		if _, err := os.Stat(tempPath); !os.IsNotExist(err) {
			t.Fatalf("temp file should not exist after successful write, stat err=%v", err)
		}
	}

	// Concurrent reader/writer stress: no partial/truncated JSON should ever be observable.
	var stop int32
	readerErr := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for atomic.LoadInt32(&stop) == 0 {
			data, err := os.ReadFile(filepath)
			if err != nil {
				readerErr <- fmt.Errorf("read failed: %w", err)
				return
			}
			if len(data) == 0 {
				readerErr <- fmt.Errorf("observed empty payload")
				return
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(data, &payload); err != nil {
				readerErr <- fmt.Errorf("observed invalid/truncated JSON: %w; payload=%q", err, string(data))
				return
			}
		}
	}()

	for i := 20; i < 120; i++ {
		value := fmt.Sprintf("value-%d", i)
		if err := m.Set("key", value); err != nil {
			atomic.StoreInt32(&stop, 1)
			wg.Wait()
			t.Fatalf("Set failed during stress for %q: %v", value, err)
		}
	}
	atomic.StoreInt32(&stop, 1)
	wg.Wait()

	select {
	case err := <-readerErr:
		t.Fatal(err)
	default:
	}

	// Interrupted write path: make final destination path non-renamable and ensure old file remains valid.
	if err := m.Set("key", "stable-before-failure"); err != nil {
		t.Fatalf("failed to set stable state: %v", err)
	}
	beforeFailureData, err := os.ReadFile(filepath)
	if err != nil {
		t.Fatalf("failed to read baseline file: %v", err)
	}
	if err := os.Remove(filepath); err != nil {
		t.Fatalf("failed to remove settings file for fault setup: %v", err)
	}
	if err := os.Mkdir(filepath, 0755); err != nil {
		t.Fatalf("failed to create directory at settings path for fault setup: %v", err)
	}

	err = m.Set("key", "should-fail")
	if err == nil {
		t.Fatal("expected Set to fail when rename destination is a directory")
	}

	// Restore original file and verify it is still valid/unchanged from before the interrupted path.
	if err := os.Remove(filepath); err != nil {
		t.Fatalf("failed to remove fault directory: %v", err)
	}
	if err := os.WriteFile(filepath, beforeFailureData, 0644); err != nil {
		t.Fatalf("failed to restore baseline file: %v", err)
	}
	readAndValidate("stable-before-failure")

	// Failed path should also clean up temp file.
	if _, statErr := os.Stat(tempPath); !os.IsNotExist(statErr) {
		t.Fatalf("temp file should be cleaned up after failed rename, stat err=%v", statErr)
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

// TestSettingsSetAfterNullFile tests setting values after loading a JSON null payload.
func TestSettingsSetAfterNullFile(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "null_settings.json")

	if err := os.WriteFile(settingsPath, []byte("null"), 0644); err != nil {
		t.Fatalf("failed to write null settings file: %v", err)
	}

	m := NewManager(settingsPath)

	if err := m.Set("key", "value"); err != nil {
		t.Fatalf("Set returned error after loading null file: %v", err)
	}

	if got := m.Get("key"); got != "value" {
		t.Fatalf("Get returned %v, want value", got)
	}
}

// TestSettingsSetManyRollbackOnPersistFailure ensures batch updates are all-or-nothing.
func TestSettingsSetManyRollbackOnPersistFailure(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "batch_settings.json")

	m := NewManager(settingsPath)
	if err := m.Set("stable", "value"); err != nil {
		t.Fatalf("failed to seed initial state: %v", err)
	}

	beforeData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read baseline settings file: %v", err)
	}
	beforeState := m.GetAll()

	if err := os.Remove(settingsPath); err != nil {
		t.Fatalf("failed to remove settings file for failure setup: %v", err)
	}
	if err := os.Mkdir(settingsPath, 0755); err != nil {
		t.Fatalf("failed to create directory at settings path for failure setup: %v", err)
	}

	err = m.SetMany(map[string]interface{}{
		"new_a": "A",
		"new_b": "B",
	})
	if err == nil {
		t.Fatal("expected SetMany to fail when rename destination is a directory")
	}

	if gotState := m.GetAll(); !reflect.DeepEqual(gotState, beforeState) {
		t.Fatalf("in-memory settings changed after failed SetMany, got=%v want=%v", gotState, beforeState)
	}

	if err := os.Remove(settingsPath); err != nil {
		t.Fatalf("failed to remove fault directory: %v", err)
	}
	if err := os.WriteFile(settingsPath, beforeData, 0644); err != nil {
		t.Fatalf("failed to restore baseline file: %v", err)
	}

	afterData, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read restored settings file: %v", err)
	}
	if string(afterData) != string(beforeData) {
		t.Fatalf("settings file changed unexpectedly after failed SetMany")
	}
}
