package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWebhookPayloadMarshalAfterUTF8Sanitize(t *testing.T) {
	t.Parallel()
	badTitle := string([]byte{0xff, 0xfe, 'M', 'k', 'v'})
	safeTitle := strings.ToValidUTF8(badTitle, "\uFFFD")
	p := WebhookPayload{
		ID:     1,
		Title:  safeTitle,
		Status: statusCompleted,
	}
	if _, err := json.Marshal(p); err != nil {
		t.Fatalf("marshal: %v", err)
	}
}

func TestEffectiveWebhookFormat(t *testing.T) {
	t.Parallel()
	cases := []struct {
		url, explicit, want string
	}{
		{"http://x/hooks/wake", "", formatOpenClawWake},
		{"http://x/Hooks/Wake/foo", "", formatOpenClawWake},
		{"http://x/hooks/agent", "", formatOpenClawAgent},
		{"http://x/hooks/tms", "", formatTMS},
		{"http://x/hooks/wake", "tms", formatTMS},
		{"http://x/", "openclaw_wake", formatOpenClawWake},
		{"http://x/", "wake", formatOpenClawWake},
		{"http://x/", "openclaw_agent", formatOpenClawAgent},
		{"http://x/", "agent", formatOpenClawAgent},
		{"http://x/", "json", formatTMS},
	}
	for _, tc := range cases {
		if got := effectiveWebhookFormat(tc.url, tc.explicit); got != tc.want {
			t.Errorf("effectiveWebhookFormat(%q,%q)=%q want %q", tc.url, tc.explicit, got, tc.want)
		}
	}
}

func TestMarshalWebhookBodyOpenClawWake(t *testing.T) {
	t.Parallel()
	b, err := marshalWebhookBody(formatOpenClawWake, 7, "Show", statusCompleted, "", "e1")
	if err != nil {
		t.Fatal(err)
	}
	var w openclawWakeBody
	if err := json.Unmarshal(b, &w); err != nil {
		t.Fatal(err)
	}
	if w.Text == "" || !strings.Contains(w.Text, "7") || !strings.Contains(w.Text, "Show") {
		t.Fatalf("unexpected text: %q", w.Text)
	}
	if w.Mode != "now" {
		t.Fatalf("mode: %q", w.Mode)
	}
}

func TestMarshalWebhookBodyTMSIncludesEventID(t *testing.T) {
	t.Parallel()
	b, err := marshalWebhookBody(formatTMS, 1, "T", statusFailed, "oops", "evt-uuid")
	if err != nil {
		t.Fatal(err)
	}
	var p WebhookPayload
	if err := json.Unmarshal(b, &p); err != nil {
		t.Fatal(err)
	}
	if p.EventID != "evt-uuid" || p.Status != statusFailed || p.Error != "oops" {
		t.Fatalf("%+v", p)
	}
}
