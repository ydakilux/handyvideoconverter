package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	"video-converter/internal/types"
)

func fullRecord() types.Record {
	return types.Record{
		OriginalSize:           4096000,
		ConvertedSize:          2048000,
		Output:                 `D:\HSORTED\Movies\test.mp4`,
		Note:                   "conversion_complete",
		Error:                  "",
		SourceCodec:            "h264",
		SourceContainer:        "matroska",
		SourcePath:             `D:\Videos\Movies\test.mkv`,
		Width:                  1920,
		Height:                 1080,
		DurationSecs:           3600.5,
		ConvertedAt:            "2026-04-06T12:00:00Z",
		ConversionDurationSecs: 120.75,
	}
}

func newTestSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath, newTestLogger())
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSQLiteRoundTrip(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	rec := fullRecord()
	if err := store.UpdateRecord(ctx, `D:\`, "hash_abc123", rec); err != nil {
		t.Fatalf("UpdateRecord: %v", err)
	}

	got, err := store.GetRecord(ctx, `D:\`, "hash_abc123")
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	if got == nil {
		t.Fatal("GetRecord returned nil after UpdateRecord")
	}

	if got.OriginalSize != rec.OriginalSize {
		t.Errorf("OriginalSize = %d, want %d", got.OriginalSize, rec.OriginalSize)
	}
	if got.ConvertedSize != rec.ConvertedSize {
		t.Errorf("ConvertedSize = %d, want %d", got.ConvertedSize, rec.ConvertedSize)
	}
	if got.Output != rec.Output {
		t.Errorf("Output = %q, want %q", got.Output, rec.Output)
	}
	if got.Note != rec.Note {
		t.Errorf("Note = %q, want %q", got.Note, rec.Note)
	}
	if got.Error != rec.Error {
		t.Errorf("Error = %q, want %q", got.Error, rec.Error)
	}
	if got.SourceCodec != rec.SourceCodec {
		t.Errorf("SourceCodec = %q, want %q", got.SourceCodec, rec.SourceCodec)
	}
	if got.SourceContainer != rec.SourceContainer {
		t.Errorf("SourceContainer = %q, want %q", got.SourceContainer, rec.SourceContainer)
	}
	if got.SourcePath != rec.SourcePath {
		t.Errorf("SourcePath = %q, want %q", got.SourcePath, rec.SourcePath)
	}
	if got.Width != rec.Width {
		t.Errorf("Width = %d, want %d", got.Width, rec.Width)
	}
	if got.Height != rec.Height {
		t.Errorf("Height = %d, want %d", got.Height, rec.Height)
	}
	if got.DurationSecs != rec.DurationSecs {
		t.Errorf("DurationSecs = %f, want %f", got.DurationSecs, rec.DurationSecs)
	}
	if got.ConvertedAt != rec.ConvertedAt {
		t.Errorf("ConvertedAt = %q, want %q", got.ConvertedAt, rec.ConvertedAt)
	}
	if got.ConversionDurationSecs != rec.ConversionDurationSecs {
		t.Errorf("ConversionDurationSecs = %f, want %f", got.ConversionDurationSecs, rec.ConversionDurationSecs)
	}
}

func TestSQLiteUpsert(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	rec := types.Record{
		OriginalSize:  1000,
		ConvertedSize: 800,
		Note:          "first_pass",
	}
	if err := store.UpdateRecord(ctx, `D:\`, "hash_upsert", rec); err != nil {
		t.Fatalf("UpdateRecord (insert): %v", err)
	}

	rec.ConvertedSize = 600
	rec.Note = "second_pass"
	rec.SourceCodec = "h264"
	if err := store.UpdateRecord(ctx, `D:\`, "hash_upsert", rec); err != nil {
		t.Fatalf("UpdateRecord (upsert): %v", err)
	}

	got, err := store.GetRecord(ctx, `D:\`, "hash_upsert")
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	if got == nil {
		t.Fatal("GetRecord returned nil")
	}
	if got.ConvertedSize != 600 {
		t.Errorf("ConvertedSize = %d, want 600", got.ConvertedSize)
	}
	if got.Note != "second_pass" {
		t.Errorf("Note = %q, want %q", got.Note, "second_pass")
	}
	if got.SourceCodec != "h264" {
		t.Errorf("SourceCodec = %q, want %q", got.SourceCodec, "h264")
	}
}

func TestSQLiteNotFound(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	got, err := store.GetRecord(ctx, `D:\`, "nonexistent_hash")
	if err != nil {
		t.Fatalf("GetRecord error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent record, got %+v", got)
	}
}

func TestSQLiteMultipleDrives(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	hash := "shared_hash"
	recD := types.Record{OriginalSize: 1000, Output: `D:\HSORTED\test.mp4`}
	recE := types.Record{OriginalSize: 2000, Output: `E:\HSORTED\test.mp4`}

	if err := store.UpdateRecord(ctx, `D:\`, hash, recD); err != nil {
		t.Fatalf("UpdateRecord D: %v", err)
	}
	if err := store.UpdateRecord(ctx, `E:\`, hash, recE); err != nil {
		t.Fatalf("UpdateRecord E: %v", err)
	}

	gotD, err := store.GetRecord(ctx, `D:\`, hash)
	if err != nil {
		t.Fatalf("GetRecord D: %v", err)
	}
	gotE, err := store.GetRecord(ctx, `E:\`, hash)
	if err != nil {
		t.Fatalf("GetRecord E: %v", err)
	}

	if gotD == nil || gotE == nil {
		t.Fatal("one of the drive records is nil")
	}
	if gotD.OriginalSize != 1000 {
		t.Errorf("D drive OriginalSize = %d, want 1000", gotD.OriginalSize)
	}
	if gotE.OriginalSize != 2000 {
		t.Errorf("E drive OriginalSize = %d, want 2000", gotE.OriginalSize)
	}
	if gotD.Output != recD.Output {
		t.Errorf("D drive Output = %q, want %q", gotD.Output, recD.Output)
	}
	if gotE.Output != recE.Output {
		t.Errorf("E drive Output = %q, want %q", gotE.Output, recE.Output)
	}
}

func TestSQLiteConcurrentWrites(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	const goroutines = 10
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			hash := "concurrent_hash_" + string(rune('a'+idx))
			rec := types.Record{
				OriginalSize:  int64(idx * 1000),
				ConvertedSize: int64(idx * 500),
				SourceCodec:   "h264",
			}
			if err := store.UpdateRecord(ctx, `D:\`, hash, rec); err != nil {
				t.Errorf("goroutine %d UpdateRecord: %v", idx, err)
				return
			}
			got, err := store.GetRecord(ctx, `D:\`, hash)
			if err != nil {
				t.Errorf("goroutine %d GetRecord: %v", idx, err)
				return
			}
			if got == nil {
				t.Errorf("goroutine %d: GetRecord returned nil", idx)
				return
			}
			if got.OriginalSize != int64(idx*1000) {
				t.Errorf("goroutine %d: OriginalSize = %d, want %d", idx, got.OriginalSize, idx*1000)
			}
		}(i)
	}

	wg.Wait()
}

func TestSQLiteDBCreation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "creation_test.db")

	store, err := NewSQLiteStore(dbPath, newTestLogger())
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()
	got, err := store.GetRecord(ctx, `D:\`, "any_hash")
	if err != nil {
		t.Fatalf("GetRecord on fresh DB: %v", err)
	}
	if got != nil {
		t.Error("expected nil for empty DB")
	}

	if err := store.UpdateRecord(ctx, `D:\`, "test_hash", types.Record{OriginalSize: 42}); err != nil {
		t.Fatalf("UpdateRecord on fresh DB: %v", err)
	}
	got, err = store.GetRecord(ctx, `D:\`, "test_hash")
	if err != nil {
		t.Fatalf("GetRecord after insert: %v", err)
	}
	if got == nil || got.OriginalSize != 42 {
		t.Errorf("round-trip on fresh DB failed: got %+v", got)
	}
}

func TestSQLiteWALMode(t *testing.T) {
	store := newTestSQLiteStore(t)

	var journalMode string
	err := store.db.QueryRow("PRAGMA journal_mode").Scan(&journalMode)
	if err != nil {
		t.Fatalf("PRAGMA journal_mode query: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q", journalMode, "wal")
	}
}

func TestSQLiteCloseAndUse(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "close_test.db")
	store, err := NewSQLiteStore(dbPath, newTestLogger())
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	ctx := context.Background()
	_, err = store.GetRecord(ctx, `D:\`, "any_hash")
	if err == nil {
		t.Error("expected error after Close, got nil")
	}

	err = store.UpdateRecord(ctx, `D:\`, "any_hash", types.Record{OriginalSize: 1})
	if err == nil {
		t.Error("expected error from UpdateRecord after Close, got nil")
	}
}

func TestSQLiteEmptyFields(t *testing.T) {
	store := newTestSQLiteStore(t)
	ctx := context.Background()

	rec := types.Record{OriginalSize: 999}
	if err := store.UpdateRecord(ctx, `D:\`, "sparse_hash", rec); err != nil {
		t.Fatalf("UpdateRecord: %v", err)
	}

	got, err := store.GetRecord(ctx, `D:\`, "sparse_hash")
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	if got == nil {
		t.Fatal("GetRecord returned nil")
	}
	if got.OriginalSize != 999 {
		t.Errorf("OriginalSize = %d, want 999", got.OriginalSize)
	}
	if got.ConvertedSize != 0 {
		t.Errorf("ConvertedSize = %d, want 0", got.ConvertedSize)
	}
	if got.Output != "" {
		t.Errorf("Output = %q, want empty", got.Output)
	}
	if got.Note != "" {
		t.Errorf("Note = %q, want empty", got.Note)
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty", got.Error)
	}
	if got.SourceCodec != "" {
		t.Errorf("SourceCodec = %q, want empty", got.SourceCodec)
	}
	if got.SourceContainer != "" {
		t.Errorf("SourceContainer = %q, want empty", got.SourceContainer)
	}
	if got.SourcePath != "" {
		t.Errorf("SourcePath = %q, want empty", got.SourcePath)
	}
	if got.Width != 0 {
		t.Errorf("Width = %d, want 0", got.Width)
	}
	if got.Height != 0 {
		t.Errorf("Height = %d, want 0", got.Height)
	}
	if got.DurationSecs != 0 {
		t.Errorf("DurationSecs = %f, want 0", got.DurationSecs)
	}
	if got.ConvertedAt != "" {
		t.Errorf("ConvertedAt = %q, want empty", got.ConvertedAt)
	}
	if got.ConversionDurationSecs != 0 {
		t.Errorf("ConversionDurationSecs = %f, want 0", got.ConversionDurationSecs)
	}
}

var _ = (*sql.DB)(nil)
