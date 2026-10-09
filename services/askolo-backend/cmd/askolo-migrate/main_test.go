package main

import "testing"

func TestValidateDevelopmentReconciliationIdentity(t *testing.T) {
	tests := []struct {
		name         string
		databaseName string
		schemaName   string
		majorVersion int
		wantError    bool
	}{
		{
			name:         "verified Development database",
			databaseName: "heliumdb",
			schemaName:   "public",
			majorVersion: 16,
		},
		{
			name:         "Production database",
			databaseName: "neondb",
			schemaName:   "public",
			majorVersion: 16,
			wantError:    true,
		},
		{
			name:         "wrong schema",
			databaseName: "heliumdb",
			schemaName:   "public_v2",
			majorVersion: 16,
			wantError:    true,
		},
		{
			name:         "wrong PostgreSQL major",
			databaseName: "heliumdb",
			schemaName:   "public",
			majorVersion: 17,
			wantError:    true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateDevelopmentReconciliationIdentity(test.databaseName, test.schemaName, test.majorVersion)
			if (err != nil) != test.wantError {
				t.Fatalf("identity validation error = %v, wantError=%t", err, test.wantError)
			}
		})
	}
}

func TestValidateDevelopmentMigrationIdentity(t *testing.T) {
	valid := struct {
		databaseName, schemaName, databaseUser string
		majorVersion                           int
		systemIdentifier                       string
	}{
		databaseName: "heliumdb", schemaName: "public", databaseUser: "postgres",
		majorVersion: 16, systemIdentifier: verifiedDevelopmentSystemIdentifier,
	}
	if err := validateDevelopmentMigrationIdentity(
		valid.databaseName, valid.schemaName, valid.databaseUser, valid.majorVersion, valid.systemIdentifier,
	); err != nil {
		t.Fatalf("verified Development target rejected: %v", err)
	}

	tests := []struct {
		name                                   string
		databaseName, schemaName, databaseUser string
		majorVersion                           int
		systemIdentifier                       string
	}{
		{"wrong database", "neondb", "public", "postgres", 16, verifiedDevelopmentSystemIdentifier},
		{"wrong schema", "heliumdb", "migration_test", "postgres", 16, verifiedDevelopmentSystemIdentifier},
		{"wrong user", "heliumdb", "public", "app", 16, verifiedDevelopmentSystemIdentifier},
		{"wrong PostgreSQL version", "heliumdb", "public", "postgres", 15, verifiedDevelopmentSystemIdentifier},
		{"wrong cluster", "heliumdb", "public", "postgres", 16, "1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateDevelopmentMigrationIdentity(
				test.databaseName, test.schemaName, test.databaseUser, test.majorVersion, test.systemIdentifier,
			); err == nil {
				t.Fatal("unverified Development target accepted")
			}
		})
	}
}
