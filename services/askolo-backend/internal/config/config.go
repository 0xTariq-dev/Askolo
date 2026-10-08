package config

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/mail"
	"net/url"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHost = "0.0.0.0"
	defaultPort = 8090
)

type Config struct {
	ServiceName                   string
	Environment                   string
	Host                          string
	Port                          int
	InternalAuthToken             string
	DatabaseURL                   string
	DatabaseIdentity              string
	SessionSecret                 string
	AuthRateLimitHMACSecret       string
	TurnstileSecret               string
	TurnstileAllowedHostnames     map[string]struct{}
	CanonicalOrigin               string
	SessionCookieName             string
	BuildCommit                   string
	ReleaseTag                    string
	ReleaseMode                   string
	ParentReleaseTag              string
	EmailChallengeCleanupInterval time.Duration
	TOTPEncryptionKey             []byte
	Email                         EmailConfig
	Google                        GoogleOAuthConfig
	GitHub                        GitHubOAuthConfig
	AssemblyAIKey                 string
	AssemblyAIVoiceAgentEnabled   bool
	AssemblyAIVoiceAgentIDs       map[string]string
	OpenAIAPIKey                  string
	OpenAIBaseURL                 string
	AzureTTSKey                   string
	AzureTTSRegion                string
	AzureTTSURL                   string
	AllowedOAuthHosts             map[string]struct{}
	AdminEmails                   map[string]struct{}
}

type EmailConfig struct {
	ResendAPIKey    string
	FromAddress     string
	ChallengeSecret string
}

func (e EmailConfig) ResendConfigurationStatus() string {
	if strings.TrimSpace(e.ResendAPIKey) == "" || strings.TrimSpace(e.FromAddress) == "" {
		return "missing"
	}
	if strings.ContainsAny(e.ResendAPIKey, "\r\n") || strings.ContainsAny(e.FromAddress, "\r\n") {
		return "invalid"
	}
	if parsed, err := mail.ParseAddress(e.FromAddress); err != nil || parsed.Address == "" {
		return "invalid"
	}
	return "configured"
}

