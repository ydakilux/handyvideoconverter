package database

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/sirupsen/logrus"

	"video-converter/internal/types"
)

func newTestLogger() *logrus.Logger {
	l := logrus.New()
	l.SetOutput(os.Stderr)
	l.SetLevel(logrus.WarnLevel)
	return l
}

func TestNewDatabaseManager(t *testing.T) {
	dm := NewDatabaseManager(newTestLogger())
	if dm == nil {
		t.Fatal("NewDatabaseManager returned nil")
	}
	if dm.dbs == nil {
		t.Error("dbs map is nil")
	}
	if dm.dirty == nil {
		t.Error("dirty map is nil")
	}
	if dm.logger == nil {
		t.Error("logger is nil")
	}
}

func TestUpdateGetRoundTrip(t *testing.T) {
	dm := NewDatabaseManager(newTestLogger())

	tmpDir := t.TempDir()
	driveRoot := tmpDir + string(filepath.Separator)

	rec := types.Record{
		OriginalSize:  1000,
		ConvertedSize: 500,
		Output:        filepath.Join(driveRoot, "HSORTED", "test.mp4"),
	}

	dm.UpdateRecord(driveRoot, "abc123", rec)

	got := dm.GetRecord(driveRoot, "abc123")
	if got == nil {
		t.Fatal("GetRecord returned nil after UpdateRecord")
	}
	if got.OriginalSize != 1000 {
		t.Errorf("OriginalSize = %d, want 1000", got.OriginalSize)
	}
	if got.ConvertedSize != 500 {
		t.Errorf("ConvertedSize = %d, want 500", got.ConvertedSize)
	}
	if got.Output != rec.Output {
		t.Errorf("Output = %q, want %q", got.Output, rec.Output)
	}
}

func TestGetRecordNonExistentDrive(t *testing.T) {
	dm := NewDatabaseManager(newTestLogger())

	// Use a temp dir that has no converted_files.json
	tmpDir := t.TempDir()
	driveRoot := tmpDir + string(filepath.Separator)

	got := dm.GetRecord(driveRoot, "nonexistent_hash")
	if got != nil {
		t.Errorf("expected nil for non-existent drive+hash, got %+v", got)
	}
}

func TestGetRecordNonExistentHash(t *testing.T) {
	dm := NewDatabaseManager(newTestLogger())

	tmpDir := t.TempDir()
	driveRoot := tmpDir + string(filepath.Separator)

	// Add a record so the drive is loaded
	dm.UpdateRecord(driveRoot, "existing_hash", types.Record{OriginalSize: 100})

	got := dm.GetRecord(driveRoot, "missing_hash")
	if got != nil {
		t.Errorf("expected nil for non-existent hash, got %+v", got)
	}
}

func TestSaveLoadPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	driveRoot := tmpDir + string(filepath.Separator)

	logger := newTestLogger()

	// First manager: write and save
	dm1 := NewDatabaseManager(logger)
	dm1.UpdateRecord(driveRoot, "hash_a", types.Record{
		OriginalSize:  2000,
		ConvertedSize: 1200,
		Output:        "output_a.mp4",
	})
	dm1.UpdateRecord(driveRoot, "hash_b", types.Record{
		OriginalSize: 3000,
		Note:         "not_beneficial",
	})
	dm1.SaveAll()

	// Verify file exists on disk
	dbPath := filepath.Join(driveRoot, "converted_files.json")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("converted_files.json was not created")
	}

	// Verify JSON is valid
	data, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}
	var records map[string]types.Record
	if err := json.Unmarshal(data, &records); err != nil {
		t.Fatalf("Invalid JSON in saved file: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("Expected 2 records on disk, got %d", len(records))
	}

	// Second manager: load from disk
	dm2 := NewDatabaseManager(logger)
	got := dm2.GetRecord(driveRoot, "hash_a")
	if got == nil {
		t.Fatal("GetRecord returned nil after loading from disk")
	}
	if got.OriginalSize != 2000 {
		t.Errorf("OriginalSize = %d, want 2000", got.OriginalSize)
	}
	if got.Output != "output_a.mp4" {
		t.Errorf("Output = %q, want %q", got.Output, "output_a.mp4")
	}

	gotB := dm2.GetRecord(driveRoot, "hash_b")
	if gotB == nil {
		t.Fatal("GetRecord for hash_b returned nil after loading from disk")
	}
	if gotB.Note != "not_beneficial" {
		t.Errorf("Note = %q, want %q", gotB.Note, "not_beneficial")
	}
}

