package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// TestSettingsGetReturnsStoredValueAndNilForMissingKey verifies Get's value
// and missing-key behavior using a single populated manager.
func TestSettingsGetReturnsStoredValueAndNilForMissingKey(t *testing.T) {
	m := NewManager(filepath.Join(t.TempDir(), "settings.json"))
	if err := m.Set("key", "value"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}
	result := m.Get("key")

	if result != "value" {
		t.Fatalf("Get returned %v, want value", result)
	}
	result = m.Get("nonexistent")
	if result != nil {
		t.Fatalf("Get nonexistent returned %v, want nil", result)
	}
}

// TestSettingsGetString tests GetString with defaults
func TestSettingsGetString(t *testing.T) {
	m := NewManager(filepath.Join(t.TempDir(), "settings.json"))

	if err := m.Set("str", "hello"); err != nil {
		t.Fatalf("Set string returned error: %v", err)
	}
	result := m.GetString("str", "default")
	if result != "hello" {
		t.Errorf("GetString returned %q, want hello", result)
	}

	result = m.GetString("missing", "default")
	if result != "default" {
		t.Errorf("GetString missing returned %q, want default", result)
	}

	if err := m.Set("notstring", 42); err != nil {
		t.Fatalf("Set non-string returned error: %v", err)
	}
	result = m.GetString("notstring", "default")
	if result != "default" {
		t.Errorf("GetString non-string returned %q, want default", result)
	}
}

// TestSettingsGetInt tests GetInt with type conversions
func TestSettingsGetInt(t *testing.T) {
	m := NewManager(filepath.Join(t.TempDir(), "settings.json"))

	if err := m.Set("int", 42); err != nil {
		t.Fatalf("Set int returned error: %v", err)
	}
	result := m.GetInt("int", 0)
	if result != 42 {
		t.Errorf("GetInt returned %d, want 42", result)
	}

	// JSON unmarshals numbers as float64
	if err := m.Set("float", 99.0); err != nil {
		t.Fatalf("Set float returned error: %v", err)
	}
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
	m := NewManager(filepath.Join(t.TempDir(), "settings.json"))
	if err := m.SetMany(map[string]interface{}{"key1": "value1", "key2": "value2"}); err != nil {
		t.Fatalf("SetMany returned error: %v", err)
	}
	if err := m.Clear(); err != nil {
		t.Fatalf("Clear returned error: %v", err)
	}

	if got := m.GetAll(); len(got) != 0 {
		t.Fatalf("GetAll after Clear returned %v, want empty settings", got)
	}
}

// TestSettingsGetAll tests getting all settings
func TestSettingsGetAll(t *testing.T) {
	m := NewManager(filepath.Join(t.TempDir(), "settings.json"))
	want := map[string]interface{}{"key1": "value1", "key2": 42, "key3": true}
	if err := m.SetMany(want); err != nil {
		t.Fatalf("SetMany returned error: %v", err)
	}

	all := m.GetAll()
	if !reflect.DeepEqual(all, want) {
		t.Fatalf("GetAll() = %#v, want %#v", all, want)
	}
}

func TestSettingsGetReturnsClonedValue(t *testing.T) {
	m := NewManager("")
	defer func() { _ = m.Clear() }()

	original := map[string]interface{}{
		"nested": map[string]interface{}{
			"flag": true,
		},
		"items": []interface{}{map[string]interface{}{"name": "a"}},
	}
	_ = m.Set("payload", original)

	got, ok := m.Get("payload").(map[string]interface{})
	if !ok {
		t.Fatalf("Get returned unexpected type: %T", m.Get("payload"))
	}

	got["newKey"] = "mutated"
	gotNested := got["nested"].(map[string]interface{})
	gotNested["flag"] = false
	gotItems := got["items"].([]interface{})
	gotItems[0].(map[string]interface{})["name"] = "changed"

	current := m.Get("payload").(map[string]interface{})
	if _, exists := current["newKey"]; exists {
		t.Fatalf("Get mutation leaked into manager state")
	}
	if current["nested"].(map[string]interface{})["flag"] != true {
		t.Fatalf("nested Get mutation leaked into manager state")
	}
	if current["items"].([]interface{})[0].(map[string]interface{})["name"] != "a" {
		t.Fatalf("slice element mutation leaked into manager state")
	}
}

