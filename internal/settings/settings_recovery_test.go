package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentManagersSetManyMergeLatestDiskContents(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	first := NewManager(settingsPath)
	second := NewManager(settingsPath)

	start := make(chan struct{})
	errs := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	go func() {
		ready.Done()
		<-start
		errs <- first.SetMany(map[string]interface{}{"first": "one"})
	}()
	go func() {
		ready.Done()
		<-start
		errs <- second.SetMany(map[string]interface{}{"second": "two"})
	}()
	ready.Wait()
	close(start)

	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent SetMany failed: %v", err)
		}
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read final settings: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(content, &got); err != nil {
		t.Fatalf("decode final settings: %v", err)
	}
	if got["first"] != "one" || got["second"] != "two" {
		t.Fatalf("concurrent updates were not merged: %v", got)
	}
}

func TestConcurrentManagersDeleteAndSetManyUseLatestDiskContents(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	seed := NewManager(settingsPath)
	if err := seed.SetMany(map[string]interface{}{"remove": "me", "keep": "yes"}); err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	deleter := NewManager(settingsPath)
	updater := NewManager(settingsPath)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	go func() {
		ready.Done()
		<-start
		errs <- deleter.Delete("remove")
	}()
	go func() {
		ready.Done()
		<-start
		errs <- updater.SetMany(map[string]interface{}{"added": "now"})
	}()
	ready.Wait()
	close(start)

	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent mutation failed: %v", err)
		}
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read final settings: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(content, &got); err != nil {
		t.Fatalf("decode final settings: %v", err)
	}
	if _, exists := got["remove"]; exists {
		t.Fatalf("deleted key was restored: %v", got)
	}
	if got["keep"] != "yes" || got["added"] != "now" {
		t.Fatalf("update or existing key was lost: %v", got)
	}
}

func TestClearRemovesLatestDiskContents(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	clearer := NewManager(settingsPath)
	writer := NewManager(settingsPath)

	if err := writer.SetMany(map[string]interface{}{"written-after-load": "value"}); err != nil {
		t.Fatalf("write settings from second manager: %v", err)
	}
	if err := clearer.Clear(); err != nil {
		t.Fatalf("clear settings: %v", err)
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("read cleared settings: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(content, &got); err != nil {
		t.Fatalf("decode cleared settings: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("Clear retained settings written by another manager: %v", got)
	}
}

// TestSettingsCorruptionRecovery tests that corrupted JSON is recovered from backup
func TestSettingsCorruptionRecovery(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "gogomio_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	settingsPath := filepath.Join(tmpDir, "settings.json")

	// Create manager and set a value
	m := NewManager(settingsPath)
	_ = m.Set("test_key", "test_value")
	_ = m.persist()

	// Verify settings file exists
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("Settings file not created: %v", err)
	}

	// Create a backup with good data
	backupPath := settingsPath + ".bak"
	if err := os.WriteFile(backupPath, []byte(`{"test_key":"test_value"}`), 0644); err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Corrupt the primary file
	if err := os.WriteFile(settingsPath, []byte("{invalid json}"), 0644); err != nil {
		t.Fatalf("Failed to corrupt settings: %v", err)
	}

	// Create new manager instance - should recover from backup
	m2 := NewManager(settingsPath)
	if err := m2.load(); err != nil {
		t.Fatalf("load recovered settings: %v", err)
	}

	// Verify recovery
	if m2.Get("test_key") != "test_value" {
		t.Errorf("Failed to recover from backup: got %v, want test_value", m2.Get("test_key"))
	}
}

// TestSettingsCorruptedFileArchiving tests that corrupted files are archived
func TestSettingsCorruptedFileArchiving(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.json")
	corruptContent := []byte("{corrupt}")

	// Write corrupted file with no backup
	if err := os.WriteFile(settingsPath, corruptContent, 0644); err != nil {
		t.Fatalf("Failed to write corrupted file: %v", err)
	}

	// Loading without a valid backup should archive the original and start clean.
	m := NewManager(settingsPath)
	if err := m.load(); err != nil {
		t.Fatalf("load corrupted settings: %v", err)
	}

	files, err := filepath.Glob(settingsPath + ".corrupted.*")
	if err != nil {
		t.Fatalf("find archived settings: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("archived settings files = %d, want exactly 1", len(files))
	}
	archivedContent, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("read archived settings: %v", err)
	}
	if string(archivedContent) != string(corruptContent) {
		t.Fatalf("archived content = %q, want original corrupt content %q", archivedContent, corruptContent)
	}
	if got := m.GetAll(); len(got) != 0 {
		t.Fatalf("settings after recovery = %v, want an empty map", got)
	}
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Fatalf("corrupt primary file should be moved to archive; stat error = %v", err)
	}
}

