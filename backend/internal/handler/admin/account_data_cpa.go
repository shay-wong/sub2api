package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

const cpaDataType = "cpa-auth-files"

type CPADataPayload struct {
	Type           string           `json:"type"`
	ExportedAt     string           `json:"exported_at"`
	Accounts       []CPADataAccount `json:"accounts"`
	SkippedShadows int              `json:"skipped_shadows,omitempty"`
}

type CPADataAccount struct {
	AccountID    string `json:"account_id"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	Email        string `json:"email,omitempty"`
	Type         string `json:"type,omitempty"`
	Expired      string `json:"expired,omitempty"`
}

func looksLikeCPAData(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return false
	}
	if trimmed[0] == '[' {
		var items []map[string]any
		if err := json.Unmarshal(trimmed, &items); err != nil || len(items) == 0 {
			return false
		}
		return looksLikeCPAAccountMap(items[0])
	}
	if trimmed[0] != '{' {
		return false
	}
	var obj map[string]any
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return false
	}
	if looksLikeCPAAccountMap(obj) {
		return true
	}
	accounts, ok := obj["accounts"].([]any)
	if !ok || len(accounts) == 0 {
		return false
	}
	first, ok := accounts[0].(map[string]any)
	return ok && looksLikeCPAAccountMap(first)
}

func looksLikeCPAAccountMap(item map[string]any) bool {
	if item == nil {
		return false
	}
	if _, hasAccessToken := item["access_token"]; !hasAccessToken {
		return false
	}
	_, hasCredentials := item["credentials"]
	_, hasPlatform := item["platform"]
	return !hasCredentials && !hasPlatform
}

func convertCPADataPayload(raw json.RawMessage) (DataPayload, error) {
	accounts, err := parseCPAAccounts(raw)
	if err != nil {
		return DataPayload{}, err
	}
	dataAccounts := make([]DataAccount, 0, len(accounts))
	for i := range accounts {
		account, err := convertCPAAccountToDataAccount(accounts[i], i+1)
		if err != nil {
			return DataPayload{}, err
		}
		dataAccounts = append(dataAccounts, account)
	}
	return DataPayload{
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Proxies:    []DataProxy{},
		Accounts:   dataAccounts,
	}, nil
}

func parseCPAAccounts(raw json.RawMessage) ([]CPADataAccount, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, errors.New("CPA data is required")
	}
	if trimmed[0] == '[' {
		var accounts []CPADataAccount
		if err := json.Unmarshal(trimmed, &accounts); err != nil {
			return nil, fmt.Errorf("invalid CPA accounts array: %w", err)
		}
		return validateCPAAccounts(accounts)
	}
	if trimmed[0] != '{' {
		return nil, errors.New("invalid CPA data")
	}

	var wrapper struct {
		Type     string           `json:"type"`
		Accounts []CPADataAccount `json:"accounts"`
	}
	if err := json.Unmarshal(trimmed, &wrapper); err != nil {
		return nil, fmt.Errorf("invalid CPA data: %w", err)
	}
	if len(wrapper.Accounts) > 0 {
		if strings.TrimSpace(wrapper.Type) != "" && strings.TrimSpace(wrapper.Type) != cpaDataType {
			return nil, fmt.Errorf("unsupported CPA data type: %s", wrapper.Type)
		}
		return validateCPAAccounts(wrapper.Accounts)
	}

	var account CPADataAccount
	if err := json.Unmarshal(trimmed, &account); err != nil {
		return nil, fmt.Errorf("invalid CPA account: %w", err)
	}
	return validateCPAAccounts([]CPADataAccount{account})
}

func validateCPAAccounts(accounts []CPADataAccount) ([]CPADataAccount, error) {
	if len(accounts) == 0 {
		return nil, errors.New("CPA accounts is required")
	}
	for i := range accounts {
		if strings.TrimSpace(accounts[i].AccessToken) == "" {
			return nil, fmt.Errorf("CPA account %d access_token is required", i+1)
		}
	}
	return accounts, nil
}

func convertCPAAccountToDataAccount(source CPADataAccount, index int) (DataAccount, error) {
	source.AccessToken = strings.TrimSpace(source.AccessToken)
	source.IDToken = strings.TrimSpace(source.IDToken)
	source.RefreshToken = strings.TrimSpace(source.RefreshToken)
	source.AccountID = strings.TrimSpace(source.AccountID)
	source.Email = strings.TrimSpace(source.Email)
	source.Type = strings.TrimSpace(source.Type)

	accessClaims, _ := decodeCodexJWTClaims(source.AccessToken)
	idClaims, _ := decodeCodexJWTClaims(source.IDToken)

	chatGPTUserID := cpaClaimsUserID(accessClaims)
	chatGPTAccountID := firstNonEmptyString(source.AccountID, cpaClaimsAccountID(accessClaims), cpaClaimsAccountID(idClaims))
	email := firstNonEmptyString(source.Email, cpaClaimsEmail(accessClaims), cpaClaimsEmail(idClaims))
	organizationID := firstNonEmptyString(cpaClaimsOrganizationID(idClaims), cpaClaimsOrganizationID(accessClaims))
	expiresAt := parseCPAExpiredTime(source.Expired)
	if expiresAt == 0 && accessClaims != nil {
		expiresAt = accessClaims.Exp
	}

	credentials := map[string]any{
		"access_token": source.AccessToken,
		"expires_in":   float64(864000),
	}
	setIfNotEmpty(credentials, "chatgpt_account_id", chatGPTAccountID)
	setIfNotEmpty(credentials, "chatgpt_user_id", chatGPTUserID)
	setIfNotEmpty(credentials, "organization_id", organizationID)
	setIfNotEmpty(credentials, "refresh_token", source.RefreshToken)
	setIfNotEmpty(credentials, "id_token", source.IDToken)
	setIfNotEmpty(credentials, "email", email)
	if expiresAt > 0 {
		credentials["expires_at"] = float64(expiresAt)
	}

	extra := map[string]any{}
	setIfNotEmpty(extra, "email", email)
	setIfNotEmpty(extra, "cpa_type", source.Type)
	if len(extra) == 0 {
		extra = nil
	}

	rateMultiplier := 1.0
	autoPauseOnExpired := true
	return DataAccount{
		Name:               buildCPAImportAccountName(source.Type, email, chatGPTUserID, chatGPTAccountID, index),
		Platform:           service.PlatformOpenAI,
		Type:               service.AccountTypeOAuth,
		Credentials:        credentials,
		Extra:              extra,
		Concurrency:        10,
		Priority:           1,
		RateMultiplier:     &rateMultiplier,
		AutoPauseOnExpired: &autoPauseOnExpired,
	}, nil
}

func buildCPADataPayload(accounts []service.Account, skippedShadows int) (CPADataPayload, error) {
	out := make([]CPADataAccount, 0, len(accounts))
	for i := range accounts {
		account := accounts[i]
		if account.Platform != service.PlatformOpenAI || account.Type != service.AccountTypeOAuth {
			return CPADataPayload{}, fmt.Errorf("CPA export only supports OpenAI OAuth accounts: %s", account.Name)
		}
		accessToken := strings.TrimSpace(account.GetOpenAIAccessToken())
		if accessToken == "" {
			return CPADataPayload{}, fmt.Errorf("OpenAI account access_token is required for CPA export: %s", account.Name)
		}
		accessClaims, _ := decodeCodexJWTClaims(accessToken)
		idToken := strings.TrimSpace(account.GetOpenAIIDToken())
		idClaims, _ := decodeCodexJWTClaims(idToken)
		accountID := firstNonEmptyString(account.GetChatGPTAccountID(), cpaClaimsAccountID(accessClaims), cpaClaimsAccountID(idClaims), account.Name)
		expiresAt := account.GetOpenAITokenExpiresAt()
		expired := ""
		if expiresAt != nil {
			expired = expiresAt.UTC().Format(time.RFC3339)
		}
		out = append(out, CPADataAccount{
			AccountID:    accountID,
			AccessToken:  accessToken,
			RefreshToken: strings.TrimSpace(account.GetOpenAIRefreshToken()),
			IDToken:      idToken,
			Email:        firstNonEmptyString(account.GetExtraString("email"), account.GetCredential("email"), cpaClaimsEmail(accessClaims), cpaClaimsEmail(idClaims)),
			Type:         firstNonEmptyString(account.GetExtraString("cpa_type"), "codex"),
			Expired:      expired,
		})
	}
	if len(out) == 0 {
		return CPADataPayload{}, errors.New("no OpenAI OAuth accounts available for CPA export")
	}
	return CPADataPayload{
		Type:           cpaDataType,
		ExportedAt:     time.Now().UTC().Format(time.RFC3339),
		Accounts:       out,
		SkippedShadows: skippedShadows,
	}, nil
}

func buildCPAImportAccountName(accountType, email, userID, accountID string, index int) string {
	prefix := strings.TrimSpace(accountType)
	if prefix == "" {
		prefix = "unknown"
	}
	suffix := firstNonEmptyString(email, userID, accountID)
	if suffix == "" {
		suffix = fmt.Sprintf("account-%04d", index)
	}
	return fmt.Sprintf("%s-%s", prefix, suffix)
}

func parseCPAExpiredTime(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.Unix()
	}
	if t, err := time.Parse("2006-01-02T15:04:05", value); err == nil {
		return t.UTC().Unix()
	}
	return 0
}

func cpaClaimsEmail(claims *codexJWTClaims) string {
	if claims == nil {
		return ""
	}
	return strings.TrimSpace(claims.Email)
}

func cpaClaimsAccountID(claims *codexJWTClaims) string {
	if claims == nil || claims.OpenAIAuth == nil {
		return ""
	}
	return strings.TrimSpace(claims.OpenAIAuth.ChatGPTAccountID)
}

func cpaClaimsUserID(claims *codexJWTClaims) string {
	if claims == nil {
		return ""
	}
	if claims.OpenAIAuth == nil {
		return strings.TrimSpace(claims.Sub)
	}
	return firstNonEmptyString(
		claims.OpenAIAuth.ChatGPTUserID,
		claims.OpenAIAuth.UserID,
		claims.Sub,
	)
}

func cpaClaimsOrganizationID(claims *codexJWTClaims) string {
	if claims == nil || claims.OpenAIAuth == nil {
		return ""
	}
	if strings.TrimSpace(claims.OpenAIAuth.POID) != "" {
		return strings.TrimSpace(claims.OpenAIAuth.POID)
	}
	for _, org := range claims.OpenAIAuth.Organizations {
		if org.IsDefault && strings.TrimSpace(org.ID) != "" {
			return strings.TrimSpace(org.ID)
		}
	}
	if len(claims.OpenAIAuth.Organizations) > 0 {
		return strings.TrimSpace(claims.OpenAIAuth.Organizations[0].ID)
	}
	return ""
}

func setIfNotEmpty(target map[string]any, key, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		target[key] = value
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