func TestSettingsGetAllReturnsDeepClonedValues(t *testing.T) {
	m := NewManager("")
	defer func() { _ = m.Clear() }()

	_ = m.SetMany(map[string]interface{}{
		"config": map[string]interface{}{
			"threshold": float64(10),
		},
		"list": []interface{}{
			map[string]interface{}{"id": "x"},
		},
	})

	all := m.GetAll()
	all["extra"] = "mutated"
	all["config"].(map[string]interface{})["threshold"] = float64(99)
	all["list"].([]interface{})[0].(map[string]interface{})["id"] = "changed"

	current := m.GetAll()
	if _, exists := current["extra"]; exists {
		t.Fatalf("GetAll top-level mutation leaked into manager state")
	}
	if current["config"].(map[string]interface{})["threshold"] != float64(10) {
		t.Fatalf("GetAll nested map mutation leaked into manager state")
	}
	if current["list"].([]interface{})[0].(map[string]interface{})["id"] != "x" {
		t.Fatalf("GetAll nested slice mutation leaked into manager state")
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

// TestSettingsWritesPersistValidJSONAndRemoveTemporaryFiles checks each committed value.
func TestSettingsWritesPersistValidJSONAndRemoveTemporaryFiles(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "atomic_test.json")
	tempGlob := settingsPath + ".*.tmp"
	m := NewManager(settingsPath)

	readAndValidate := func(expectedValue string) {
		t.Helper()

		data, err := os.ReadFile(settingsPath)
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

		if leftovers, err := filepath.Glob(tempGlob); err != nil {
			t.Fatalf("failed to check temp file leftovers: %v", err)
		} else if len(leftovers) != 0 {
			t.Fatalf("temp files should not exist after successful write, found=%v", leftovers)
		}
	}
}

func TestSettingsSetDoesNotUpdateMemoryWhenSettingsFileIsUnreadable(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	m := NewManager(settingsPath)
	if err := m.Set("key", "stable"); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	if err := os.Remove(settingsPath); err != nil {
		t.Fatalf("remove settings file for failure setup: %v", err)
	}
	if err := os.Mkdir(settingsPath, 0755); err != nil {
		t.Fatalf("replace settings file with a directory: %v", err)
	}

	err := m.Set("key", "should-fail")
	if err == nil || !strings.Contains(err.Error(), "failed to read settings file") {
		t.Fatalf("Set error = %v, want an unreadable-settings error", err)
	}
	if got := m.Get("key"); got != "stable" {
		t.Fatalf("in-memory setting after failed Set = %v, want stable", got)
	}
}

// TestSettingsConcurrentWritersSamePath ensures concurrent writers to the same file path
// never leave truncated/invalid JSON on disk.
// Requirement: settings-concurrency.
func TestSettingsConcurrentWritersSamePath(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "shared_settings.json")

	const writers = 8
	const writesPerWriter = 25

	// Seed the file so the disk reader can distinguish an invalid read from setup.
	if err := NewManager(settingsPath).Set("seed", "value"); err != nil {
		t.Fatalf("failed seeding settings file: %v", err)
	}

	var writerWG sync.WaitGroup
	var readerWG sync.WaitGroup
	errCh := make(chan error, writers*writesPerWriter+1)
	var stopReader int32

	readerWG.Add(1)
	go func() {
		defer readerWG.Done()
		for atomic.LoadInt32(&stopReader) == 0 {
			data, err := os.ReadFile(settingsPath)
			if err != nil {
				errCh <- fmt.Errorf("concurrent disk read failed: %w", err)
				return
			}
			if len(data) == 0 {
				errCh <- fmt.Errorf("concurrent disk read observed an empty file")
				return
			}
			var payload map[string]interface{}
			if err := json.Unmarshal(data, &payload); err != nil {
				errCh <- fmt.Errorf("concurrent disk read observed invalid JSON: %w; payload=%q", err, string(data))
				return
			}
		}
	}()

	for i := 0; i < writers; i++ {
		writerWG.Add(1)
		go func(id int) {
			defer writerWG.Done()

			m := NewManager(settingsPath)
			for j := 0; j < writesPerWriter; j++ {
				key := fmt.Sprintf("writer_%d", id)
				value := fmt.Sprintf("value_%d", j)
				if err := m.Set(key, value); err != nil {
					errCh <- fmt.Errorf("writer %d iteration %d failed: %w", id, j, err)
					return
				}
			}
		}(i)
	}

	writerWG.Wait()
	atomic.StoreInt32(&stopReader, 1)
	readerWG.Wait()
	close(errCh)

	for err := range errCh {
		t.Fatal(err)
	}

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed reading final settings file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("final settings file is empty")
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("final settings file contains invalid/truncated JSON: %v; payload=%q", err, string(data))
	}
}

