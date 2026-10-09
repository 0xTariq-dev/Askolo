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
