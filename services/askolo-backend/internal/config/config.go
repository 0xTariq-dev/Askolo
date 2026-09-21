package config

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	defaultHost = "0.0.0.0"
	defaultPort = 8090
)

type Config struct {
	ServiceName       string
	Environment       string
	Host              string
	Port              int
	InternalAuthToken string
	DatabaseURL       string
	DatabaseIdentity  string
	SessionSecret     string
	CanonicalOrigin   string
	SessionCookieName string
	BuildCommit       string
	ReleaseTag        string
	ReleaseMode       string
	ParentReleaseTag  string
	TOTPEncryptionKey []byte
	Email             EmailConfig
	Google            GoogleOAuthConfig
	GitHub            GitHubOAuthConfig
	AllowedOAuthHosts map[string]struct{}
}

type EmailConfig struct {
	SMTPHost        string
	SMTPPort        int
	SMTPUsername    string
	SMTPPassword    string
	FromAddress     string
	ChallengeSecret string
}

type GoogleOAuthConfig struct {
	LoginClientID       string
	LoginClientSecret   string
	IntegrationClientID string
	IntegrationSecret   string
	TokenEncryptionKey  []byte
	AuthURL             string
	TokenURL            string
	UserInfoURL         string
	RevokeURL           string
}

type GitHubOAuthConfig struct {
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	UserURL      string
	EmailsURL    string
}

func Load() (Config, error) {
	port, err := envPort("PORT", defaultPort)
	if err != nil {
		return Config{}, err
	}
	smtpPort, err := envPort("AUTH_SMTP_PORT", 587)
	if err != nil {
		return Config{}, err
	}

	explicitEnvironment := strings.TrimSpace(os.Getenv("ASKOLO_ENVIRONMENT"))
	environment := explicitEnvironment
	if environment == "" {
		environment = strings.TrimSpace(os.Getenv("NODE_ENV"))
	}
	if environment == "" {
		environment = strings.TrimSpace(os.Getenv("APP_ENV"))
	}
	if environment == "" {
		environment = "development"
	}

	host := strings.TrimSpace(os.Getenv("BACKEND_HOST"))
	if host == "" {
		host = defaultHost
	}

	environment = strings.ToLower(environment)
	if environment != "development" && environment != "staging" && environment != "production" {
		return Config{}, fmt.Errorf("ASKOLO_ENVIRONMENT must be development, staging, or production")
	}
	if environment == "production" && explicitEnvironment == "" {
		return Config{}, fmt.Errorf("ASKOLO_ENVIRONMENT is required in production")
	}
	encryptionKey, err := loadEncryptionKey(os.Getenv("GOOGLE_TOKEN_ENCRYPTION_KEY"))
	if err != nil {
		return Config{}, err
	}
	sessionSecret := strings.TrimSpace(os.Getenv("SESSION_SECRET"))
	internalAuthToken := strings.TrimSpace(os.Getenv("ASKOLO_INTERNAL_TOKEN"))
	if internalAuthToken == "" && environment != "production" && sessionSecret != "" {
		derived := sha256.Sum256([]byte("askolo-internal-auth:" + sessionSecret))
		internalAuthToken = hex.EncodeToString(derived[:])
	}
	totpEncryptionKey, err := loadEncryptionKey(os.Getenv("AUTH_TOTP_ENCRYPTION_KEY"))
	if err != nil {
		return Config{}, err
	}
	if len(totpEncryptionKey) == 0 && sessionSecret != "" {
		derived := sha256.Sum256([]byte("askolo-auth-totp:" + sessionSecret))
		totpEncryptionKey = derived[:]
	}
	challengeSecret := strings.TrimSpace(os.Getenv("AUTH_CHALLENGE_SECRET"))
	if challengeSecret == "" {
		challengeSecret = sessionSecret
	}
	canonicalOrigin := strings.TrimSpace(os.Getenv("ASKOLO_CANONICAL_ORIGIN"))
	if environment != "development" {
		if canonicalOrigin == "" {
			return Config{}, fmt.Errorf("ASKOLO_CANONICAL_ORIGIN is required outside development")
		}
		parsedOrigin, parseErr := url.Parse(canonicalOrigin)
		if parseErr != nil || parsedOrigin.Scheme != "https" || parsedOrigin.Host == "" ||
			(parsedOrigin.Path != "" && parsedOrigin.Path != "/") || parsedOrigin.RawQuery != "" || parsedOrigin.Fragment != "" {
			return Config{}, fmt.Errorf("ASKOLO_CANONICAL_ORIGIN must be an HTTPS origin")
		}
		canonicalOrigin = parsedOrigin.Scheme + "://" + parsedOrigin.Host
	}
	databaseIdentity := strings.TrimSpace(os.Getenv("ASKOLO_DATABASE_ID"))
	if databaseIdentity == "" && environment == "development" {
		databaseIdentity = "development-database"
	}
	cookieNamespace := strings.TrimSpace(os.Getenv("ASKOLO_COOKIE_NAMESPACE"))
	if cookieNamespace == "" && environment == "development" {
		cookieNamespace = "askolo_dev"
	}
	if !validCookieNamespace(cookieNamespace) {
		return Config{}, fmt.Errorf("ASKOLO_COOKIE_NAMESPACE must be 2-32 lowercase characters")
	}
	releaseMode := strings.TrimSpace(os.Getenv("ASKOLO_RELEASE_MODE"))
	if releaseMode == "" && environment == "development" {
		releaseMode = "development"
	}
	buildCommit := strings.TrimSpace(os.Getenv("ASKOLO_COMMIT_SHA"))
	releaseTag := strings.TrimSpace(os.Getenv("ASKOLO_RELEASE_TAG"))
	parentReleaseTag := strings.TrimSpace(os.Getenv("ASKO_PARENT_PRODUCTION_TAG"))
	if environment != "development" {
		for name, value := range map[string]string{
			"ASKOLO_DATABASE_ID": databaseIdentity, "ASKOLO_COMMIT_SHA": buildCommit,
			"ASKOLO_RELEASE_TAG": releaseTag, "ASKOLO_INTERNAL_TOKEN": internalAuthToken,
		} {
			if value == "" {
				return Config{}, fmt.Errorf("%s is required outside development", name)
			}
		}
		if releaseMode != "normal" && releaseMode != "hotfix" {
			return Config{}, fmt.Errorf("ASKOLO_RELEASE_MODE must be normal or hotfix outside development")
		}
		if releaseMode == "hotfix" && parentReleaseTag == "" {
			return Config{}, fmt.Errorf("ASKOLO_PARENT_PRODUCTION_TAG is required for hotfix releases")
		}
	}

	return Config{
		ServiceName:       "askolo-backend",
		Environment:       environment,
		Host:              host,
		Port:              port,
		InternalAuthToken: internalAuthToken,
		DatabaseURL:       strings.TrimSpace(os.Getenv("DATABASE_URL")),
		DatabaseIdentity:  databaseIdentity,
		SessionSecret:     sessionSecret,
		CanonicalOrigin:   canonicalOrigin,
		SessionCookieName: cookieNamespace + "_sid",
		BuildCommit:       buildCommit,
		ReleaseTag:        releaseTag,
		ReleaseMode:       releaseMode,
		ParentReleaseTag:  parentReleaseTag,
		TOTPEncryptionKey: totpEncryptionKey,
		Email: EmailConfig{
			SMTPHost:        strings.TrimSpace(os.Getenv("AUTH_SMTP_HOST")),
			SMTPPort:        smtpPort,
			SMTPUsername:    strings.TrimSpace(os.Getenv("AUTH_SMTP_USERNAME")),
			SMTPPassword:    os.Getenv("AUTH_SMTP_PASSWORD"),
			FromAddress:     strings.TrimSpace(os.Getenv("AUTH_EMAIL_FROM")),
			ChallengeSecret: challengeSecret,
		},
		Google: GoogleOAuthConfig{
			LoginClientID:       strings.TrimSpace(os.Getenv("GOOGLE_LOGIN_CLIENT_ID")),
			LoginClientSecret:   strings.TrimSpace(os.Getenv("GOOGLE_LOGIN_CLIENT_SECRET")),
			IntegrationClientID: strings.TrimSpace(os.Getenv("GOOGLE_INTEGRATION_CLIENT_ID")),
			IntegrationSecret:   strings.TrimSpace(os.Getenv("GOOGLE_INTEGRATION_CLIENT_SECRET")),
			TokenEncryptionKey:  encryptionKey,
			AuthURL:             "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:            "https://oauth2.googleapis.com/token",
			UserInfoURL:         "https://openidconnect.googleapis.com/v1/userinfo",
			RevokeURL:           "https://oauth2.googleapis.com/revoke",
		},
		GitHub: GitHubOAuthConfig{
			ClientID:     strings.TrimSpace(os.Getenv("GITHUB_LOGIN_CLIENT_ID")),
			ClientSecret: strings.TrimSpace(os.Getenv("GITHUB_LOGIN_CLIENT_SECRET")),
			AuthURL:      "https://github.com/login/oauth/authorize",
			TokenURL:     "https://github.com/login/oauth/access_token",
			UserURL:      "https://api.github.com/user",
			EmailsURL:    "https://api.github.com/user/emails",
		},
		AllowedOAuthHosts: oauthHosts(environment, canonicalOrigin),
	}, nil
}

