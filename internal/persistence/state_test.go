// Package persistence provides state persistence for pipeline execution.
package persistence

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestNewStateStore(t *testing.T) {
	tmpDir := t.TempDir()

	store := NewStateStore(tmpDir)
	if store == nil {
		t.Fatal("NewStateStore returned nil")
	}
	if store.basePath != tmpDir {
		t.Errorf("basePath = %q, want %q", store.basePath, tmpDir)
	}
}

func TestNewStateStore_DefaultPath(t *testing.T) {
	store := NewStateStore("")
	if store == nil {
		t.Fatal("NewStateStore returned nil")
	}
	if store.basePath != DefaultStatePath {
		t.Errorf("basePath = %q, want default %q", store.basePath, DefaultStatePath)
	}
}

func TestStateStore_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStateStore(tmpDir)

	pipelineID := "test-pipeline-1"
	timestamp := time.Date(2026, 1, 26, 10, 30, 0, 0, time.UTC)
	lastID := "12345"

	state := &State{
		PipelineID:    pipelineID,
		LastTimestamp: &timestamp,
		LastID:        &lastID,
		UpdatedAt:     time.Now(),
	}

	// Save state
	err := store.Save(pipelineID, state)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	filePath := filepath.Join(tmpDir, pipelineID+".json")
	if _, statErr := os.Stat(filePath); os.IsNotExist(statErr) {
		t.Errorf("State file not created at %s", filePath)
	}

	// Load state
	loaded, err := store.Load(pipelineID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.PipelineID != pipelineID {
		t.Errorf("PipelineID = %q, want %q", loaded.PipelineID, pipelineID)
	}
	if loaded.LastTimestamp == nil || !loaded.LastTimestamp.Equal(timestamp) {
		t.Errorf("LastTimestamp = %v, want %v", loaded.LastTimestamp, timestamp)
	}
	if loaded.LastID == nil || *loaded.LastID != lastID {
		t.Errorf("LastID = %v, want %v", loaded.LastID, &lastID)
	}
}

func TestStateStore_Load_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStateStore(tmpDir)

	// Load non-existent state
	loaded, err := store.Load("non-existent-pipeline")
	if err != nil {
		t.Fatalf("Load should not return error for non-existent state: %v", err)
	}
	if loaded != nil {
		t.Errorf("Load should return nil for non-existent state, got %v", loaded)
	}
}

func TestStateStore_Save_TimestampOnly(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStateStore(tmpDir)

	pipelineID := "timestamp-only-pipeline"
	timestamp := time.Date(2026, 1, 26, 12, 0, 0, 0, time.UTC)

	state := &State{
		PipelineID:    pipelineID,
		LastTimestamp: &timestamp,
		LastID:        nil, // No ID
		UpdatedAt:     time.Now(),
	}

	err := store.Save(pipelineID, state)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Load(pipelineID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.LastTimestamp == nil || !loaded.LastTimestamp.Equal(timestamp) {
		t.Errorf("LastTimestamp = %v, want %v", loaded.LastTimestamp, timestamp)
	}
	if loaded.LastID != nil {
		t.Errorf("LastID should be nil, got %v", loaded.LastID)
	}
}

func TestStateStore_Save_IDOnly(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStateStore(tmpDir)

	pipelineID := "id-only-pipeline"
	lastID := "abc-123"

	state := &State{
		PipelineID:    pipelineID,
		LastTimestamp: nil, // No timestamp
		LastID:        &lastID,
		UpdatedAt:     time.Now(),
	}

	err := store.Save(pipelineID, state)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Load(pipelineID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.LastTimestamp != nil {
		t.Errorf("LastTimestamp should be nil, got %v", loaded.LastTimestamp)
	}
	if loaded.LastID == nil || *loaded.LastID != lastID {
		t.Errorf("LastID = %v, want %v", loaded.LastID, &lastID)
	}
}

func TestStateStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStateStore(tmpDir)

	pipelineID := "delete-test-pipeline"
	timestamp := time.Now()

	state := &State{
		PipelineID:    pipelineID,
		LastTimestamp: &timestamp,
		UpdatedAt:     time.Now(),
	}

	// Save state
	err := store.Save(pipelineID, state)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify it exists
	loaded, err := store.Load(pipelineID)
	if err != nil || loaded == nil {
		t.Fatalf("State should exist after save")
	}

	// Delete state
	err = store.Delete(pipelineID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	loaded, err = store.Load(pipelineID)
	if err != nil {
		t.Fatalf("Load after delete should not error: %v", err)
	}
	if loaded != nil {
		t.Errorf("State should be nil after delete, got %v", loaded)
	}
}

func TestStateStore_Delete_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStateStore(tmpDir)

	// Delete non-existent state should not error
	err := store.Delete("non-existent-pipeline")
	if err != nil {
		t.Errorf("Delete should not error for non-existent state: %v", err)
	}
}

func TestStateStore_PerPipelineIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStateStore(tmpDir)

	// Create states for two different pipelines
	pipelineID1 := "pipeline-1"
	pipelineID2 := "pipeline-2"

	timestamp1 := time.Date(2026, 1, 26, 10, 0, 0, 0, time.UTC)
	timestamp2 := time.Date(2026, 1, 26, 11, 0, 0, 0, time.UTC)
	id1 := "id-1"
	id2 := "id-2"

	state1 := &State{
		PipelineID:    pipelineID1,
		LastTimestamp: &timestamp1,
		LastID:        &id1,
		UpdatedAt:     time.Now(),
	}
	state2 := &State{
		PipelineID:    pipelineID2,
		LastTimestamp: &timestamp2,
		LastID:        &id2,
		UpdatedAt:     time.Now(),
	}

	// Save both states
	if err := store.Save(pipelineID1, state1); err != nil {
		t.Fatalf("Save state1 failed: %v", err)
	}
	if err := store.Save(pipelineID2, state2); err != nil {
		t.Fatalf("Save state2 failed: %v", err)
	}

	// Load and verify isolation
	loaded1, err := store.Load(pipelineID1)
	if err != nil {
		t.Fatalf("Load pipeline1 failed: %v", err)
	}
	loaded2, err := store.Load(pipelineID2)
	if err != nil {
		t.Fatalf("Load pipeline2 failed: %v", err)
	}

	// Verify pipeline 1 state
	if loaded1.LastTimestamp == nil || !loaded1.LastTimestamp.Equal(timestamp1) {
		t.Errorf("Pipeline1 timestamp = %v, want %v", loaded1.LastTimestamp, timestamp1)
	}
	if loaded1.LastID == nil || *loaded1.LastID != id1 {
		t.Errorf("Pipeline1 ID = %v, want %v", loaded1.LastID, id1)
	}

	// Verify pipeline 2 state
	if loaded2.LastTimestamp == nil || !loaded2.LastTimestamp.Equal(timestamp2) {
		t.Errorf("Pipeline2 timestamp = %v, want %v", loaded2.LastTimestamp, timestamp2)
	}
	if loaded2.LastID == nil || *loaded2.LastID != id2 {
		t.Errorf("Pipeline2 ID = %v, want %v", loaded2.LastID, id2)
	}
}