// TestSettingsConcurrency tests the in-memory Manager concurrency contract.
// Requirement: settings-concurrency.
func TestSettingsConcurrency(t *testing.T) {
	m := NewManager(filepath.Join(t.TempDir(), "settings.json"))

	const writers = 5
	const writesPerWriter = 10
	const readers = 5
	const readsPerReader = 20

	var wg sync.WaitGroup
	// Every operation can report one error without blocking completion of the group.
	errors := make(chan error, writers+readers)

	// Writer goroutines
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerWriter; j++ {
				key := fmt.Sprintf("key_%d_%d", id, j)
				if err := m.Set(key, j*100); err != nil {
					errors <- fmt.Errorf("Set(%q) failed: %w", key, err)
					return
				}
			}
		}(i)
	}

	// Reader goroutines
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < readsPerReader; j++ {
				got := m.GetAll()
				for key, value := range got {
					if _, ok := value.(int); !ok {
						if _, ok := value.(float64); ok {
							continue
						}
						errors <- fmt.Errorf("reader %d got unusable entry %q=%v (%T)", id, key, value, value)
						return
					}
					_ = got[key]
				}

				aliasKey := fmt.Sprintf("reader_alias_%d_%d", id, j)
				got[aliasKey] = "must not leak"
				if _, leaked := m.GetAll()[aliasKey]; leaked {
					errors <- fmt.Errorf("reader %d mutation of GetAll result leaked into manager", id)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}

	all := m.GetAll()
	if len(all) != writers*writesPerWriter {
		t.Errorf("GetAll returned %d settings, want %d", len(all), writers*writesPerWriter)
	}
	for i := 0; i < writers; i++ {
		for j := 0; j < writesPerWriter; j++ {
			key := fmt.Sprintf("key_%d_%d", i, j)
			want := j * 100
			got, exists := all[key]
			if !exists || (got != want && got != float64(want)) {
				t.Errorf("GetAll[%q] = %v (exists=%t), want %d", key, got, exists, want)
			}
		}
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
		t.Fatal("expected SetMany to fail when the settings path is a directory")
	} else if !strings.Contains(err.Error(), "failed to read settings file") {
		t.Fatalf("expected settings read failure, got: %v", err)
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

// TestSettingsDeleteRollbackOnPersistFailure ensures delete keeps in-memory state on persist failure.
func TestSettingsDeleteRollbackOnPersistFailure(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "delete_settings.json")
	m := NewManager(settingsPath)
	if err := m.SetMany(map[string]interface{}{
		"stable": "value",
		"drop":   "me",
	}); err != nil {
		t.Fatalf("failed to seed initial state: %v", err)
	}

	beforeState := m.GetAll()

	if err := os.Remove(settingsPath); err != nil {
		t.Fatalf("failed to remove settings file for failure setup: %v", err)
	}
	if err := os.Mkdir(settingsPath, 0755); err != nil {
		t.Fatalf("failed to create directory at settings path for failure setup: %v", err)
	}

	err := m.Delete("drop")
	if err == nil {
		t.Fatal("expected Delete to fail when the settings path is a directory")
	} else if !strings.Contains(err.Error(), "failed to read settings file") {
		t.Fatalf("expected settings read failure, got: %v", err)
	}

	if gotState := m.GetAll(); !reflect.DeepEqual(gotState, beforeState) {
		t.Fatalf("in-memory settings changed after failed Delete, got=%v want=%v", gotState, beforeState)
	}
}

// TestSettingsClearRollbackOnPersistFailure ensures clear keeps in-memory state on persist failure.
func TestSettingsClearRollbackOnPersistFailure(t *testing.T) {
	tmpDir := t.TempDir()
	settingsPath := filepath.Join(tmpDir, "clear_settings.json")
	m := NewManager(settingsPath)
	if err := m.SetMany(map[string]interface{}{
		"stable": "value",
		"keep":   "this",
	}); err != nil {
		t.Fatalf("failed to seed initial state: %v", err)
	}

	beforeState := m.GetAll()

	if err := os.Remove(settingsPath); err != nil {
		t.Fatalf("failed to remove settings file for failure setup: %v", err)
	}
	if err := os.Mkdir(settingsPath, 0755); err != nil {
		t.Fatalf("failed to create directory at settings path for failure setup: %v", err)
	}

	err := m.Clear()
	if err == nil {
		t.Fatal("expected Clear to fail when the settings path is a directory")
	} else if !strings.Contains(err.Error(), "failed to read settings file") {
		t.Fatalf("expected settings read failure, got: %v", err)
	}

	if gotState := m.GetAll(); !reflect.DeepEqual(gotState, beforeState) {
		t.Fatalf("in-memory settings changed after failed Clear, got=%v want=%v", gotState, beforeState)
	}
}
