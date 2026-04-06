package database

const createTableSQL = `
CREATE TABLE IF NOT EXISTS conversions (
    file_hash               TEXT    NOT NULL,
    drive_root              TEXT    NOT NULL,
    source_path             TEXT,
    original_size           INTEGER NOT NULL,
    converted_size          INTEGER,
    output_path             TEXT,
    note                    TEXT,
    error                   TEXT,
    source_codec            TEXT,
    source_container        TEXT,
    width                   INTEGER,
    height                  INTEGER,
    duration_secs           REAL,
    converted_at            TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    conversion_duration_secs REAL,
    PRIMARY KEY (file_hash, drive_root)
);
CREATE INDEX IF NOT EXISTS idx_conversions_drive ON conversions(drive_root);
CREATE INDEX IF NOT EXISTS idx_conversions_error ON conversions(error) WHERE error IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_conversions_note ON conversions(note) WHERE note IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_conversions_converted_at ON conversions(converted_at);
`
