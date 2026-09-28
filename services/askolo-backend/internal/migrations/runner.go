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
	migrationSet, err := Load(SQL)
	if err != nil {
		return err
	}
	return run(ctx, pool, migrationSet)
}

func run(ctx context.Context, pool *pgxpool.Pool, migrations []Migration) error {
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
	if len(applied) > 0 && storedFingerprint != "" {
		var current string
		if strings.HasPrefix(storedFingerprint, fingerprintPrefix) {
			current, err = fingerprint(ctx, tx)
		} else if strings.HasPrefix(storedFingerprint, "inventory-") {
			return fmt.Errorf("unsupported schema fingerprint version %q", strings.SplitN(storedFingerprint, ":", 2)[0])
		} else {
			// Histories created before inventory-v1 used the legacy,
			// narrower columns/constraints/indexes fingerprint. It remains
			// only to let those histories advance; adoption never trusts it.
			current, err = legacyFingerprint(ctx, tx)
		}
		if err != nil {
			return err
		}
		if current != storedFingerprint {
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

// baselineContract pins the archived database state after the legacy and auth
// migrations. The AI credit contract migration is deliberately not adoptable.
type baselineContract struct {
	Version              int
	ArchiveCommit        string
	PostgresMajorVersion int
	LastMigrationVersion int
	Fingerprint          string
	MigrationChecksums   [][2]string
}

var archiveBaselineContract = baselineContract{
	Version:              1,
	ArchiveCommit:        "1ce174d19cc62d03b27f55bd54ceef3fa4c60dea",
	PostgresMajorVersion: 16,
	LastMigrationVersion: 1,
	// Derived from the independently verified archive-vs-Go schema comparison
	// and pinned with migration checksums and the protected archive commit.
	Fingerprint: "inventory-v1:f712d478e83b68d96e26defd3f8456be3e0ea8a95ae961948a431555eff21660",
	MigrationChecksums: [][2]string{
		{"0000_legacy_schema", "be3362f2caa0dc8beed2c19fc8ef4f1ea44e1d73548dd11e11a773918fe3bfb3"},
		{"0001_auth_schema", "6c34db4a8e1c675215e829bbc3030c439bbe4049ba1cc642a4289cc868600b45"},
	},
}

type workspaceBaselineContract struct {
	Version              int
	MigrationVersion     int
	MigrationName        string
	MigrationChecksum    string
	LogicalFingerprint   string
	WorkspaceFingerprint string
}

// The archive contains the reviewed workspace schema, but migrations 0000 and
// 0001 predate its inclusion in the executable runner. This separate contract
// recognizes that exact extension without changing the historical archive
// baseline or the persisted inventory-v1 fingerprint format.
var archivedWorkspaceBaselineContract = workspaceBaselineContract{
	Version:              1,
	MigrationVersion:     3,
	MigrationName:        "0003_workspace_authorization",
	MigrationChecksum:    "1f2c55fa47c2fb029ab0467e200cc4a5fb567d790ffcacd14379967b0022e9e2",
	LogicalFingerprint:   "inventory-v2:55b46e72ad129df300df6983ee826b162f4052003999d77b6470037cf2fc3ace",
	WorkspaceFingerprint: "workspace-inventory-v1:3d9df8cfd0eb7e3804fe99dc2bef3819080a9781cd598570971bcefbc5dc67ae",
}

const fingerprintPrefix = "inventory-v1:"
const logicalFingerprintPrefix = "inventory-v2:"
const workspaceFingerprintPrefix = "workspace-inventory-v1:"

func ValidateBaseline(ctx context.Context, tx pgx.Tx) error {
	if archiveBaselineContract.Fingerprint == "" {
		return errors.New("existing-schema adoption refused: pinned archive contract fingerprint is not configured")
	}
	migrations, err := Load(SQL)
	if err != nil {
		return fmt.Errorf("load pinned baseline migrations: %w", err)
	}
	last := archiveBaselineContract.LastMigrationVersion
	if last < 0 || last >= len(migrations) || last+1 != len(archiveBaselineContract.MigrationChecksums) {
		return errors.New("existing-schema adoption refused: pinned archive contract migration range is invalid")
	}
	for version := 0; version <= last; version++ {
		expected := archiveBaselineContract.MigrationChecksums[version]
		actual := migrations[version]
		if expected[0] != actual.Name || expected[1] != actual.SHA256 {
			return fmt.Errorf("existing-schema adoption refused: baseline migration %04d differs from pinned contract", version)
		}
	}
	var schemaName string
	if err := tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&schemaName); err != nil {
		return fmt.Errorf("resolve target schema: %w", err)
	}
	if schemaName == "" {
		return errors.New("existing-schema adoption refused: no current schema is selected")
	}
	var serverVersion int
	if err := tx.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&serverVersion); err != nil {
		return fmt.Errorf("resolve PostgreSQL version: %w", err)
	}
	if serverVersion/10000 != archiveBaselineContract.PostgresMajorVersion {
		return fmt.Errorf("existing-schema adoption refused: pinned inventory contract requires PostgreSQL %d.x, target is %s", archiveBaselineContract.PostgresMajorVersion, strconv.Itoa(serverVersion/10000))
	}
	var eventTriggerExists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_event_trigger)`).Scan(&eventTriggerExists); err != nil {
		return fmt.Errorf("check database-wide DDL triggers: %w", err)
	}
	if eventTriggerExists {
		return errors.New("existing-schema adoption refused: database-wide event triggers are outside the pinned contract")
	}
	var historyExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM pg_class c
			JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = current_schema()
			  AND c.relname = 'askolo_schema_migrations'
		)`).Scan(&historyExists); err != nil {
		return fmt.Errorf("check existing migration history: %w", err)
	}
	if historyExists {
		return errors.New("existing-schema adoption refused: migration history already exists")
	}
	actual, err := fingerprint(ctx, tx)
	if err != nil {
		return fmt.Errorf("inventory target schema: %w", err)
	}
	if actual == archiveBaselineContract.Fingerprint {
		return nil
	}
	return validateArchivedWorkspaceBaseline(ctx, tx, migrations)
}

