package codexauth

import "testing"

// devicePromptRaw is the verbatim output captured live against codex-cli
// 0.147.0 (see the session scratchpad), ANSI escapes included, exactly as the
// CLI emits them around the URL and the one-time code.
const devicePromptRaw = "Welcome to Codex [v0.147.0]\n" +
	"OpenAI's command-line coding agent\n" +
	"\n" +
	"Follow these steps to sign in with ChatGPT using device code authorization:\n" +
	"\n" +
	"1. Open this link in your browser and sign in to your account\n" +
	"   \x1b[94mhttps://auth.openai.com/codex/device\x1b[0m\n" +
	"\n" +
	"2. Enter this one-time code (expires in 15 minutes)\n" +
	"   \x1b[94mZ937-61BWU\x1b[0m\n" +
	"\n" +
	"\x1b[90mContinue only if you started this login in Codex. If a website or another\n" +
	"person gave you this code, cancel.\x1b[0m\n"

func TestStripANSI(t *testing.T) {
	got := StripANSI("\x1b[94mhttps://auth.openai.com/codex/device\x1b[0m")
	want := "https://auth.openai.com/codex/device"
	if got != want {
		t.Fatalf("StripANSI() = %q, want %q", got, want)
	}
}

func TestStripANSINoEscapes(t *testing.T) {
	got := StripANSI("plain text, no escapes")
	if got != "plain text, no escapes" {
		t.Fatalf("StripANSI() changed plain text: %q", got)
	}
}

func TestParseDevicePromptVerbatim(t *testing.T) {
	url, code, ok := ParseDevicePrompt(devicePromptRaw)
	if !ok {
		t.Fatalf("ParseDevicePrompt() ok = false, want true")
	}
	if url != "https://auth.openai.com/codex/device" {
		t.Errorf("url = %q, want https://auth.openai.com/codex/device", url)
	}
	if code != "Z937-61BWU" {
		t.Errorf("code = %q, want Z937-61BWU", code)
	}
}

func TestParseDevicePromptMalformedOutput(t *testing.T) {
	cases := []string{
		"",
		"Welcome to Codex [v0.147.0]\nOpenAI's command-line coding agent\n",
		"1. Open this link in your browser and sign in to your account\n\n2. Enter this one-time code\n",
		"garbage garbage garbage",
	}
	for _, c := range cases {
		if _, _, ok := ParseDevicePrompt(c); ok {
			t.Errorf("ParseDevicePrompt(%q) ok = true, want false", c)
		}
	}
}

func TestParseDevicePromptDifferentHostAndCodeShape(t *testing.T) {
	// The CLI may rotate the host or the code's exact grouping; the loose
	// fallback pattern must still accept a differently-shaped but plausible
	// value instead of only matching the one example we captured.
	out := "1. Open this link in your browser and sign in to your account\n" +
		"   https://auth.example.org/device/verify\n" +
		"2. Enter this one-time code (expires in 15 minutes)\n" +
		"   AB12-XYZ99\n"
	url, code, ok := ParseDevicePrompt(out)
	if !ok {
		t.Fatalf("ParseDevicePrompt() ok = false, want true")
	}
	if url != "https://auth.example.org/device/verify" {
		t.Errorf("url = %q", url)
	}
	if code != "AB12-XYZ99" {
		t.Errorf("code = %q", code)
	}
}
