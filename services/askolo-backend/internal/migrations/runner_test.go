package migrations

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

func TestPublishCompatibilityGateAllowsOnlyReviewedHistoricalDataMigrations(t *testing.T) {
	migrations, err := Load(SQL)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if err := ValidatePublishMigrationCompatibility(migrations); err != nil {
		t.Fatalf("current migrations are not Publish-compatible: %v", err)
	}

	unreviewed := append(append([]Migration(nil), migrations...), Migration{
		Version: len(migrations),
		Name:    "0007_unreviewed_seed",
		SQL:     "INSERT INTO users (id) VALUES ('seed');",
		SHA256:  "unreviewed",
	})
	if err := ValidatePublishMigrationCompatibility(unreviewed); err == nil ||
		!strings.Contains(err.Error(), "without a reviewed Publish compatibility exception") {
		t.Fatalf("unreviewed data migration error = %v, want compatibility block", err)
	}
}

func TestMigrationSQLDataMutationScanner(t *testing.T) {
	tests := []struct {
		name string
		sql  string
		want bool
	}{
		{"insert", "INSERT INTO users (id) VALUES ('x')", true},
		{"update in CTE", "WITH changed AS (UPDATE users SET id = 'x' RETURNING id) SELECT * FROM changed", true},
		{"update inside DO block", "DO $$ BEGIN UPDATE users SET id = 'x'; END $$;", true},
		{"create table as select", "CREATE TABLE snapshot AS SELECT id FROM users;", true},
		{"select into", "SELECT id INTO snapshot FROM users;", true},
		{"materialized view seed", "CREATE MATERIALIZED VIEW snapshot AS SELECT id FROM users;", true},
		{"comments and string values", "-- DELETE FROM users\nSELECT 'UPDATE users'; /* INSERT INTO users */", false},
		{"foreign key actions", "REFERENCES users(id) ON DELETE CASCADE ON UPDATE NO ACTION", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := containsDataMutation(test.sql); got != test.want {
				t.Fatalf("containsDataMutation() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestBaselineContractPinsArchiveAndMigrationChecksums(t *testing.T) {
	if archiveBaselineContract.Version != 1 {
		t.Fatalf("contract version = %d, want 1", archiveBaselineContract.Version)
	}
	if len(archiveBaselineContract.ArchiveCommit) != 40 {
		t.Fatalf("archive commit is not a full Git object ID: %q", archiveBaselineContract.ArchiveCommit)
	}
	if archiveBaselineContract.PostgresMajorVersion != 16 {
		t.Fatalf("contract PostgreSQL major = %d, want 16", archiveBaselineContract.PostgresMajorVersion)
	}
	if !strings.HasPrefix(archiveBaselineContract.Fingerprint, fingerprintPrefix) {
		t.Fatalf("contract fingerprint %q is not versioned with %q", archiveBaselineContract.Fingerprint, fingerprintPrefix)
	}
	manifestPath := filepath.Join("..", "..", "..", "..", "scripts", "database-history-archive.manifest")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read protected archive manifest: %v", err)
	}
	if !strings.Contains(string(manifest), "archive_commit="+archiveBaselineContract.ArchiveCommit+"\n") {
		t.Fatalf("contract archive commit %s does not match the protected archive manifest", archiveBaselineContract.ArchiveCommit)
	}
	migrations, err := Load(SQL)
	if err != nil {
		t.Fatalf("load embedded migrations: %v", err)
	}
	if archiveBaselineContract.LastMigrationVersion != 1 {
		t.Fatalf("adoptable migration prefix ends at %d, want 1", archiveBaselineContract.LastMigrationVersion)
	}
	if len(archiveBaselineContract.MigrationChecksums) != archiveBaselineContract.LastMigrationVersion+1 {
		t.Fatalf("pinned checksum count = %d, want %d", len(archiveBaselineContract.MigrationChecksums), archiveBaselineContract.LastMigrationVersion+1)
	}
	for version, expected := range archiveBaselineContract.MigrationChecksums {
		if migrations[version].Name != expected[0] || migrations[version].SHA256 != expected[1] {
			t.Errorf("migration %d = %s/%s, want pinned %s/%s",
				version, migrations[version].Name, migrations[version].SHA256, expected[0], expected[1])
		}
	}
}

func TestNormalizeSchemaReferenceRemovesOnlyTargetQualification(t *testing.T) {
	got := normalizeSchemaReference(`"archive_copy".users nextval('"archive_copy".users_id_seq'::regclass)`, "archive_copy")
	want := `users nextval('users_id_seq'::regclass)`
	if got != want {
		t.Fatalf("normalized schema reference = %q, want %q", got, want)
	}
}
