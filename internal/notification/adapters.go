package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SlackChannel delivers messages to a Slack Incoming Webhook.
type SlackChannel struct {
	WebhookURL string
	Mention    string // optional @mention prepended to every message
}

func (c *SlackChannel) Kind() string { return "slack" }

func (c *SlackChannel) Send(ctx context.Context, msg Message) error {
	text := msg.Body
	if c.Mention != "" {
		text = c.Mention + " " + text
	}
	payload := map[string]string{"text": text}
	return postJSON(ctx, c.WebhookURL, payload, nil)
}

// TeamsChannel delivers messages to a Microsoft Teams Incoming Webhook
// using the MessageCard format (universally supported).
type TeamsChannel struct {
	WebhookURL string
}

func (c *TeamsChannel) Kind() string { return "teams" }

func (c *TeamsChannel) Send(ctx context.Context, msg Message) error {
	payload := map[string]interface{}{
		"@type":      "MessageCard",
		"@context":   "http://schema.org/extensions",
		"themeColor": "C23B22",
		"summary":    msg.Title,
		"sections": []map[string]interface{}{
			{
				"activityTitle": msg.Title,
				"activityText":  msg.Body,
				"markdown":      true,
			},
		},
	}
	return postJSON(ctx, c.WebhookURL, payload, nil)
}

// WebhookChannel delivers structured JSON payloads to any HTTP endpoint.
// It is the escape hatch for custom integrations (PagerDuty, custom pipelines, etc.).
type WebhookChannel struct {
	URL     string
	Headers map[string]string // e.g. Authorization, X-Custom-Header
}

func (c *WebhookChannel) Kind() string { return "webhook" }

func (c *WebhookChannel) Send(ctx context.Context, msg Message) error {
	payload := map[string]interface{}{
		"rule_id":    msg.RuleID,
		"rule_name":  msg.RuleName,
		"event":      msg.Event,
		"title":      msg.Title,
		"body":       msg.Body,
		"job_id":     msg.JobID,
		"repository": msg.Repository,
		"meta":       msg.Meta,
		"sent_at":    msg.SentAt.Format(time.RFC3339),
	}
	return postJSON(ctx, c.URL, payload, c.Headers)
}

// postJSON marshals payload and POSTs it, applying optional extra headers.
func postJSON(ctx context.Context, url string, payload interface{}, headers map[string]string) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}
	return nil
}