func validateArchivedWorkspaceBaseline(ctx context.Context, tx pgx.Tx, migrations []Migration) error {
	contract := archivedWorkspaceBaselineContract
	if contract.LogicalFingerprint == "" || contract.WorkspaceFingerprint == "" {
		return errors.New("existing-schema adoption refused: verified workspace baseline fingerprints are not configured")
	}
	if contract.MigrationVersion < 0 || contract.MigrationVersion >= len(migrations) {
		return errors.New("existing-schema adoption refused: workspace migration contract version is invalid")
	}
	migration := migrations[contract.MigrationVersion]
	if migration.Name != contract.MigrationName || migration.SHA256 != contract.MigrationChecksum {
		return fmt.Errorf("existing-schema adoption refused: workspace migration %04d differs from pinned contract", contract.MigrationVersion)
	}
	logical, err := logicalFingerprint(ctx, tx)
	if err != nil {
		return fmt.Errorf("inventory target schema under the logical-column contract: %w", err)
	}
	if logical != contract.LogicalFingerprint {
		return fmt.Errorf("existing-schema adoption refused: schema does not match archived workspace contract v%d and does not match pinned archive contract (got %s)", contract.Version, logical)
	}
	workspace, err := workspaceFingerprint(ctx, tx)
	if err != nil {
		return fmt.Errorf("inventory workspace authorization schema: %w", err)
	}
	if workspace != contract.WorkspaceFingerprint {
		return fmt.Errorf("existing-schema adoption refused: workspace schema does not match archived contract v%d (got %s)", contract.Version, workspace)
	}
	return nil
}

