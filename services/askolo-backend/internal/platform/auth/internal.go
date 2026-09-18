package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"askolo/backend/internal/platform/apierror"
)

type InternalMiddleware struct {
	token string
}

func NewInternalMiddleware(token string) InternalMiddleware {
	return InternalMiddleware{token: strings.TrimSpace(token)}
}

func (m InternalMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.token == "" {
			apierror.Write(w, r, http.StatusServiceUnavailable, "INTERNAL_AUTH_NOT_CONFIGURED", "Internal service authentication is not configured.")
			return
		}

		presented := r.Header.Get("X-Askolo-Internal-Token")
		if presented == "" {
			authorization := r.Header.Get("Authorization")
			if strings.HasPrefix(authorization, "Bearer ") {
				presented = strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
			}
		}

		if !secureEqual(presented, m.token) {
			apierror.Write(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Unauthorized.")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func secureEqual(left string, right string) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}