// TestSettingsBackupCreation tests that backups are created before writes
func TestSettingsBackupCreation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gogomio_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	settingsPath := filepath.Join(tmpDir, "settings.json")
	backupPath := settingsPath + ".bak"

	m := NewManager(settingsPath)
	if err := m.Set("key1", "value1"); err != nil {
		t.Fatalf("Failed first write: %v", err)
	}

	// First write should create settings file
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("Settings file not created: %v", err)
	}

	// Second write should create backup
	if err := m.Set("key2", "value2"); err != nil {
		t.Fatalf("Failed second write: %v", err)
	}

	// By design, persist() copies the current settings file to .bak before overwrite,
	// so after second write the backup must exist and contain the previous state.
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("Backup file was not created after second write: %v", err)
	}

	content, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("Failed to read backup: %v", err)
	}

	var backupData map[string]interface{}
	if err := json.Unmarshal(content, &backupData); err != nil {
		t.Fatalf("Backup file contains invalid JSON: %v", err)
	}

	expected := map[string]interface{}{
		"key1": "value1",
	}
	if len(backupData) != len(expected) {
		t.Fatalf("Backup JSON has unexpected number of keys: got %d, want %d (content=%v)", len(backupData), len(expected), backupData)
	}
	for k, v := range expected {
		if backupData[k] != v {
			t.Fatalf("Backup JSON mismatch for key %q: got %v, want %v (content=%v)", k, backupData[k], v, backupData)
		}
	}
}

// TestSettingsEmptyFileRecovery tests that empty files trigger recovery
func TestSettingsEmptyFileRecovery(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gogomio_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	settingsPath := filepath.Join(tmpDir, "settings.json")
	backupPath := settingsPath + ".bak"

	// Create empty primary file
	if err := os.WriteFile(settingsPath, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create empty file: %v", err)
	}

	// Create valid backup
	if err := os.WriteFile(backupPath, []byte(`{"key":"value"}`), 0644); err != nil {
		t.Fatalf("Failed to create backup: %v", err)
	}

	// Create manager - should recover from backup
	m := NewManager(settingsPath)
	if err := m.load(); err != nil {
		t.Fatalf("load settings recovered from empty primary file: %v", err)
	}

	if m.Get("key") != "value" {
		t.Error("Failed to recover from backup with empty primary file")
	}
}

// TestSettingsRecoveryConcurrentWithWrite ensures recovery writes are serialized with concurrent persist writes.
func TestSettingsRecoveryConcurrentWithWrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gogomio_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	settingsPath := filepath.Join(tmpDir, "settings.json")
	backupPath := settingsPath + ".bak"

	if err := os.WriteFile(settingsPath, []byte("{corrupt}"), 0644); err != nil {
		t.Fatalf("failed to write corrupt settings: %v", err)
	}
	if err := os.WriteFile(backupPath, []byte(`{"from_backup":"ok"}`), 0644); err != nil {
		t.Fatalf("failed to write backup settings: %v", err)
	}

	recoveryMgr := NewManager(settingsPath)
	writerMgr := NewManager(settingsPath)

	errCh := make(chan error, 2)
	go func() { errCh <- recoveryMgr.load() }()
	go func() { errCh <- writerMgr.Set("writer", "won") }()

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("concurrent operation failed: %v", err)
		}
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read resulting settings file: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(content, &got); err != nil {
		t.Fatalf("resulting settings file contains invalid JSON: %v\ncontent=%q", err, string(content))
	}

	if got["writer"] != "won" {
		t.Fatalf("writer update missing after concurrent recovery/write: %v", got)
	}
}

// TestSettingsArchiveConcurrentWithWrite ensures archive rename is serialized with concurrent writes.
func TestSettingsArchiveConcurrentWithWrite(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gogomio_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	settingsPath := filepath.Join(tmpDir, "settings.json")
	if err := os.WriteFile(settingsPath, []byte("{corrupt}"), 0644); err != nil {
		t.Fatalf("failed to write corrupt settings: %v", err)
	}

	recoveryMgr := NewManager(settingsPath)
	writerMgr := NewManager(settingsPath)

	errCh := make(chan error, 2)
	go func() { errCh <- recoveryMgr.load() }()
	go func() { errCh <- writerMgr.Set("writer", "won") }()

	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil {
			t.Fatalf("concurrent operation failed: %v", err)
		}
	}

	content, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("failed to read resulting settings file: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(content, &got); err != nil {
		t.Fatalf("resulting settings file contains invalid JSON: %v\ncontent=%q", err, string(content))
	}

	if got["writer"] != "won" {
		t.Fatalf("writer update missing after concurrent archive/write: %v", got)
	}

	archived, err := filepath.Glob(filepath.Join(tmpDir, "settings.json.corrupted.*"))
	if err != nil {
		t.Fatalf("failed to glob archive files: %v", err)
	}
	if len(archived) == 0 {
		t.Fatal("expected at least one archived corrupted file")
	}
}
