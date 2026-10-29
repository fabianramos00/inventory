//go:build contract

package contract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNormalizeGeneratedValues(t *testing.T) {
	s := &side{ids: map[string]string{"42": "category_id"}}
	raw := map[string]any{
		"id":           json.Number("7"),
		"category_id":  json.Number("42"),
		"created_at":   "2026-10-24T15:04:05Z",
		"access_token": "secret",
		"full_name":    nil,
		"sku":          "SKU-AB12CD34",
	}
	got := normalize(raw, s, "").(map[string]any)
	if got["id"] != "<id>" {
		t.Errorf("id = %v", got["id"])
	}
	if got["category_id"] != "<category_id>" {
		t.Errorf("category_id = %v", got["category_id"])
	}
	if got["created_at"] != "<timestamp>" {
		t.Errorf("created_at = %v", got["created_at"])
	}
	if got["access_token"] != "<token>" {
		t.Errorf("access_token = %v", got["access_token"])
	}
	if got["full_name"] != "" {
		t.Errorf("full_name = %v", got["full_name"])
	}
	if got["sku"] != "<generated-sku>" {
		t.Errorf("sku = %v", got["sku"])
	}
}

func TestMoneyComparesAsDecimal(t *testing.T) {
	if details := diffValues("$", json.Number("21.20"), json.Number("21.2"), false); len(details) != 0 {
		t.Fatalf("money mismatch: %v", details)
	}
	if details := diffValues("$", json.Number("21.20"), json.Number("21.21"), false); len(details) == 0 {
		t.Fatal("expected a money mismatch")
	}
}

func TestRenderReport(t *testing.T) {
	report := renderReport("http://legacy.test", []failure{{
		Module:  "users",
		Fixture: "list users",
		Request: "GET /api/v1/users",
		Details: []string{"$.items[0].id: legacy=<id> go=<id>"},
	}}, []skipNote{{Module: "users", Fixture: "delete user", Reason: "fixture already seeded"}})

	for _, want := range []string{
		"# Contract parity report",
		"http://legacy.test",
		"users / list users",
		"GET /api/v1/users",
		"fixture already seeded",
		"$.items[0].id",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report missing %q", want)
		}
	}
}

func TestRedactRecordedTokens(t *testing.T) {
	body := []byte(`{"access_token":"a","refresh_token":"b","user":{"email":"u@example.com"}}`)
	got := string(redactRecorded(body))
	if strings.Contains(got, `"a"`) || strings.Contains(got, `"b"`) {
		t.Fatalf("tokens were not redacted: %s", got)
	}
	if !strings.Contains(got, "u@example.com") {
		t.Fatalf("non-token field was removed: %s", got)
	}
}
