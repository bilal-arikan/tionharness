package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bilal-arikan/tionharness/internal/announce"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/secrets"
)

// Secret names the announcement tool reads its endpoints from. They live in the
// workspace vault rather than the tool arguments so a webhook URL — which is a
// credential, anyone holding it can post as the project — never passes through
// a model's context or a transcript.
const (
	discordSecret     = "ANNOUNCE_DISCORD_WEBHOOK"
	telegramSecret    = "ANNOUNCE_TELEGRAM_API"
	telegramChatIDKey = "ANNOUNCE_TELEGRAM_CHAT_ID"
)

// releaseFilePath is where the changelog generator writes the machine-readable
// description of the LATEST release (see internal/changelog, _Docs/75).
const releaseFilePath = "_Docs/release.json"

// AnnounceReleaseTool posts the current release to the project's chat channels.
// It is the agent half of the release announcement fan-out (_Docs/86): a
// schedule wakes an agent, the agent calls this, and TionHarness announces its
// own release instead of a CI bot doing it.
//
// The tool reads _Docs/release.json rather than taking the text as an argument:
// the announcement must describe what was actually released, and a model asked
// to retype a changelog will eventually paraphrase a version number.
type AnnounceReleaseTool struct {
	sb     Sandbox
	vault  *secrets.Vault
	client *http.Client
}

// NewAnnounceReleaseTool binds the tool to the workspace sandbox (where
// release.json is read from) and the secret vault (where the endpoints live).
func NewAnnounceReleaseTool(sb Sandbox, vault *secrets.Vault) AnnounceReleaseTool {
	// Webhook endpoints are public chat services; the shared SSRF guard keeps a
	// mis-set secret from turning this into an internal-network probe.
	return AnnounceReleaseTool{sb: sb, vault: vault, client: newGuardedHTTPClient(30 * time.Second)}
}

func (AnnounceReleaseTool) Def() providers.ToolDef {
	return providers.ToolDef{
		Name: "announce_release",
		Description: "Announce the CURRENT release to the project's chat channels (Discord, Telegram). " +
			"Reads " + releaseFilePath + " — written by the release pipeline — and renders a short " +
			"headline with the breaking changes, features and fixes, so the announcement always matches " +
			"what actually shipped. Set `targets` to choose channels (default: every one that has an " +
			"endpoint configured). Endpoints come from the secret vault (" + discordSecret + ", " +
			telegramSecret + " + " + telegramChatIDKey + "), never from arguments. Use `dry_run` to see " +
			"the exact message without posting it. Posting is NOT idempotent: calling this twice " +
			"announces the same release twice.",
		InputSchema: json.RawMessage(`{
			"type":"object",
			"properties":{
				"targets":{"type":"array","items":{"type":"string","enum":["discord","telegram"]},"description":"Channels to post to. Omit for every configured channel."},
				"notes_url":{"type":"string","description":"Link to the full release notes, appended to the message."},
				"dry_run":{"type":"boolean","description":"Render and return the message without posting it."}
			},
			"additionalProperties":false
		}`),
		Examples: []json.RawMessage{
			json.RawMessage(`{"dry_run":true}`),
			json.RawMessage(`{"targets":["discord"],"notes_url":"https://tionharness.com/releases/v1.2.3"}`),
		},
	}
}