// AdoptBaseline records the verified legacy/auth migration prefix without
// replaying non-idempotent DDL. The caller owns transaction boundaries; ledger
// creation and both history rows therefore commit atomically with validation.
func AdoptBaseline(ctx context.Context, tx pgx.Tx) error {
	if err := ValidateBaseline(ctx, tx); err != nil {
		return err
	}
	migrations, err := Load(SQL)
	if err != nil {
		return err
	}
	fp, err := fingerprint(ctx, tx)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE askolo_schema_migrations (
		version integer PRIMARY KEY,
		name text NOT NULL,
		checksum text NOT NULL,
		applied_at timestamptz NOT NULL DEFAULT now(),
		schema_fingerprint text NOT NULL
	)`); err != nil {
		return fmt.Errorf("create migration ledger after baseline validation: %w", err)
	}
	for version := 0; version <= archiveBaselineContract.LastMigrationVersion; version++ {
		migration := migrations[version]
		if _, err := tx.Exec(ctx, `
			INSERT INTO askolo_schema_migrations(version, name, checksum, schema_fingerprint)
			VALUES($1, $2, $3, $4)`,
			migration.Version, migration.Name, migration.SHA256, fp); err != nil {
			return fmt.Errorf("record adopted baseline migration %04d: %w", version, err)
		}
	}
	return nil
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
	if m.Name == "0002_ai_credit_contract" {
		// The migration source is checksum-protected history. Its constraint
		// guards query pg_constraint by name alone, but constraint names can
		// repeat across schemas and tables. Scope those lookups to the target
		// relation without changing the recorded migration checksum.
		sqlText, err = scopeAICreditConstraintLookups(sqlText)
		if err != nil {
			return fmt.Errorf("prepare migration %04d: %w", m.Version, err)
		}
	}
	if _, err = tx.Exec(ctx, sqlText); err != nil {
		return fmt.Errorf("migration %04d failed (rolled back): %w", m.Version, err)
	}
	if m.Name == archivedWorkspaceBaselineContract.MigrationName {
		actualWorkspace, verifyErr := workspaceFingerprint(ctx, tx)
		if verifyErr != nil {
			return fmt.Errorf("migration %04d workspace verification failed (rolled back): %w", m.Version, verifyErr)
		}
		if actualWorkspace != archivedWorkspaceBaselineContract.WorkspaceFingerprint {
			return fmt.Errorf("migration %04d workspace schema differs from pinned contract (rolled back): got %s", m.Version, actualWorkspace)
		}
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

func scopeAICreditConstraintLookups(sqlText string) (string, error) {
	constraints := []struct {
		table string
		name  string
	}{
		{table: "ai_credit_grants", name: "ai_credit_grants_amount_positive"},
		{table: "ai_credit_adjustments", name: "ai_credit_adjustments_amount_nonzero"},
		{table: "ai_credit_reservations", name: "ai_credit_reservations_reserved_nonnegative"},
		{table: "ai_credit_reservations", name: "ai_credit_reservations_settled_nonnegative"},
		{table: "ai_credit_reservations", name: "ai_credit_reservations_refunded_nonnegative"},
		{table: "ai_credit_reservations", name: "ai_credit_reservations_unit_rate_positive"},
		{table: "ai_credit_reservations", name: "ai_credit_reservations_max_nonnegative"},
		{table: "ai_credit_reservations", name: "ai_credit_reservations_duration_nonnegative"},
		{table: "ai_credit_reservations", name: "ai_credit_reservations_connected_duration_nonnegative"},
		{table: "ai_credit_reservation_events", name: "ai_credit_reservation_events_credits_nonnegative"},
		{table: "ai_credit_reservation_events", name: "ai_credit_reservation_events_duration_nonnegative"},
		{table: "ai_provider_usage_evidence", name: "ai_provider_usage_evidence_duration_nonnegative"},
		{table: "ai_provider_usage_evidence", name: "ai_provider_usage_evidence_input_nonnegative"},
		{table: "ai_provider_usage_evidence", name: "ai_provider_usage_evidence_output_nonnegative"},
	}

	for _, constraint := range constraints {
		unscoped := fmt.Sprintf("WHERE conname='%s'", constraint.name)
		scoped := fmt.Sprintf(
			"WHERE conrelid=to_regclass('%s') AND conname='%s'",
			constraint.table,
			constraint.name,
		)
		if strings.Count(sqlText, unscoped) != 1 {
			return "", fmt.Errorf("expected exactly one unscoped lookup for constraint %q", constraint.name)
		}
		sqlText = strings.Replace(sqlText, unscoped, scoped, 1)
	}
	return sqlText, nil
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
	return inventoryFingerprint(ctx, tx, true, false, nil, fingerprintPrefix)
}

// logicalFingerprint is used only to verify archived schema adoption. It
// compares columns by table and name while retaining every other inventory
// property. Persisted runner fingerprints remain inventory-v1 and
// order-sensitive.
func logicalFingerprint(ctx context.Context, tx pgx.Tx) (string, error) {
	return inventoryFingerprint(ctx, tx, false, false, nil, logicalFingerprintPrefix)
}

func workspaceFingerprint(ctx context.Context, tx pgx.Tx) (string, error) {
	return inventoryFingerprint(ctx, tx, true, true, []string{
		"workspaces",
		"workspace_memberships",
		"authorization_resources",
	}, workspaceFingerprintPrefix)
}

func inventoryFingerprint(ctx context.Context, tx pgx.Tx, includeColumnOrdinal, filterWorkspace bool, workspaceTables []string, prefix string) (string, error) {
	var schema string
	if err := tx.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		return "", err
	}
	if schema == "" {
		return "", errors.New("cannot fingerprint schema: no current schema is selected")
	}
	// Deparser output depends on search_path. Pin it to the target schema for
	// the duration of this transaction so the digest is stable across callers
	// whose connection strings add different fallback schemas.
	var searchPath string
	if err := tx.QueryRow(ctx, `SELECT set_config('search_path', quote_ident($1), true)`, schema).Scan(&searchPath); err != nil {
		return "", fmt.Errorf("pin schema inventory search_path: %w", err)
	}
	rows, err := tx.Query(ctx, schemaInventoryQuery, includeColumnOrdinal, filterWorkspace, workspaceTables)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	h := sha256.New()
	for rows.Next() {
		var kind, identity, definition string
		if err := rows.Scan(&kind, &identity, &definition); err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s\x00%s\n", kind, normalizeSchemaReference(identity+":"+definition, schema))
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(h.Sum(nil)), nil
}

func normalizeSchemaReference(definition, schema string) string {
	definition = strings.ReplaceAll(definition, quoteIdentifier(schema)+".", "")
	return strings.ReplaceAll(definition, schema+".", "")
}

// Histories from the original runner used this narrower, unversioned digest.
// It remains solely as a compatibility check for existing ledgers; it is
// never accepted as evidence for schema adoption.
func legacyFingerprint(ctx context.Context, tx pgx.Tx) (string, error) {
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

func quoteIdentifier(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

const schemaInventoryQuery = `
WITH target AS (
	SELECT oid FROM pg_namespace WHERE nspname = current_schema()
), inventory(kind, identity, definition) AS (
	SELECT 'relation', c.relname,
		concat_ws(':', c.relkind, c.relpersistence, c.relrowsecurity,
			c.relforcerowsecurity, c.relreplident, COALESCE(array_to_string(c.reloptions, ','), ''),
			COALESCE(am.amname, ''))
	FROM pg_class c
	JOIN target ON target.oid = c.relnamespace
	LEFT JOIN pg_am am ON am.oid = c.relam
	WHERE c.relkind IN ('r', 'p', 'v', 'm', 'f', 'c')
	  AND c.relname <> 'askolo_schema_migrations'
	UNION ALL
	SELECT 'column',
		CASE WHEN $1::boolean
			THEN c.relname || '.' || a.attnum::text || '.' || a.attname
			ELSE c.relname || '.' || a.attname
		END,
		concat_ws(':', format_type(a.atttypid, a.atttypmod), a.attnotnull,
			a.attidentity, a.attgenerated, COALESCE(pg_get_expr(d.adbin, d.adrelid, true), ''))
	FROM pg_attribute a
	JOIN pg_class c ON c.oid = a.attrelid
	JOIN target ON target.oid = c.relnamespace
	LEFT JOIN pg_attrdef d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
	WHERE c.relkind IN ('r', 'p', 'v', 'm', 'f', 'c')
	  AND c.relname <> 'askolo_schema_migrations'
	  AND a.attnum > 0 AND NOT a.attisdropped
	UNION ALL
	SELECT 'constraint', c.relname || '.' || con.conname,
		concat_ws(':', con.contype, con.condeferrable, con.condeferred,
			con.convalidated, con.conislocal, con.coninhcount, con.connoinherit,
			pg_get_constraintdef(con.oid, true))
	FROM pg_constraint con
	JOIN pg_class c ON c.oid = con.conrelid
	JOIN target ON target.oid = c.relnamespace
	WHERE c.relname <> 'askolo_schema_migrations'
	UNION ALL
	SELECT 'domain_constraint', t.typname || '.' || con.conname,
		concat_ws(':', con.condeferrable, con.condeferred, con.convalidated,
			pg_get_constraintdef(con.oid, true))
	FROM pg_constraint con
	JOIN pg_type t ON t.oid = con.contypid
	JOIN target ON target.oid = t.typnamespace
	UNION ALL
	SELECT 'index', table_class.relname || '.' || index_class.relname,
		concat_ws(':', idx.indisunique, idx.indisprimary, idx.indisexclusion,
			idx.indisvalid, idx.indisready, idx.indislive,
			pg_get_indexdef(idx.indexrelid, 0, true))
	FROM pg_index idx
	JOIN pg_class index_class ON index_class.oid = idx.indexrelid
	JOIN pg_class table_class ON table_class.oid = idx.indrelid
	JOIN target ON target.oid = table_class.relnamespace
	WHERE table_class.relname <> 'askolo_schema_migrations'
	UNION ALL
	SELECT 'sequence', c.relname,
		concat_ws(':', s.seqtypid::regtype::text, s.seqstart, s.seqincrement,
			s.seqmax, s.seqmin, s.seqcache, s.seqcycle,
			COALESCE(owner.table_name || '.' || owner.column_name || ':' || owner.deptype::text, ''))
	FROM pg_class c
	JOIN target ON target.oid = c.relnamespace
	JOIN pg_sequence s ON s.seqrelid = c.oid
	LEFT JOIN LATERAL (
		SELECT owner_class.relname AS table_name, owner_att.attname AS column_name, d.deptype
		FROM pg_depend d
		JOIN pg_class owner_class ON owner_class.oid = d.refobjid
		JOIN pg_attribute owner_att ON owner_att.attrelid = owner_class.oid AND owner_att.attnum = d.refobjsubid
		WHERE d.classid = 'pg_class'::regclass AND d.objid = c.oid
		  AND d.refclassid = 'pg_class'::regclass AND d.deptype IN ('a', 'i')
		ORDER BY owner_class.relname, owner_att.attname
		LIMIT 1
	) owner ON true
	WHERE c.relkind = 'S' AND c.relname <> 'askolo_schema_migrations'
	UNION ALL
	SELECT 'trigger', c.relname || '.' || t.tgname,
		concat_ws(':', t.tgenabled, (t.tgconstraint <> 0), pg_get_triggerdef(t.oid, true))
	FROM pg_trigger t
	JOIN pg_class c ON c.oid = t.tgrelid
	JOIN target ON target.oid = c.relnamespace
	WHERE NOT t.tgisinternal AND c.relname <> 'askolo_schema_migrations'
	UNION ALL
	SELECT 'policy', c.relname || '.' || p.polname,
		concat_ws(':', p.polcmd, p.polpermissive,
			(SELECT string_agg(CASE WHEN roles.role_oid = 0 THEN 'PUBLIC' ELSE quote_ident(r.rolname) END, ',' ORDER BY roles.role_oid)
			 FROM unnest(p.polroles) AS roles(role_oid)
			 LEFT JOIN pg_roles r ON r.oid = roles.role_oid),
			COALESCE(pg_get_expr(p.polqual, p.polrelid, true), ''),
			COALESCE(pg_get_expr(p.polwithcheck, p.polrelid, true), ''))
	FROM pg_policy p
	JOIN pg_class c ON c.oid = p.polrelid
	JOIN target ON target.oid = c.relnamespace
	WHERE c.relname <> 'askolo_schema_migrations'
	UNION ALL
	SELECT 'view', c.relname, pg_get_viewdef(c.oid, true)
	FROM pg_class c
	JOIN target ON target.oid = c.relnamespace
	WHERE c.relkind IN ('v', 'm') AND c.relname <> 'askolo_schema_migrations'
	UNION ALL
	SELECT 'rule', c.relname || '.' || r.rulename, pg_get_ruledef(r.oid, true)
	FROM pg_rewrite r
	JOIN pg_class c ON c.oid = r.ev_class
	JOIN target ON target.oid = c.relnamespace
	WHERE r.rulename <> '_RETURN' AND c.relname <> 'askolo_schema_migrations'
	UNION ALL
	SELECT 'partition', child.relname,
		parent.relname || ':' || child.relkind::text || ':' ||
		CASE WHEN child.relispartition THEN pg_get_expr(child.relpartbound, child.oid, true) ELSE '' END
	FROM pg_inherits i
	JOIN pg_class child ON child.oid = i.inhrelid
	JOIN pg_class parent ON parent.oid = i.inhparent
	JOIN target ON target.oid = child.relnamespace
	WHERE child.relname <> 'askolo_schema_migrations'
	UNION ALL
	SELECT 'partition_key', c.relname, pg_get_partkeydef(c.oid)
	FROM pg_class c
	JOIN target ON target.oid = c.relnamespace
	WHERE c.relkind = 'p' AND c.relname <> 'askolo_schema_migrations'
	UNION ALL
	SELECT 'type', t.typname,
		concat_ws(':', t.typtype, t.typcategory, t.typisdefined, t.typnotnull,
			COALESCE(t.typbasetype::regtype::text, ''),
			COALESCE(t.typdefault, ''), COALESCE(t.typcollation::regcollation::text, ''))
	FROM pg_type t
	JOIN target ON target.oid = t.typnamespace
	LEFT JOIN pg_class composite_type ON composite_type.oid = t.typrelid
	WHERE t.typisdefined
	  AND ((t.typrelid = 0 AND t.typelem = 0)
	       OR (t.typtype = 'c' AND composite_type.relkind = 'c'))
	UNION ALL
	SELECT 'enum', t.typname || '.' || e.enumlabel, e.enumsortorder::text
	FROM pg_enum e
	JOIN pg_type t ON t.oid = e.enumtypid
	JOIN target ON target.oid = t.typnamespace
	UNION ALL
	SELECT 'range', t.typname,
		concat_ws(':', r.rngsubtype::regtype::text, r.rngcollation::regcollation::text,
			r.rngsubopc::regclass::text, r.rngcanonical::regproc::text,
			r.rngsubdiff::regproc::text, r.rngmultitypid::regtype::text)
	FROM pg_range r
	JOIN pg_type t ON t.oid = r.rngtypid
	JOIN target ON target.oid = t.typnamespace
	UNION ALL
	SELECT 'routine', p.proname || '(' || pg_get_function_identity_arguments(p.oid) || ')',
		pg_get_functiondef(p.oid)
	FROM pg_proc p
	JOIN target ON target.oid = p.pronamespace
	WHERE p.prokind IN ('f', 'p', 'w')
	UNION ALL
	SELECT 'aggregate', p.proname || '(' || pg_get_function_identity_arguments(p.oid) || ')',
		concat_ws(':', a.aggkind, a.aggnumdirectargs,
			a.aggtransfn::regprocedure::text, a.aggtranstype::regtype::text, a.aggtransspace,
			a.aggfinalfn::regprocedure::text, a.aggfinalextra, a.aggfinalmodify,
			a.aggcombinefn::regprocedure::text, a.aggserialfn::regprocedure::text,
			a.aggdeserialfn::regprocedure::text, a.aggmtransfn::regprocedure::text,
			a.aggmtranstype::regtype::text, a.aggmtransspace,
			a.aggminvtransfn::regprocedure::text, a.aggmfinalfn::regprocedure::text,
			a.aggmfinalextra, a.aggmfinalmodify, a.aggsortop::regoperator::text,
			a.agginitval, a.aggminitval)
	FROM pg_aggregate a
	JOIN pg_proc p ON p.oid = a.aggfnoid
	JOIN target ON target.oid = p.pronamespace
	UNION ALL
	SELECT 'operator', o.oprname || ':' || o.oprkind::text || ':' ||
			COALESCE(o.oprleft::regtype::text, '') || ':' || COALESCE(o.oprright::regtype::text, ''),
		concat_ws(':', o.oprresult::regtype::text, o.oprcode::regprocedure::text,
			o.oprrest::regprocedure::text, o.oprjoin::regprocedure::text)
	FROM pg_operator o
	JOIN target ON target.oid = o.oprnamespace
	UNION ALL
	SELECT 'collation', c.collname,
		concat_ws(':', c.collprovider, c.collcollate, c.collctype, c.collisdeterministic, c.collversion)
	FROM pg_collation c
	JOIN target ON target.oid = c.collnamespace
	UNION ALL
	SELECT 'conversion', c.conname,
		concat_ws(':', c.conforencoding, c.contoencoding, c.conproc::regprocedure::text, c.condefault)
	FROM pg_conversion c
	JOIN target ON target.oid = c.connamespace
	UNION ALL
	SELECT 'text_search_dictionary', d.dictname,
		concat_ws(':', t.tmplname, d.dictinitoption)
	FROM pg_ts_dict d
	JOIN pg_ts_template t ON t.oid = d.dicttemplate
	JOIN target ON target.oid = d.dictnamespace
	UNION ALL
	SELECT 'text_search_config', c.cfgname,
		concat_ws(':', parser.prsname,
			(SELECT string_agg(m.maptokentype::text || ':' || COALESCE(d.dictname, ''), ',' ORDER BY m.maptokentype, d.dictname)
			 FROM pg_ts_config_map m
			 LEFT JOIN pg_ts_dict d ON d.oid = m.mapdict
			 WHERE m.mapcfg = c.oid))
	FROM pg_ts_config c
	JOIN pg_ts_parser parser ON parser.oid = c.cfgparser
	JOIN target ON target.oid = c.cfgnamespace
	UNION ALL
	SELECT 'text_search_parser', p.prsname,
		concat_ws(':', p.prsstart::regprocedure::text, p.prstoken::regprocedure::text,
			p.prsend::regprocedure::text, p.prsheadline::regprocedure::text,
			p.prslextype::regprocedure::text)
	FROM pg_ts_parser p
	JOIN target ON target.oid = p.prsnamespace
	UNION ALL
	SELECT 'text_search_template', t.tmplname,
		concat_ws(':', t.tmplinit::regprocedure::text, t.tmpllexize::regprocedure::text)
	FROM pg_ts_template t
	JOIN target ON target.oid = t.tmplnamespace
	UNION ALL
	SELECT 'transform', t.trftype::regtype::text || ':' || l.lanname,
		concat_ws(':', t.trffromsql::regprocedure::text, t.trftosql::regprocedure::text)
	FROM pg_transform t
	JOIN pg_type transformed_type ON transformed_type.oid = t.trftype
	JOIN pg_language l ON l.oid = t.trflang
	JOIN target ON target.oid = transformed_type.typnamespace
	UNION ALL
	SELECT 'opclass', c.opcname,
		concat_ws(':', a.amname, c.opcintype::regtype::text, c.opcdefault, c.opckeytype::regtype::text)
	FROM pg_opclass c
	JOIN pg_am a ON a.oid = c.opcmethod
	JOIN target ON target.oid = c.opcnamespace
	UNION ALL
	SELECT 'opfamily', f.opfname, a.amname
	FROM pg_opfamily f
	JOIN pg_am a ON a.oid = f.opfmethod
	JOIN target ON target.oid = f.opfnamespace
	UNION ALL
	SELECT 'opfamily_operator', f.opfname || ':' || m.amopstrategy::text || ':' || m.amoppurpose::text,
		concat_ws(':', m.amoplefttype::regtype::text, m.amoprighttype::regtype::text,
			m.amopopr::regoperator::text, m.amopsortfamily::regclass::text)
	FROM pg_amop m
	JOIN pg_opfamily f ON f.oid = m.amopfamily
	JOIN target ON target.oid = f.opfnamespace
	UNION ALL
	SELECT 'opfamily_function', f.opfname || ':' || m.amprocnum::text,
		concat_ws(':', m.amproclefttype::regtype::text, m.amprocrighttype::regtype::text,
			m.amproc::regprocedure::text)
	FROM pg_amproc m
	JOIN pg_opfamily f ON f.oid = m.amprocfamily
	JOIN target ON target.oid = f.opfnamespace
	UNION ALL
	SELECT 'extension', e.extname, e.extversion
	FROM pg_extension e
	JOIN target ON target.oid = e.extnamespace
	UNION ALL
	SELECT 'extended_statistics', c.relname || '.' || s.stxname, pg_get_statisticsobjdef(s.oid)
	FROM pg_statistic_ext s
	JOIN pg_class c ON c.oid = s.stxrelid
	JOIN target ON target.oid = c.relnamespace
)
SELECT kind, identity, definition
FROM inventory
WHERE NOT $2::boolean
   OR (kind = 'relation' AND identity = ANY($3::text[]))
   OR (kind IN ('column', 'constraint', 'index', 'trigger', 'policy', 'rule', 'partition', 'partition_key')
       AND split_part(identity, '.', 1) = ANY($3::text[]))
ORDER BY kind, identity, definition`
