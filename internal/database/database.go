package database

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"

	"video-converter/internal/types"
)

// DatabaseManager manages per-drive cache files with thread-safe access.
// Uses sync.Mutex (not RWMutex) to avoid lock promotion races in GetRecord.
type DatabaseManager struct {
	mu     sync.Mutex
	dbs    map[string]map[string]types.Record
	dirty  map[string]bool
	logger *logrus.Logger
}

func NewDatabaseManager(logger *logrus.Logger) *DatabaseManager {
	return &DatabaseManager{
		dbs:    make(map[string]map[string]types.Record),
		dirty:  make(map[string]bool),
		logger: logger,
	}
}

func (db *DatabaseManager) GetRecord(driveRoot, fileHash string) *types.Record {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.dbs[driveRoot] == nil {
		db.loadDB(driveRoot)
	}

	if rec, ok := db.dbs[driveRoot][fileHash]; ok {
		return &rec
	}
	return nil
}

func (db *DatabaseManager) UpdateRecord(driveRoot, fileHash string, rec types.Record) {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.dbs[driveRoot] == nil {
		db.loadDB(driveRoot)
	}

	db.dbs[driveRoot][fileHash] = rec
	db.dirty[driveRoot] = true
}

// fallbackDBPath computes a user-writable fallback path for the per-drive DB cache.
// Returns empty string if the path cannot be determined.
func fallbackDBPath(driveRoot string) string {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	vol := filepath.VolumeName(driveRoot)
	letter := strings.TrimSuffix(vol, ":")
	if letter == "" {
		return ""
	}
	return filepath.Join(cacheDir, "video-converter", letter, "converted_files.json")
}

// loadDB reads the cache file for a drive. Must be called while holding db.mu.
func (db *DatabaseManager) loadDB(driveRoot string) {
	dbPath := filepath.Join(driveRoot, "converted_files.json")
	data, err := os.ReadFile(dbPath)
	if err != nil {
		// Try fallback location
		if fbPath := fallbackDBPath(driveRoot); fbPath != "" {
			data, err = os.ReadFile(fbPath)
		}
		if err != nil {
			db.dbs[driveRoot] = make(map[string]types.Record)
			return
		}
	}

	var records map[string]types.Record
	if err := json.Unmarshal(data, &records); err != nil {
		db.dbs[driveRoot] = make(map[string]types.Record)
		return
	}
	db.dbs[driveRoot] = records
}

func (db *DatabaseManager) SaveAll() {
	db.mu.Lock()
	defer db.mu.Unlock()

	for driveRoot, isDirty := range db.dirty {
		if !isDirty {
			continue
		}

		dbPath := filepath.Join(driveRoot, "converted_files.json")
		tmpPath := dbPath + ".tmp"

		data, err := json.MarshalIndent(db.dbs[driveRoot], "", "  ")
		if err != nil {
			db.logger.Errorf("Failed to marshal DB for %s: %v", driveRoot, err)
			continue
		}

		if err := os.WriteFile(tmpPath, data, 0644); err != nil {
			// Try fallback location
			if fbPath := fallbackDBPath(driveRoot); fbPath != "" {
				fbDir := filepath.Dir(fbPath)
				if mkErr := os.MkdirAll(fbDir, 0755); mkErr == nil {
					fbTmp := fbPath + ".tmp"
					if wErr := os.WriteFile(fbTmp, data, 0644); wErr == nil {
						if rErr := os.Rename(fbTmp, fbPath); rErr == nil {
							db.logger.Infof("DB saved to fallback location for %s", driveRoot)
							db.dirty[driveRoot] = false
							continue
						}
						os.Remove(fbTmp)
					}
				}
			}
			db.logger.Errorf("Failed to write DB temp file for %s: %v", driveRoot, err)
			continue
		}

		if err := os.Rename(tmpPath, dbPath); err != nil {
			db.logger.Errorf("Failed to rename DB file for %s: %v", driveRoot, err)
			continue
		}
		db.dirty[driveRoot] = false
	}
}
