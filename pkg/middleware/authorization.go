package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// Authorization provides account allowlist-based authorization middleware
type Authorization struct {
	allowedAccounts map[string]struct{}
	logger          *slog.Logger
}

// NewAuthorization creates a new Authorization middleware
func NewAuthorization(allowedAccounts []string, logger *slog.Logger) *Authorization {
	allowed := make(map[string]struct{}, len(allowedAccounts))
	for _, acc := range allowedAccounts {
		allowed[acc] = struct{}{}
	}
	return &Authorization{
		allowedAccounts: allowed,
		logger:          logger,
	}
}

// RequireAllowedAccount verifies that the AWS account is in the allowlist
func (a *Authorization) RequireAllowedAccount(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		accountID := GetAccountID(ctx)

		if accountID == "" {
			a.logger.Warn("missing account ID in request")
			a.writeError(w, http.StatusForbidden, "missing-account-id", "Account ID header is required")
			return
		}

		if _, allowed := a.allowedAccounts[accountID]; !allowed {
			a.logger.Warn("account not allowed", "account_id", accountID)
			a.writeError(w, http.StatusForbidden, "account-not-allowed", "account not allowed")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (a *Authorization) writeError(w http.ResponseWriter, status int, code, reason string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := map[string]interface{}{
		"kind":   "Error",
		"code":   code,
		"reason": reason,
	}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		a.logger.Error("failed to encode error response", "error", err)
	}
}
