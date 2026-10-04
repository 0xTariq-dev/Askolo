package migrations

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// publishSchemaFingerprint is the inventory produced by the reviewed migration
// chain on PostgreSQL 16. It deliberately excludes the Go runner's ledger.
// Update it only after validating the new inventory in the disposable migration
// test database.
const publishSchemaFingerprint = "inventory-v1:1ab9dc8eaba4c336acf3aad2c9dd2908b17095723f158a19cd3683eace5459ef"

type approvedPublishDataMigration struct {
	name     string
	checksum string
}

// Existing seed-data migrations are frozen compatibility exceptions. New
// migrations that change rows require an explicitly reviewed production data
// path that is not currently established for Replit Publish.
var approvedPublishDataMigrations = map[int]approvedPublishDataMigration{
	4: {
		name:     "0004_ai_credit_policy",
		checksum: "f51f64bc2caf84b7c12d61bb51aa133c37f575c21dae462416f313bc238cf895",
	},
	5: {
		name:     "0005_guarded_assistant_runtime",
		checksum: "6f95a74805fa040ef26079284eed55a33665f65d4a93924dd91930cdf859502d",
	},
	8: {
		name:     "0008_usd_micro_ledger",
		checksum: "a09d959a24986ff386279dfeddfb936aba9971e058c7cad904d7d97f9039b7a5",
	},
}

// ValidatePublishMigrationCompatibility rejects unreviewed migration DML.
// Structural SQL may pass this check, but this does not claim that Replit
// Publish transfers migration-file data or runner ledger rows.
func ValidatePublishMigrationCompatibility(migrations []Migration) error {
	for _, migration := range migrations {
		if !containsDataMutation(migration.SQL) {
			continue
		}
		approved, ok := approvedPublishDataMigrations[migration.Version]
		if !ok || approved.name != migration.Name || approved.checksum != migration.SHA256 {
			return fmt.Errorf(
				"migration %04d (%s) contains data-changing SQL without a reviewed Publish compatibility exception",
				migration.Version,
				migration.Name,
			)
		}
	}
	return nil
}

// ValidateProductionSchemaCompatibility checks the managed schema against the
// reviewed structural inventory and verifies the seed rows currently required
// by the assistant. It is read-only and does not require or create a Go ledger.
func ValidateProductionSchemaCompatibility(ctx context.Context, pool *pgxpool.Pool) error {
	if pool == nil {
		return errors.New("production schema compatibility unavailable: database is not configured")
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire production schema compatibility connection: %w", err)
	}
	defer conn.Release()

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.RepeatableRead,
		AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return fmt.Errorf("begin production schema compatibility check: %w", err)
	}
	defer tx.Rollback(ctx)

	currentFingerprint, err := fingerprint(ctx, tx)
	if err != nil {
		return fmt.Errorf("inventory production schema: %w", err)
	}
	if currentFingerprint != publishSchemaFingerprint {
		return fmt.Errorf(
			"managed production schema inventory mismatch: expected %s, current %s",
			publishSchemaFingerprint,
			currentFingerprint,
		)
	}

	var requiredPolicyRowsPresent bool
	if err := tx.QueryRow(ctx, `
		SELECT
			EXISTS (
				SELECT 1 FROM ai_credit_policy_versions WHERE version = 1
			)
			AND EXISTS (
				SELECT 1
				FROM ai_credit_policy_versions AS current_policy
				WHERE current_policy.version = (
					SELECT MAX(version) FROM ai_credit_policy_versions
				)
				  AND current_policy.operation_weights ? 'assistant'
			)`).Scan(&requiredPolicyRowsPresent); err != nil {
		return fmt.Errorf("check production assistant policy data: %w", err)
	}
	if !requiredPolicyRowsPresent {
		return errors.New("production assistant policy seed data is incomplete")
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("finish production schema compatibility check: %w", err)
	}
	return nil
}