func TestStateStore_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStateStore(tmpDir)

	pipelineID := "concurrent-test-pipeline"
	iterations := 100

	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			timestamp := time.Now()
			id := "id-" + string(rune('a'+n%26))
			state := &State{
				PipelineID:    pipelineID,
				LastTimestamp: &timestamp,
				LastID:        &id,
				UpdatedAt:     time.Now(),
			}
			if err := store.Save(pipelineID, state); err != nil {
				t.Errorf("Concurrent save failed: %v", err)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := store.Load(pipelineID)
			if err != nil {
				t.Errorf("Concurrent load failed: %v", err)
			}
		}()
	}

	wg.Wait()

	// Verify final state is valid
	loaded, err := store.Load(pipelineID)
	if err != nil {
		t.Fatalf("Final load failed: %v", err)
	}
	if loaded == nil {
		t.Error("Final state should not be nil")
	}
}

func TestStateStore_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	subPath := filepath.Join(tmpDir, "nested", "state", "dir")

	store := NewStateStore(subPath)

	pipelineID := "test-pipeline"
	timestamp := time.Now()
	state := &State{
		PipelineID:    pipelineID,
		LastTimestamp: &timestamp,
		UpdatedAt:     time.Now(),
	}

	// Save should create the directory
	err := store.Save(pipelineID, state)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(subPath); os.IsNotExist(err) {
		t.Errorf("Directory was not created: %s", subPath)
	}
}

func TestStateStore_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStateStore(tmpDir)

	pipelineID := "atomic-test-pipeline"

	// Save initial state
	timestamp1 := time.Date(2026, 1, 26, 10, 0, 0, 0, time.UTC)
	state1 := &State{
		PipelineID:    pipelineID,
		LastTimestamp: &timestamp1,
		UpdatedAt:     time.Now(),
	}
	if err := store.Save(pipelineID, state1); err != nil {
		t.Fatalf("First save failed: %v", err)
	}

	// Save updated state
	timestamp2 := time.Date(2026, 1, 26, 11, 0, 0, 0, time.UTC)
	state2 := &State{
		PipelineID:    pipelineID,
		LastTimestamp: &timestamp2,
		UpdatedAt:     time.Now(),
	}
	if err := store.Save(pipelineID, state2); err != nil {
		t.Fatalf("Second save failed: %v", err)
	}

	// Load and verify latest state
	loaded, err := store.Load(pipelineID)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.LastTimestamp == nil || !loaded.LastTimestamp.Equal(timestamp2) {
		t.Errorf("Timestamp = %v, want %v", loaded.LastTimestamp, timestamp2)
	}

	// Verify no temp files left behind
	files, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".tmp" {
			t.Errorf("Temp file left behind: %s", f.Name())
		}
	}
}

func TestState_FormatTimestamp(t *testing.T) {
	timestamp := time.Date(2026, 1, 26, 10, 30, 0, 0, time.UTC)
	state := &State{
		PipelineID:    "test",
		LastTimestamp: &timestamp,
	}

	formatted := state.FormatTimestamp()
	expected := "2026-01-26T10:30:00Z"
	if formatted != expected {
		t.Errorf("FormatTimestamp() = %q, want %q", formatted, expected)
	}
}

func TestState_FormatTimestamp_Nil(t *testing.T) {
	state := &State{
		PipelineID:    "test",
		LastTimestamp: nil,
	}

	formatted := state.FormatTimestamp()
	if formatted != "" {
		t.Errorf("FormatTimestamp() = %q, want empty string", formatted)
	}
}
