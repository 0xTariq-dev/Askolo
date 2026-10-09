package testdb

import (
	"os"
	"strings"
	"testing"
)

// RequireDisposable refuses to run a database-backed integration test unless
// the caller explicitly marks its target as disposable. This prevents an
// ambient ASKOLO_TEST_DATABASE_URL from turning a local test run into writes
// against a shared Development or Production database.
func RequireDisposable(t *testing.T, databaseURL string) {
	t.Helper()
	if strings.TrimSpace(databaseURL) == "" {
		t.Fatal("integration test database URL is empty")
	}
	if strings.TrimSpace(os.Getenv("ASKOLO_TEST_DATABASE_DISPOSABLE")) != "yes" {
		t.Fatal("refusing database integration test: set ASKOLO_TEST_DATABASE_DISPOSABLE=yes only for a disposable database")
	}
	if appURL := strings.TrimSpace(os.Getenv("DATABASE_URL")); appURL != "" && appURL == strings.TrimSpace(databaseURL) {
		t.Fatal("refusing database integration test because ASKOLO_TEST_DATABASE_URL matches DATABASE_URL")
	}
}
