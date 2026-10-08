package config

import (
	"testing"
)

func TestResolveReleaseMetadata(t *testing.T) {
	tests := []struct {
		name             string
		configuredCommit string
		configuredTag    string
		vcsRevision      string
		vcsModified      bool
		deriveMissingTag bool
		wantCommit       string
		wantTag          string
	}{
		{
			name:             "embedded revision overrides stale configured commit",
			configuredCommit: "stale-revision",
			vcsRevision:      "abc123",
			deriveMissingTag: true,
			wantCommit:       "abc123",
			wantTag:          "commit-abc123",
		},
		{
			name:             "dirty build is identified",
			vcsRevision:      "abc123",
			vcsModified:      true,
			deriveMissingTag: true,
			wantCommit:       "abc123",
			wantTag:          "commit-abc123-dirty",
		},
		{
			name:             "configured tag is preserved",
			configuredTag:    "release-1",
			vcsRevision:      "abc123",
			deriveMissingTag: true,
			wantCommit:       "abc123",
			wantTag:          "release-1",
		},
		{
			name:             "configured values remain fallback without VCS metadata",
			configuredCommit: "configured-revision",
			configuredTag:    "release-1",
			wantCommit:       "configured-revision",
			wantTag:          "release-1",
		},
		{
			name:             "development does not derive a release tag",
			vcsRevision:      "abc123",
			deriveMissingTag: false,
			wantCommit:       "abc123",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotCommit, gotTag := resolveReleaseMetadata(
				test.configuredCommit,
				test.configuredTag,
				test.vcsRevision,
				test.vcsModified,
				test.deriveMissingTag,
			)
			if gotCommit != test.wantCommit || gotTag != test.wantTag {
				t.Fatalf(
					"resolveReleaseMetadata() = (%q, %q), want (%q, %q)",
					gotCommit,
					gotTag,
					test.wantCommit,
					test.wantTag,
				)
			}
		})
	}
}

func TestLoadStagingDerivesReleaseMetadataFromBuild(t *testing.T) {
	revision, modified := currentBuildVCSMetadata()
	if revision == "" {
		t.Fatal("test binary must include the embedded VCS revision")
	}

	for key, value := range map[string]string{
		"ASKOLO_ENVIRONMENT":                    "staging",
		"ASKOLO_ASSEMBLYAI_VOICE_AGENT_ENABLED": "false",
		"ASKOLO_ASSEMBLYAI_AGENT_IDS":           "",
		"ASKOLO_CANONICAL_ORIGIN":               "https://staging.askolo.app",
		"ASKOLO_COOKIE_NAMESPACE":               "askolo_test",
		"ASKOLO_DATABASE_ID":                    "staging-database",
		"ASKOLO_RELEASE_MODE":                   "normal",
		"ASKOLO_COMMIT_SHA":                     "stale-revision",
		"ASKOLO_RELEASE_TAG":                    "",
		"ASKO_PARENT_PRODUCTION_TAG":            "",
		"ASKOLO_INTERNAL_TOKEN":                 "test-internal-token",
		"ASKOLO_ADMIN_EMAILS":                   "",
		"SESSION_SECRET":                        "session-secret-for-tests-123456789",
		"AUTH_CHALLENGE_SECRET":                 "",
		"AUTH_RATE_LIMIT_HMAC_SECRET":           "rate-limit-hmac-secret-for-tests-123456789",
		"GOOGLE_TOKEN_ENCRYPTION_KEY":           "",
		"AUTH_TOTP_ENCRYPTION_KEY":              "",
		"TURNSTILE_SECRET_KEY":                  "test-turnstile-secret",
	} {
		t.Setenv(key, value)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() with embedded staging metadata: %v", err)
	}
	wantTag := "commit-" + revision
	if modified {
		wantTag += "-dirty"
	}
	if cfg.BuildCommit != revision {
		t.Fatalf("BuildCommit = %q, want embedded revision %q", cfg.BuildCommit, revision)
	}
	if cfg.ReleaseTag != wantTag {
		t.Fatalf("ReleaseTag = %q, want derived tag %q", cfg.ReleaseTag, wantTag)
	}
}
