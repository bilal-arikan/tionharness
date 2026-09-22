package announce

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Target is one chat destination the announcement is posted to.
type Target string

const (
	// TargetDiscord posts to a Discord webhook URL as a plain content message.
	TargetDiscord Target = "discord"
	// TargetTelegram posts through the Bot API sendMessage endpoint.
	TargetTelegram Target = "telegram"
)

// ParseTarget validates a target name.
func ParseTarget(s string) (Target, error) {
	switch Target(strings.ToLower(strings.TrimSpace(s))) {
	case TargetDiscord:
		return TargetDiscord, nil
	case TargetTelegram:
		return TargetTelegram, nil
	default:
		return "", fmt.Errorf("unknown announce target %q (use discord or telegram)", s)
	}
}

// maxResponseBytes bounds how much of a webhook's error body is read back into
// the agent's context.
const maxResponseBytes = 4 * 1024

// Post delivers body to one target.
//
//   - discord: endpoint is the full webhook URL; chatID is unused.
//   - telegram: endpoint is the bot token URL base (https://api.telegram.org/bot<token>)
//     and chatID names the channel or group.
//
// A non-2xx response is an ERROR carrying the provider's own message. A silent
// success on a rejected post would let a release go unannounced while the agent
// reports it delivered.
func Post(ctx context.Context, client *http.Client, target Target, endpoint, chatID, body string) error {
	if strings.TrimSpace(endpoint) == "" {
		return fmt.Errorf("announce: no webhook endpoint configured for %s", target)
	}
	if _, err := url.Parse(endpoint); err != nil {
		return fmt.Errorf("announce: invalid %s endpoint: %w", target, err)
	}

	var reqURL string
	var payload []byte
	var err error
	switch target {
	case TargetDiscord:
		reqURL = endpoint
		payload, err = json.Marshal(map[string]string{"content": body})
	case TargetTelegram:
		if strings.TrimSpace(chatID) == "" {
			return fmt.Errorf("announce: telegram needs a chat id")
		}
		reqURL = strings.TrimRight(endpoint, "/") + "/sendMessage"
		payload, err = json.Marshal(map[string]any{
			"chat_id": chatID,
			"text":    body,
			// Markdown so the "**bold**" headings the renderer emits are shown as
			// formatting rather than literal asterisks, matching Discord.
			"parse_mode":               "Markdown",
			"disable_web_page_preview": true,
		})
	default:
		return fmt.Errorf("announce: unknown target %q", target)
	}
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "tionharness/0.0.1")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("announce: posting to %s failed: %w", target, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		return fmt.Errorf("announce: %s rejected the post (HTTP %d): %s",
			target, resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}
