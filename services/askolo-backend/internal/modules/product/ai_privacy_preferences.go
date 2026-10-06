package product

import (
	"context"
	"errors"
	"net/http"

	"askolo/backend/internal/adapters/postgres"
	policy "askolo/backend/internal/platform/authorization"
)

func (h *Handler) aiPrivacyPreferences(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeCreditSessionError(w, status)
		return
	}
	if !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	preferences, err := h.store.AIPrivacyPreferences(r.Context(), userID)
	if err != nil {
		h.storeError(w, "AI privacy preference lookup failed", err)
		return
	}
	preferences.AssemblyAILiveAvailable = h.assemblyAILiveAvailable(r.Context())
	writeJSON(w, http.StatusOK, preferences)
}

func (h *Handler) updateAIPrivacyPreferences(w http.ResponseWriter, r *http.Request) {
	userID, status := h.sessionUserID(r)
	if status != http.StatusOK {
		writeCreditSessionError(w, status)
		return
	}
	if !h.authorize(r, userID, "ai", "", policy.ActionAIExecute, w) {
		return
	}
	var input struct {
		RecordedVoiceInputConsent  *bool   `json:"recordedVoiceInput"`
		AssistantProcessingConsent *bool   `json:"assistantProcessing"`
		AssemblyAILiveConsent      *bool   `json:"assemblyAiLive"`
		AssemblyAILiveVoice        *string `json:"assemblyAiLiveVoice"`
		RedactionLocation          *string `json:"redactionLocation"`
	}
	if !decodeBody(w, r, &input) {
		return
	}
	if input.RecordedVoiceInputConsent == nil && input.AssistantProcessingConsent == nil &&
		input.AssemblyAILiveConsent == nil && input.AssemblyAILiveVoice == nil && input.RedactionLocation == nil {
		writeError(w, http.StatusBadRequest, "INVALID_AI_PRIVACY_PREFERENCES", "Choose at least one privacy setting to update.")
		return
	}
	if input.AssemblyAILiveVoice != nil && !postgres.ValidAssemblyAILiveVoice(*input.AssemblyAILiveVoice) {
		writeError(w, http.StatusBadRequest, "INVALID_LIVE_VOICE", "Choose one of the supported Live Mode voices.")
		return
	}
	if input.RedactionLocation != nil && *input.RedactionLocation != "app" {
		writeError(w, http.StatusBadRequest, "INVALID_REDACTION_LOCATION", "App-side transcript redaction is the only available mode.")
		return
	}
	if input.AssistantProcessingConsent != nil && *input.AssistantProcessingConsent {
		redactionLocation := input.RedactionLocation
		if redactionLocation == nil {
			current, err := h.store.AIPrivacyPreferences(r.Context(), userID)
			if err != nil {
				h.storeError(w, "AI privacy preference lookup failed", err)
				return
			}
			redactionLocation = current.RedactionLocation
		}
		if redactionLocation == nil || *redactionLocation != "app" {
			writeError(w, http.StatusBadRequest, "REDACTION_LOCATION_REQUIRED", "Choose app-side transcript redaction before enabling Assistant processing.")
			return
		}
	}

	preferences, err := h.store.UpdateAIPrivacyPreferences(r.Context(), userID, postgres.AIPrivacyPreferencesPatch{
		RecordedVoiceInputConsent:  input.RecordedVoiceInputConsent,
		AssistantProcessingConsent: input.AssistantProcessingConsent,
		AssemblyAILiveConsent:      input.AssemblyAILiveConsent,
		AssemblyAILiveVoice:        input.AssemblyAILiveVoice,
		RedactionLocation:          input.RedactionLocation,
	})
	if err != nil {
		if errors.Is(err, postgres.ErrAssistantRedactionLocationRequired) {
			writeError(w, http.StatusBadRequest, "REDACTION_LOCATION_REQUIRED", "Choose app-side transcript redaction before enabling Assistant processing.")
			return
		}
		h.storeError(w, "AI privacy preference update failed", err)
		return
	}
	preferences.AssemblyAILiveAvailable = h.assemblyAILiveAvailable(r.Context())
	writeJSON(w, http.StatusOK, preferences)
}

func (h *Handler) assemblyAILiveAvailable(ctx context.Context) bool {
	if h == nil || !h.voiceAgentEnabled || !h.hasAllVoiceAgentIDs() || h.realtimeLimiter == nil || h.store == nil {
		return false
	}
	provider, ok := h.assemblyAI.(assemblyAIVoiceAgentProvider)
	if !ok || !provider.Configured() {
		return false
	}
	policy, err := h.store.USDPolicy(ctx)
	if err != nil {
		return false
	}
	return voiceAgentCreditRateConfigured(policy)
}

func voiceAgentCreditRateConfigured(policy postgres.USDPolicy) bool {
	card, ok := policy.RateCards["assemblyai:voice_agent:"+assemblyAIVoiceAgentModel]
	return ok && card.Meter == "hour" && card.UsdMicrosPerHour > 0
}
