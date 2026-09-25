package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SQL contains the authoritative, reviewed migrations. Files are deliberately
// numbered; the runner rejects gaps, duplicates, and non-migration files.
//
//go:embed sql/*.sql
var SQL embed.FS

type Migration struct {
	Version int
	Name    string
	SQL     string
	SHA256  string
}

func Load(filesystem fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(filesystem, "sql")
	if err != nil {
		return nil, err
	}
	var result []Migration
	seen := map[int]bool{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".sql" {
			return nil, fmt.Errorf("unexpected non-SQL migration file %q", entry.Name())
		}
		base := strings.TrimSuffix(entry.Name(), ".sql")
		parts := strings.SplitN(base, "_", 2)
		if len(parts) != 2 || len(parts[0]) != 4 {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		version, err := strconv.Atoi(parts[0])
		if err != nil || version < 0 {
			return nil, fmt.Errorf("invalid migration version %q", entry.Name())
		}
		if seen[version] {
			return nil, fmt.Errorf("duplicate migration version %04d", version)
		}
		seen[version] = true
		body, err := fs.ReadFile(filesystem, "sql/"+entry.Name())
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(body)
		result = append(result, Migration{version, base, string(body), hex.EncodeToString(sum[:])})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Version < result[j].Version })
	for i, m := range result {
		if m.Version != i {
			return nil, fmt.Errorf("migration sequence has gap or does not start at 0000 (found %04d)", m.Version)
		}
	}
	return result, nil
}

func Run(ctx context.Context, pool *pgxpool.Pool) error {
	migrations, err := Load(SQL)
	if err != nil {
		return err
	}
	if len(migrations) == 0 {
		return errors.New("no embedded migrations")
	}
	// Pin one session for the complete run. A session-level lock must not be
	// acquired through a pool connection that can be returned between
	// transactions, otherwise a second migrator can enter between migrations.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(7261736, 1)`); err != nil {
		return fmt.Errorf("migration lock: %w", err)
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(7261736, 1)`)
	if _, err = conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS askolo_schema_migrations (
		version integer PRIMARY KEY, name text NOT NULL, checksum text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now(),
		schema_fingerprint text NOT NULL
	)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT version, name, checksum, schema_fingerprint FROM askolo_schema_migrations ORDER BY version`)
	if err != nil {
		return err
	}
	applied := map[int]Migration{}
	var storedFingerprint string
	for rows.Next() {
		var v int
		var n, c, fp string
		if err := rows.Scan(&v, &n, &c, &fp); err != nil {
			rows.Close()
			return err
		}
		applied[v] = Migration{Version: v, Name: n, SHA256: c}
		storedFingerprint = fp
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	// History is a prefix, never an arbitrary set. Validate every recorded
	// row before executing any pending DDL.
	for v := 0; v < len(applied); v++ {
		m, ok := applied[v]
		if !ok {
			return fmt.Errorf("migration history missing version %04d", v)
		}
		if v >= len(migrations) || migrations[v].Name != m.Name || migrations[v].SHA256 != m.SHA256 {
			return fmt.Errorf("migration %04d checksum, name, or ordering mismatch", v)
		}
	}
	for v := range applied {
		if v < 0 || v >= len(migrations) {
			return fmt.Errorf("unexpected applied migration %04d", v)
		}
	}
	if len(applied) > 0 {
		current, err := fingerprint(ctx, tx)
		if err != nil {
			return err
		}
		if storedFingerprint != "" && current != storedFingerprint {
			return fmt.Errorf("managed schema drift detected: recorded %s, current %s", storedFingerprint, current)
		}
	}
	for _, m := range migrations {
		if _, ok := applied[m.Version]; ok {
			continue
		}
		// Each migration is its own transaction, including its history row.
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		if err := runOne(ctx, conn, m); err != nil {
			return err
		}
		tx, err = conn.Begin(ctx)
		if err != nil {
			return err
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7261736, 1)`); err != nil {
			return err
		}
	}
	if err := verifyNoUnexpected(ctx, tx, applied, migrations); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ValidateBaseline is intentionally conservative. Shape adoption is not
// automated until every archived object is represented by the managed
// contract; callers receive an explicit refusal rather than a guessed mark.
func ValidateBaseline(ctx context.Context, tx pgx.Tx) error {
	// Do not accept an operator-supplied digest as proof: it could simply be
	// computed from the wrong database. A pinned, independently generated
	// archive contract must be added before adoption is enabled.
	return errors.New("existing-schema adoption refused: no independently pinned archive baseline contract is available")
}

func AdoptBaseline(ctx context.Context, tx pgx.Tx) error {
	if err := ValidateBaseline(ctx, tx); err != nil {
		return err
	}
	migrations, err := Load(SQL)
	if err != nil {
		return err
	}
	if len(migrations) == 0 {
		return errors.New("no baseline migration")
	}
	fp, err := fingerprint(ctx, tx)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO askolo_schema_migrations(version,name,checksum,schema_fingerprint) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, migrations[0].Version, migrations[0].Name, migrations[0].SHA256, fp)
	return err
}

func runOne(ctx context.Context, conn *pgxpool.Conn, m Migration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// Historical Drizzle files qualify same-schema foreign keys with public.
	// Keep production behavior unchanged while allowing schema-isolated
	// disposable tests (and restores) to use their own search_path.
	sqlText := strings.ReplaceAll(m.SQL, `REFERENCES "public".`, `REFERENCES `)
	if _, err = tx.Exec(ctx, sqlText); err != nil {
		return fmt.Errorf("migration %04d failed (rolled back): %w", m.Version, err)
	}
	fp, err := fingerprint(ctx, tx)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO askolo_schema_migrations(version,name,checksum,schema_fingerprint) VALUES($1,$2,$3,$4)`, m.Version, m.Name, m.SHA256, fp); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func verifyNoUnexpected(ctx context.Context, tx pgx.Tx, applied map[int]Migration, expected []Migration) error {
	for v, m := range applied {
		if v < 0 || v >= len(expected) || expected[v].Name != m.Name {
			return fmt.Errorf("unexpected applied migration %04d", v)
		}
	}
	return nil
}

func fingerprint(ctx context.Context, tx pgx.Tx) (string, error) {
	rows, err := tx.Query(ctx, `
		SELECT 'column', table_schema||'.'||table_name||'.'||column_name||':'||
			ordinal_position||':'||data_type||':'||is_nullable||':'||COALESCE(column_default,'')
		FROM information_schema.columns
		WHERE table_schema = ANY(current_schemas(false)) AND table_name <> 'askolo_schema_migrations'
		UNION ALL
		SELECT 'constraint', n.nspname||'.'||c.relname||':'||con.conname||':'||pg_get_constraintdef(con.oid)
		FROM pg_constraint con JOIN pg_class c ON c.oid=con.conrelid
		JOIN pg_namespace n ON n.oid=c.relnamespace
		WHERE n.nspname = ANY(current_schemas(false)) AND c.relname <> 'askolo_schema_migrations'
		UNION ALL
		SELECT 'index', schemaname||'.'||tablename||':'||indexname||':'||indexdef
		FROM pg_indexes
		WHERE schemaname = ANY(current_schemas(false)) AND tablename <> 'askolo_schema_migrations'
		ORDER BY 1,2`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	h := sha256.New()
	for rows.Next() {
		var a, b string
		if err := rows.Scan(&a, &b); err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%s\n", a, b)
	}
	return hex.EncodeToString(h.Sum(nil)), rows.Err()
}
