package migrations_test

import (
	"os"
	"strings"
	"testing"
)

func TestMigration183DropsPromptAuditFullPrompt(t *testing.T) {
	content, err := os.ReadFile("183_drop_prompt_audit_full_prompt.sql")
	if err != nil {
		t.Fatal(err)
	}
	normalized := strings.ToLower(string(content))
	if !strings.Contains(normalized, "drop column if exists full_prompt") {
		t.Fatal("migration 183 must remove prompt_audit_events.full_prompt")
	}
}