func (t AnnounceReleaseTool) Call(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Targets  []string `json:"targets"`
		NotesURL string   `json:"notes_url"`
		DryRun   bool     `json:"dry_run"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", argErrFor("announce_release", err)
	}

	release, err := t.loadRelease()
	if err != nil {
		return "", err
	}
	body := announce.Render(release, strings.TrimSpace(args.NotesURL))

	targets, err := t.resolveTargets(args.Targets)
	if err != nil {
		return "", err
	}
	if args.DryRun {
		return fmt.Sprintf("Dry run — would post to %s:\n\n%s",
			strings.Join(targetNames(targets), ", "), body), nil
	}

	var posted []string
	for _, tg := range targets {
		endpoint, chatID := t.endpointFor(tg)
		if err := announce.Post(ctx, t.client, tg, endpoint, chatID, body); err != nil {
			// Report what already went out: the caller must not retry blindly and
			// double-post to the channels that succeeded.
			if len(posted) > 0 {
				return "", fmt.Errorf("%w (already announced on %s — do not retry those)",
					err, strings.Join(posted, ", "))
			}
			return "", err
		}
		posted = append(posted, string(tg))
	}
	return fmt.Sprintf("Announced %s on %s.", release.Tag, strings.Join(posted, ", ")), nil
}

// loadRelease reads and parses release.json from the workspace.
func (t AnnounceReleaseTool) loadRelease() (announce.Release, error) {
	path, err := t.sb.Resolve(releaseFilePath)
	if err != nil {
		return announce.Release{}, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return announce.Release{}, fmt.Errorf(
				"%s does not exist — it is written by the release pipeline, so there is nothing to announce yet", releaseFilePath)
		}
		return announce.Release{}, err
	}
	var r announce.Release
	if err := json.Unmarshal(raw, &r); err != nil {
		return announce.Release{}, fmt.Errorf("%s is not valid release JSON: %w", releaseFilePath, err)
	}
	if strings.TrimSpace(r.Tag) == "" {
		return announce.Release{}, fmt.Errorf("%s carries no tag — refusing to announce an unnamed release", releaseFilePath)
	}
	return r, nil
}

// resolveTargets turns the requested names into targets, defaulting to every
// channel that actually has an endpoint configured.
//
// An explicitly requested channel with no endpoint is an ERROR, not a skip: the
// caller asked for it, and silently announcing to fewer channels than asked is
// how a release goes unannounced without anyone noticing.
func (t AnnounceReleaseTool) resolveTargets(names []string) ([]announce.Target, error) {
	if len(names) > 0 {
		out := make([]announce.Target, 0, len(names))
		for _, n := range names {
			tg, err := announce.ParseTarget(n)
			if err != nil {
				return nil, err
			}
			if endpoint, _ := t.endpointFor(tg); endpoint == "" {
				return nil, fmt.Errorf("%s was requested but has no endpoint in the vault (set %s)",
					tg, secretNameFor(tg))
			}
			out = append(out, tg)
		}
		return out, nil
	}

	var out []announce.Target
	for _, tg := range []announce.Target{announce.TargetDiscord, announce.TargetTelegram} {
		if endpoint, _ := t.endpointFor(tg); endpoint != "" {
			out = append(out, tg)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no announcement channel is configured: store %s or %s (+ %s) in the secret vault",
			discordSecret, telegramSecret, telegramChatIDKey)
	}
	return out, nil
}

// endpointFor reads a target's endpoint (and chat id, for Telegram) from the
// vault. A nil vault yields empty values, which resolveTargets reports.
func (t AnnounceReleaseTool) endpointFor(tg announce.Target) (endpoint, chatID string) {
	if t.vault == nil {
		return "", ""
	}
	switch tg {
	case announce.TargetDiscord:
		endpoint, _ = t.vault.Get(discordSecret)
	case announce.TargetTelegram:
		endpoint, _ = t.vault.Get(telegramSecret)
		chatID, _ = t.vault.Get(telegramChatIDKey)
	}
	return endpoint, chatID
}

// secretNameFor names the vault entry a target needs, for the error message.
func secretNameFor(tg announce.Target) string {
	if tg == announce.TargetTelegram {
		return telegramSecret + " + " + telegramChatIDKey
	}
	return discordSecret
}

// targetNames renders targets for a message.
func targetNames(targets []announce.Target) []string {
	out := make([]string, 0, len(targets))
	for _, tg := range targets {
		out = append(out, string(tg))
	}
	return out
}
