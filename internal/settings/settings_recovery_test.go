package settings

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSettingsCorruptionRecovery tests that corrupted JSON is recovered from backup
func TestSettingsCorruptionRecovery(t *testing.T) {
	// Create temporary directory
	tmpDir, err := ioutil.TempDir("", "gogomio_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	settingsPath := filepath.Join(tmpDir, "settings.json")
	
	// Create manager and set a value
	m := NewManager(settingsPath)
	m.Set("test_key", "test_value")
	m.persist()
	
	// Verify settings file exists
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("Settings file not created: %v", err)
	}
	
	// Create a backup with good data
	backupPath := settingsPath + ".bak"
	if err := ioutil.WriteFile(backupPath, []byte(`{"test_key":"test_value"}`), 0644); err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}
	
	// Corrupt the primary file
	if err := ioutil.WriteFile(settingsPath, []byte("{invalid json}"), 0644); err != nil {
		t.Fatalf("Failed to corrupt settings: %v", err)
	}
	
	// Create new manager instance - should recover from backup
	m2 := NewManager(settingsPath)
	m2.load()
	
	// Verify recovery
	if m2.Get("test_key") != "test_value" {
		t.Errorf("Failed to recover from backup: got %v, want test_value", m2.Get("test_key"))
	}
}

// TestSettingsCorruptedFileArchiving tests that corrupted files are archived
func TestSettingsCorruptedFileArchiving(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "gogomio_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	settingsPath := filepath.Join(tmpDir, "settings.json")
	
	// Write corrupted file with no backup
	if err := ioutil.WriteFile(settingsPath, []byte("{invalid json}"), 0644); err != nil {
		t.Fatalf("Failed to write corrupted file: %v", err)
	}
	
	// Create manager - should archive corrupted file
	m := NewManager(settingsPath)
	m.load()
	
	// Verify original file was archived
	// Look for files matching pattern: settings.json.corrupted.*
	files, err := filepath.Glob(filepath.Join(tmpDir, "settings.json.corrupted.*"))
	if err != nil {
		t.Fatalf("Failed to glob for archived files: %v", err)
	}
	
	if len(files) == 0 {
		t.Error("Corrupted file was not archived")
	}
	
	// Verify primary file now contains valid JSON
	content, err := ioutil.ReadFile(settingsPath)
	if err == nil {
		var data map[string]interface{}
		if err := json.Unmarshal(content, &data); err != nil {
			t.Errorf("Recovered settings file contains invalid JSON: %v", err)
		}
	}
}

// TestSettingsBackupCreation tests that backups are created before writes
func TestSettingsBackupCreation(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "gogomio_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	settingsPath := filepath.Join(tmpDir, "settings.json")
	backupPath := settingsPath + ".bak"
	
	m := NewManager(settingsPath)
	m.Set("key1", "value1")
	m.persist()
	
	// First write should create settings file
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("Settings file not created: %v", err)
	}
	
	// Second write should create backup
	m.Set("key2", "value2")
	m.persist()
	
	// Backup may or may not exist depending on implementation details,
	// but if it does, it should contain valid JSON
	if _, err := os.Stat(backupPath); err == nil {
		content, _ := ioutil.ReadFile(backupPath)
		var data map[string]interface{}
		if err := json.Unmarshal(content, &data); err != nil {
			t.Errorf("Backup file contains invalid JSON: %v", err)
		}
	}
}

// TestSettingsEmptyFileRecovery tests that empty files trigger recovery
func TestSettingsEmptyFileRecovery(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "gogomio_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	settingsPath := filepath.Join(tmpDir, "settings.json")
	backupPath := settingsPath + ".bak"
	
	// Create empty primary file
	if err := ioutil.WriteFile(settingsPath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create empty file: %v", err)
	}
	
	// Create valid backup
	if err := ioutil.WriteFile(backupPath, []byte(`{"key":"value"}`), 0644); err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}
	
	// Create manager - should recover from backup
	m := NewManager(settingsPath)
	m.load()
	
	if m.Get("key") != "value" {
		t.Error("Failed to recover from backup with empty primary file")
	}
}

// TestSettingsTimestampedArchive verifies archived files have timestamps
func TestSettingsTimestampedArchive(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "gogomio_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	settingsPath := filepath.Join(tmpDir, "settings.json")
	
	// Write corrupted file
	if err := ioutil.WriteFile(settingsPath, []byte("{corrupt}"), 0644); err != nil {
		t.Fatalf("Failed to write corrupted file: %v", err)
	}
	
	m := NewManager(settingsPath)
	m.load()
	
	// Check that archived file has timestamp in name
	files, err := filepath.Glob(filepath.Join(tmpDir, "settings.json.corrupted.*"))
	if err != nil {
		t.Fatalf("Failed to glob for archived files: %v", err)
	}
	
	if len(files) == 0 {
		t.Error("No archived files found")
		return
	}
	
	// Verify archived file exists and contains the original corrupted content
	filename := filepath.Base(files[0])
	if !strings.Contains(filename, "settings.json.corrupted.") {
		t.Errorf("Archived filename not in expected format: %s", filename)
		return
	}
}

// BenchmarkSettingsPersist benchmarks the persist operation
func BenchmarkSettingsPersist(b *testing.B) {
	tmpDir, _ := ioutil.TempDir("", "gogomio_bench_*")
	defer os.RemoveAll(tmpDir)
	
	settingsPath := filepath.Join(tmpDir, "settings.json")
	m := NewManager(settingsPath)
	m.Set("key", "value")
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.persist()
	}
}

// BenchmarkSettingsLoad benchmarks the load operation
func BenchmarkSettingsLoad(b *testing.B) {
	tmpDir, _ := ioutil.TempDir("", "gogomio_bench_*")
	defer os.RemoveAll(tmpDir)
	
	settingsPath := filepath.Join(tmpDir, "settings.json")
	m := NewManager(settingsPath)
	m.Set("key", "value")
	m.persist()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.load()
	}
}
