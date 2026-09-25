package migrations

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
)

func TestLoadOrdersAndHashesMigrations(t *testing.T) {
	files := fstest.MapFS{
		"sql/0001_second.sql": {Data: []byte("second")},
		"sql/0000_first.sql":  {Data: []byte("first")},
	}

	got, err := Load(files)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(got) != 2 || got[0].Version != 0 || got[1].Version != 1 {
		t.Fatalf("Load() order = %#v, want versions 0, 1", got)
	}
	sum := sha256.Sum256([]byte("first"))
	if got[0].SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("checksum = %q, want %x", got[0].SHA256, sum)
	}
}

func TestLoadRejectsMalformedMigrationFiles(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{"duplicate", "0000_duplicate.sql"},
		{"gap", "0002_gap.sql"},
		{"malformed version", "abc_bad.sql"},
		{"unexpected file", "0001_notes.txt"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			files := fstest.MapFS{"sql/0000_first.sql": {Data: []byte("first")}}
			files["sql/"+test.file] = &fstest.MapFile{Data: []byte("other")}
			if _, err := Load(files); err == nil {
				t.Fatal("Load() succeeded; want validation error")
			}
		})
	}
}

func TestLoadRejectsMissingSQLDirectory(t *testing.T) {
	_, err := Load(fstest.MapFS{})
	if err == nil {
		t.Fatal("Load() succeeded; want missing directory error")
	}
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	t.Fatalf("Load() error = %v, want fs.ErrNotExist", err)
}
