package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// InfoHandler handles the info endpoint
type InfoHandler struct{}

// NewInfoHandler creates a new InfoHandler
func NewInfoHandler() *InfoHandler {
	return &InfoHandler{}
}

// Info handles GET /api/v0/info
// Returns the ARN of the IAM role used to invoke Lambda functions in this regional account.
// The account ID is parsed from the TARGET_GROUP_ARN environment variable.
func (h *InfoHandler) Info(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	tgARN := os.Getenv("TARGET_GROUP_ARN")
	// Target Group ARN format: arn:aws:elasticloadbalancing:{region}:{account_id}:targetgroup/{name}/{id}
	parts := strings.SplitN(tgARN, ":", 6)
	if len(parts) < 6 || parts[4] == "" {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"kind":   "Error",
			"code":   "regional-account-unavailable",
			"reason": "regional account ID is not configured",
		})
		return
	}

	accountID := parts[4]
	arn := fmt.Sprintf("arn:aws:iam::%s:role/LambdaExecutor", accountID)

	_ = json.NewEncoder(w).Encode(map[string]string{"arn": arn})
}