func containsDataMutation(sql string) bool {
	tokens := migrationSQLTokens(sql)
	for index, token := range tokens {
		switch token {
		case "INSERT", "MERGE", "TRUNCATE", "COPY", "REFRESH", "CALL", "EXECUTE":
			return true
		case "UPDATE", "DELETE":
			// These tokens are also used in PostgreSQL foreign-key clauses.
			if index > 0 && tokens[index-1] == "ON" {
				continue
			}
			return true
		case "CREATE":
			end := index + 1
			for end < len(tokens) && tokens[end] != ";" && end <= index+8 {
				if tokens[end] == "MATERIALIZED" {
					return true
				}
				if tokens[end] == "TABLE" {
					for tableEnd := end + 1; tableEnd < len(tokens) && tokens[tableEnd] != ";" && tableEnd <= end+5; tableEnd++ {
						if tokens[tableEnd] == "AS" &&
							tableEnd+1 < len(tokens) &&
							tokens[tableEnd+1] == "SELECT" {
							return true
						}
					}
				}
				end++
			}
		case "SELECT":
			for end := index + 1; end < len(tokens) && tokens[end] != ";"; end++ {
				if tokens[end] == "INTO" {
					return true
				}
			}
		}
	}
	return false
}

// migrationSQLTokens ignores comments, string values, and quoted identifiers,
// but scans dollar-quoted bodies because DO blocks can contain data statements.
func migrationSQLTokens(sql string) []string {
	var tokens []string
	for index := 0; index < len(sql); {
		switch {
		case index+1 < len(sql) && sql[index:index+2] == "--":
			index += 2
			for index < len(sql) && sql[index] != '\n' {
				index++
			}
		case index+1 < len(sql) && sql[index:index+2] == "/*":
			index = skipSQLBlockComment(sql, index)
		case sql[index] == '\'':
			index = skipSQLQuoted(sql, index, '\'')
		case sql[index] == '"':
			index = skipSQLQuoted(sql, index, '"')
		case sql[index] == '$':
			delimiterEnd := index + 1
			for delimiterEnd < len(sql) &&
				(sql[delimiterEnd] == '_' ||
					sql[delimiterEnd] >= 'a' && sql[delimiterEnd] <= 'z' ||
					sql[delimiterEnd] >= 'A' && sql[delimiterEnd] <= 'Z' ||
					sql[delimiterEnd] >= '0' && sql[delimiterEnd] <= '9') {
				delimiterEnd++
			}
			if delimiterEnd < len(sql) && sql[delimiterEnd] == '$' {
				delimiter := sql[index : delimiterEnd+1]
				bodyStart := delimiterEnd + 1
				if bodyEnd := strings.Index(sql[bodyStart:], delimiter); bodyEnd >= 0 {
					tokens = append(tokens, migrationSQLTokens(sql[bodyStart:bodyStart+bodyEnd])...)
					index = bodyStart + bodyEnd + len(delimiter)
				} else {
					index = delimiterEnd + 1
				}
			} else {
				index++
			}
		case isSQLWordStart(sql[index]):
			end := index + 1
			for end < len(sql) && isSQLWordPart(sql[end]) {
				end++
			}
			tokens = append(tokens, strings.ToUpper(sql[index:end]))
			index = end
		case sql[index] == ';':
			tokens = append(tokens, ";")
			index++
		default:
			index++
		}
	}
	return tokens
}

func skipSQLBlockComment(sql string, start int) int {
	index, depth := start+2, 1
	for index < len(sql) && depth > 0 {
		switch {
		case index+1 < len(sql) && sql[index:index+2] == "/*":
			depth++
			index += 2
		case index+1 < len(sql) && sql[index:index+2] == "*/":
			depth--
			index += 2
		default:
			index++
		}
	}
	return index
}

func skipSQLQuoted(sql string, start int, quote byte) int {
	index := start + 1
	for index < len(sql) {
		if sql[index] == quote {
			if index+1 < len(sql) && sql[index+1] == quote {
				index += 2
				continue
			}
			return index + 1
		}
		if quote == '\'' && sql[index] == '\\' && index+1 < len(sql) {
			index += 2
			continue
		}
		index++
	}
	return index
}

func isSQLWordStart(value byte) bool {
	return value == '_' || value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func isSQLWordPart(value byte) bool {
	return isSQLWordStart(value) || value >= '0' && value <= '9' || value == '$'
}
