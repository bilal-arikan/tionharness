package api

import "testing"

func TestFeedURL(t *testing.T) {
	originalBase := FeedBaseURL
	t.Cleanup(func() { FeedBaseURL = originalBase })

	t.Run("environment override", func(t *testing.T) {
		t.Setenv("TIONHARNESS_FEED_URL", "http://localhost:8080/feed")
		if got := FeedURL(); got != "http://localhost:8080/feed" {
			t.Fatalf("FeedURL() = %q, want %q", got, "http://localhost:8080/feed")
		}
	})

	t.Run("empty environment uses base", func(t *testing.T) {
		FeedBaseURL = "https://updates.example.com"
		t.Setenv("TIONHARNESS_FEED_URL", "")
		if got := FeedURL(); got != "https://updates.example.com" {
			t.Fatalf("FeedURL() = %q, want %q", got, "https://updates.example.com")
		}
	})

	t.Run("trims trailing slashes", func(t *testing.T) {
		t.Setenv("TIONHARNESS_FEED_URL", "https://updates.example.com///")
		if got := FeedURL(); got != "https://updates.example.com" {
			t.Fatalf("FeedURL() = %q, want %q", got, "https://updates.example.com")
		}
	})
}
