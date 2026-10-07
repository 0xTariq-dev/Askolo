package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	turnstileSiteverifyURL      = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	turnstileRequestTimeout     = 5 * time.Second
	turnstileMaxTokenBytes      = 2048
	turnstileMaxResponseBytes   = 16 * 1024
	turnstileFailureMessage     = "Security verification could not be completed. Please try again."
	turnstileUnavailableMessage = "Authentication is temporarily unavailable."
)

type turnstileSiteverifyResponse struct {
	Success  bool   `json:"success"`
	Action   string `json:"action"`
	Hostname string `json:"hostname"`
}

func (h *Handler) requireTurnstile(
	w http.ResponseWriter,
	r *http.Request,
	token string,
	expectedAction string,
	operation string,
) bool {
	if token == "" || len(token) > turnstileMaxTokenBytes {
		writeError(w, http.StatusForbidden, "TURNSTILE_FAILED", turnstileFailureMessage)
		return false
	}
	if strings.TrimSpace(h.cfg.TurnstileSecret) == "" || len(h.cfg.TurnstileAllowedHostnames) == 0 {
		h.logger.Error(
			"turnstile verification is unavailable",
			"operation", operation,
			"request_id", requestID(r),
			"reason", "configuration_missing",
		)
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", turnstileUnavailableMessage)
		return false
	}

	ctx, cancel := context.WithTimeout(r.Context(), turnstileRequestTimeout)
	defer cancel()
	result, err := h.siteverify(ctx, token)
	if err != nil {
		h.logger.Warn(
			"turnstile Siteverify request failed",
			"operation", operation,
			"request_id", requestID(r),
			"reason", "provider_error",
		)
		writeError(w, http.StatusServiceUnavailable, "AUTH_UNAVAILABLE", turnstileUnavailableMessage)
		return false
	}
	if !result.Success || result.Action != expectedAction {
		writeError(w, http.StatusForbidden, "TURNSTILE_FAILED", turnstileFailureMessage)
		return false
	}
	if _, ok := h.cfg.TurnstileAllowedHostnames[result.Hostname]; !ok {
		writeError(w, http.StatusForbidden, "TURNSTILE_FAILED", turnstileFailureMessage)
		return false
	}
	return true
}

func (h *Handler) siteverify(ctx context.Context, token string) (turnstileSiteverifyResponse, error) {
	if h.turnstileVerifier != nil {
		return h.turnstileVerifier(ctx, token)
	}

	form := url.Values{}
	form.Set("secret", h.cfg.TurnstileSecret)
	form.Set("response", token)
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		h.turnstileEndpoint,
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return turnstileSiteverifyResponse{}, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := h.turnstileClient
	if client == nil {
		client = &http.Client{Timeout: turnstileRequestTimeout}
	}
	response, err := client.Do(request)
	if err != nil {
		return turnstileSiteverifyResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return turnstileSiteverifyResponse{}, errTurnstileSiteverifyStatus
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, turnstileMaxResponseBytes+1))
	if err != nil || len(body) > turnstileMaxResponseBytes {
		return turnstileSiteverifyResponse{}, errTurnstileSiteverifyResponse
	}
	var result turnstileSiteverifyResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return turnstileSiteverifyResponse{}, errTurnstileSiteverifyResponse
	}
	return result, nil
}

var (
	errTurnstileSiteverifyStatus   = &turnstileSiteverifyError{kind: "http_status"}
	errTurnstileSiteverifyResponse = &turnstileSiteverifyError{kind: "invalid_response"}
)

type turnstileSiteverifyError struct {
	kind string
}

func (e *turnstileSiteverifyError) Error() string {
	return e.kind
}