func oauthHosts(environment, canonicalOrigin string) map[string]struct{} {
	hosts := map[string]struct{}{}
	if parsed, err := url.Parse(canonicalOrigin); err == nil && parsed.Hostname() != "" {
		hosts[parsed.Hostname()] = struct{}{}
	}
	if environment != "production" {
		hosts["localhost"] = struct{}{}
		hosts["127.0.0.1"] = struct{}{}
		if devDomain := strings.TrimSpace(os.Getenv("REPLIT_DEV_DOMAIN")); devDomain != "" {
			hosts[devDomain] = struct{}{}
		}
	}
	return hosts
}

func validCookieNamespace(value string) bool {
	if len(value) < 2 || len(value) > 32 {
		return false
	}
	for index, character := range value {
		if (character < 'a' || character > 'z') &&
			(character < '0' || character > '9') &&
			character != '_' && character != '-' {
			return false
		}
		if index == 0 && (character < 'a' || character > 'z') {
			return false
		}
	}
	return true
}

func CookieName(configured string) string {
	if configured == "" {
		return "sid"
	}
	return configured
}

func loadEncryptionKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(raw); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := base64.StdEncoding.DecodeString(raw); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := hex.DecodeString(raw); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if len(raw) == 32 {
		sum := sha256.Sum256([]byte(raw))
		return sum[:], nil
	}
	return nil, fmt.Errorf("GOOGLE_TOKEN_ENCRYPTION_KEY must decode to 32 bytes")
}

func envPort(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}

	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("%s must be a valid TCP port", name)
	}
	return port, nil
}