func (e EmailConfig) ChallengeConfigurationStatus() string {
	if strings.TrimSpace(e.ChallengeSecret) == "" {
		return "missing"
	}
	return "configured"
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

func currentBuildVCSMetadata() (revision string, modified bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil {
		return "", false
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = strings.TrimSpace(setting.Value)
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	return revision, modified
}

func resolveReleaseMetadata(
	configuredCommit string,
	configuredTag string,
	vcsRevision string,
	vcsModified bool,
	deriveMissingTag bool,
) (buildCommit string, releaseTag string) {
	buildCommit = strings.TrimSpace(configuredCommit)
	vcsRevision = strings.TrimSpace(vcsRevision)
	if vcsRevision != "" {
		// Deployment environment values persist between builds, so embedded
		// source metadata must win over a potentially stale configured SHA.
		buildCommit = vcsRevision
	}

	releaseTag = strings.TrimSpace(configuredTag)
	if releaseTag == "" && deriveMissingTag && vcsRevision != "" {
		releaseTag = "commit-" + vcsRevision
		if vcsModified {
			releaseTag += "-dirty"
		}
	}
	return buildCommit, releaseTag
}

func Load() (Config, error) {
	port, err := envPort("PORT", defaultPort)
	if err != nil {
		return Config{}, err
	}
	// Keep managed Voice Agent disabled until provider privacy controls and
	// the account's live-session credit rate have been verified.
	voiceAgentEnabled, err := envBool("ASKOLO_ASSEMBLYAI_VOICE_AGENT_ENABLED", false)
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
	voiceAgentIDs, err := parseAssemblyAIVoiceAgentIDs(os.Getenv("ASKOLO_ASSEMBLYAI_AGENT_IDS"))
	if err != nil {
		return Config{}, err
	}
	voiceAgentEnabled, disabledForIncompleteVoiceAgents := normalizeAssemblyAIVoiceAgentEnabled(
		environment,
		voiceAgentEnabled,
		voiceAgentIDs,
	)
	if disabledForIncompleteVoiceAgents {
		slog.Warn(
			"AssemblyAI Live Mode disabled until all nine stored voice agents are configured",
			"environment",
			environment,
		)
	}
	if err := validateAssemblyAIVoiceAgentConfiguration(environment, voiceAgentEnabled, voiceAgentIDs); err != nil {
		return Config{}, err
	}
	encryptionKey, err := loadEncryptionKey(os.Getenv("GOOGLE_TOKEN_ENCRYPTION_KEY"))
	if err != nil {
		return Config{}, err
	}
	sessionSecret := strings.TrimSpace(os.Getenv("SESSION_SECRET"))
	internalAuthToken := strings.TrimSpace(os.Getenv("ASKOLO_INTERNAL_TOKEN"))
	authRateLimitHMACSecret := strings.TrimSpace(os.Getenv("AUTH_RATE_LIMIT_HMAC_SECRET"))
	if len([]byte(authRateLimitHMACSecret)) < 32 {
		return Config{}, fmt.Errorf("AUTH_RATE_LIMIT_HMAC_SECRET must contain at least 32 bytes")
	}
	turnstileSecret := strings.TrimSpace(os.Getenv("TURNSTILE_SECRET_KEY"))
	if turnstileSecret == "" {
		return Config{}, fmt.Errorf("TURNSTILE_SECRET_KEY is required")
	}
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
	if authRateLimitHMACSecret == sessionSecret || authRateLimitHMACSecret == challengeSecret {
		return Config{}, fmt.Errorf("AUTH_RATE_LIMIT_HMAC_SECRET must be distinct from session and challenge secrets")
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
	vcsRevision, vcsModified := currentBuildVCSMetadata()
	buildCommit, releaseTag := resolveReleaseMetadata(
		os.Getenv("ASKOLO_COMMIT_SHA"),
		os.Getenv("ASKOLO_RELEASE_TAG"),
		vcsRevision,
		vcsModified,
		environment != "development",
	)
	parentReleaseTag := strings.TrimSpace(os.Getenv("ASKO_PARENT_PRODUCTION_TAG"))
	adminEmails, adminErr := parseAdminEmails(os.Getenv("ASKOLO_ADMIN_EMAILS"))
	if adminErr != nil {
		return Config{}, adminErr
	}
	if environment != "development" {
		for name, value := range map[string]string{
			"ASKOLO_DATABASE_ID": databaseIdentity, "ASKOLO_INTERNAL_TOKEN": internalAuthToken,
		} {
			if value == "" {
				return Config{}, fmt.Errorf("%s is required outside development", name)
			}
		}
		if buildCommit == "" {
			return Config{}, fmt.Errorf("ASKOLO_COMMIT_SHA or embedded VCS revision is required outside development")
		}
		if releaseTag == "" {
			return Config{}, fmt.Errorf("ASKOLO_RELEASE_TAG or embedded VCS revision is required outside development")
		}
		if releaseMode != "normal" && releaseMode != "hotfix" {
			return Config{}, fmt.Errorf("ASKOLO_RELEASE_MODE must be normal or hotfix outside development")
		}
		if releaseMode == "hotfix" && parentReleaseTag == "" {
			return Config{}, fmt.Errorf("ASKOLO_PARENT_PRODUCTION_TAG is required for hotfix releases")
		}
	}
	return Config{
		ServiceName:               "askolo-backend",
		Environment:               environment,
		Host:                      host,
		Port:                      port,
		InternalAuthToken:         internalAuthToken,
		DatabaseURL:               strings.TrimSpace(os.Getenv("DATABASE_URL")),
		DatabaseIdentity:          databaseIdentity,
		SessionSecret:             sessionSecret,
		AuthRateLimitHMACSecret:   authRateLimitHMACSecret,
		TurnstileSecret:           turnstileSecret,
		TurnstileAllowedHostnames: turnstileAllowedHostnames(environment),
		CanonicalOrigin:           canonicalOrigin,
		SessionCookieName:         cookieNamespace + "_sid",
		BuildCommit:               buildCommit,
		ReleaseTag:                releaseTag,
		ReleaseMode:               releaseMode,
		ParentReleaseTag:          parentReleaseTag,
		TOTPEncryptionKey:         totpEncryptionKey,
		Email: EmailConfig{
			ResendAPIKey:    strings.TrimSpace(os.Getenv("RESEND_API_KEY")),
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
		AssemblyAIKey:               strings.TrimSpace(os.Getenv("ASSEMBLY_AI_API_KEY")),
		AssemblyAIVoiceAgentEnabled: voiceAgentEnabled,
		AssemblyAIVoiceAgentIDs:     voiceAgentIDs,
		OpenAIAPIKey:                strings.TrimSpace(os.Getenv("AI_INTEGRATIONS_OPENAI_API_KEY")),
		OpenAIBaseURL:               strings.TrimSpace(os.Getenv("AI_INTEGRATIONS_OPENAI_BASE_URL")),
		AzureTTSKey:                 strings.TrimSpace(os.Getenv("AZURE_TTS_KEY")),
		AzureTTSRegion:              strings.TrimSpace(os.Getenv("AZURE_TTS_REGION")),
		AzureTTSURL:                 strings.TrimSpace(os.Getenv("AZURE_TTS_URL")),
		AllowedOAuthHosts:           oauthHosts(environment, canonicalOrigin),
		AdminEmails:                 adminEmails,
	}, nil
}

var supportedAssemblyAIVoices = [...]string{
	"michael", "mary", "paul", "vera", "giovanni", "lola", "juergen", "rafael", "estelle",
}

func normalizeAssemblyAIVoiceAgentEnabled(environment string, enabled bool, ids map[string]string) (bool, bool) {
	if !enabled || environment != "development" {
		return enabled, false
	}
	for _, voice := range supportedAssemblyAIVoices {
		if strings.TrimSpace(ids[voice]) == "" {
			return false, true
		}
	}
	return enabled, false
}

func validateAssemblyAIVoiceAgentConfiguration(environment string, enabled bool, ids map[string]string) error {
	if !enabled {
		return nil
	}
	if environment != "development" {
		return fmt.Errorf("AssemblyAI Live Mode can only be enabled in Development")
	}
	for _, voice := range supportedAssemblyAIVoices {
		if strings.TrimSpace(ids[voice]) == "" {
			return fmt.Errorf("AssemblyAI Live Mode requires all nine stored voice agents in Development")
		}
	}
	return nil
}

func parseAssemblyAIVoiceAgentIDs(raw string) (map[string]string, error) {
	ids := make(map[string]string)
	if strings.TrimSpace(raw) == "" {
		return ids, nil
	}
	if len(raw) > 8192 {
		return nil, fmt.Errorf("ASKOLO_ASSEMBLYAI_AGENT_IDS is too large")
	}
	var configured map[string]string
	if err := json.Unmarshal([]byte(raw), &configured); err != nil || configured == nil {
		return nil, fmt.Errorf("ASKOLO_ASSEMBLYAI_AGENT_IDS must be a JSON object")
	}
	allowed := make(map[string]bool, len(supportedAssemblyAIVoices))
	for _, voice := range supportedAssemblyAIVoices {
		allowed[voice] = true
	}
	for voice, agentID := range configured {
		if !allowed[voice] || !validProviderAgentID(agentID) {
			return nil, fmt.Errorf("ASKOLO_ASSEMBLYAI_AGENT_IDS contains an unsupported voice or invalid agent id")
		}
		ids[voice] = agentID
	}
	return ids, nil
}

func validProviderAgentID(value string) bool {
	if value == "" || len(value) > 200 || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') &&
			!(r >= '0' && r <= '9') && !strings.ContainsRune("._:-", r) {
			return false
		}
	}
	return true
}

func turnstileAllowedHostnames(environment string) map[string]struct{} {
	hosts := map[string]struct{}{
		"dev.askolo.app":     {},
		"web.askolo.app":     {},
		"staging.askolo.app": {},
		"5b8af2fa-e65c-4295-8e4b-b5f0d0c10891-00-2sb81pwvhiesj.picard.replit.dev": {},
	}
	if environment == "development" {
		hosts["localhost"] = struct{}{}
		hosts["127.0.0.1"] = struct{}{}
	}
	return hosts
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

func parseAdminEmails(raw string) (map[string]struct{}, error) {
	result := map[string]struct{}{}
	for _, value := range strings.Split(raw, ",") {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		parsed, err := mail.ParseAddress(value)
		if err != nil || parsed.Address != value || strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("ASKOLO_ADMIN_EMAILS contains an invalid email address")
		}
		result[value] = struct{}{}
	}
	return result, nil
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

func envBool(name string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return value, nil
}
