package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestProductionTLSConfigRejectsCleartextAndFallbackModes(t *testing.T) {
	tests := []struct {
		mode    string
		wantErr bool
	}{
		{mode: "disable", wantErr: true},
		{mode: "prefer", wantErr: true},
		{mode: "allow", wantErr: true},
		{mode: "require", wantErr: false},
		{mode: "verify-full", wantErr: false},
	}

	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			config, err := pgxpool.ParseConfig(
				"postgres://user@127.0.0.1/database?sslmode=" + test.mode,
			)
			if err != nil {
				t.Fatalf("parse test database URL: %v", err)
			}
			err = validateProductionTLSConfig(config)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateProductionTLSConfig() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}