func TestConcurrentGetRecord(t *testing.T) {
	tmpDir := t.TempDir()
	driveRoot := tmpDir + string(filepath.Separator)

	dm := NewDatabaseManager(newTestLogger())

	// Pre-populate some records
	for i := 0; i < 10; i++ {
		hash := "hash_" + string(rune('a'+i))
		dm.UpdateRecord(driveRoot, hash, types.Record{
			OriginalSize: int64(i * 100),
		})
	}

	var wg sync.WaitGroup
	const goroutines = 10

	// Concurrent reads and writes
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			readHash := "hash_" + string(rune('a'+(idx%10)))
			writeHash := "write_" + string(rune('a'+idx))

			// Read existing
			_ = dm.GetRecord(driveRoot, readHash)

			// Write new
			dm.UpdateRecord(driveRoot, writeHash, types.Record{
				OriginalSize: int64(idx * 200),
			})

			// Read what we just wrote
			_ = dm.GetRecord(driveRoot, writeHash)
		}(i)
	}

	wg.Wait()
}

func TestConcurrentUpdateAndSave(t *testing.T) {
	tmpDir := t.TempDir()

	dm := NewDatabaseManager(newTestLogger())

	var wg sync.WaitGroup
	const goroutines = 5

	// Multiple goroutines updating different drives while SaveAll runs
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Each goroutine creates its own "drive" subdirectory
			driveDir := filepath.Join(tmpDir, "drive_"+string(rune('a'+idx)))
			os.MkdirAll(driveDir, 0755)
			driveRoot := driveDir + string(filepath.Separator)

			for j := 0; j < 20; j++ {
				hash := "hash_" + string(rune('a'+j))
				dm.UpdateRecord(driveRoot, hash, types.Record{
					OriginalSize:  int64(idx*1000 + j),
					ConvertedSize: int64(idx*500 + j),
				})

				// Periodically save
				if j%5 == 0 {
					dm.SaveAll()
				}
			}
		}(i)
	}

	wg.Wait()

	// Final save
	dm.SaveAll()

	// Verify all drives have their files
	for i := 0; i < goroutines; i++ {
		driveDir := filepath.Join(tmpDir, "drive_"+string(rune('a'+i)))
		driveRoot := driveDir + string(filepath.Separator)
		dbPath := filepath.Join(driveRoot, "converted_files.json")
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			t.Errorf("converted_files.json not found for drive_%c", rune('a'+i))
		}
	}
}

func TestSaveAllCleansDirtyFlag(t *testing.T) {
	tmpDir := t.TempDir()
	driveRoot := tmpDir + string(filepath.Separator)

	dm := NewDatabaseManager(newTestLogger())
	dm.UpdateRecord(driveRoot, "hash1", types.Record{OriginalSize: 100})

	dm.SaveAll()

	// After SaveAll, dirty should be false
	dm.mu.Lock()
	if dm.dirty[driveRoot] {
		t.Error("dirty flag should be false after SaveAll")
	}
	dm.mu.Unlock()

	// SaveAll again should be a no-op (no file writes)
	dm.SaveAll()
}

func TestLoadDBCorruptedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	driveRoot := tmpDir + string(filepath.Separator)

	// Write corrupted JSON
	dbPath := filepath.Join(driveRoot, "converted_files.json")
	os.WriteFile(dbPath, []byte("not valid json{{{"), 0644)

	dm := NewDatabaseManager(newTestLogger())
	got := dm.GetRecord(driveRoot, "any_hash")

	// Should gracefully return nil, not panic
	if got != nil {
		t.Errorf("expected nil for corrupted DB, got %+v", got)
	}
}
