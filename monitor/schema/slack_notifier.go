package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type SlackNotifier struct {
	webhookURL string
}

func NewSlackNotifier(webhookURL string) *SlackNotifier {
	return &SlackNotifier{webhookURL: webhookURL}
}

// NotifySchemaChange formats and sends schema change alerts.
// The message structure emphasizes:
// - Visual prominence with emojis for quick attention
// - Hash comparisons for easy diffing
// - Action items to guide next steps
func (n *SlackNotifier) NotifySchemaChange(provider string, oldHash, newHash string) error {
	message := map[string]interface{}{
		"blocks": []map[string]interface{}{
			{
				"type": "header",
				"text": map[string]string{
					"type": "plain_text",
					"text": fmt.Sprintf("🚨 %s API Schema Change Detected", provider),
				},
			},
			{
				"type": "section",
				"text": map[string]string{
					"type": "mrkdwn",
					"text": fmt.Sprintf("*Previous Hash:* `%s`\n*New Hash:* `%s`", oldHash, newHash),
				},
			},
			{
				"type": "context",
				"elements": []map[string]string{
					{
						"type": "mrkdwn",
						"text": "Please review the changes and update the integration if needed.",
					},
				},
			},
		},
	}

	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("failed to marshal slack message: %w", err)
	}

	resp, err := http.Post(n.webhookURL, "application/json", bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to send slack notification: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack notification failed with status: %d", resp.StatusCode)
	}

	return nil
}
