package decider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSystemOneRequestShapeAndTolerantAnswers(t *testing.T) {
	var got map[string]any
	var path, auth string
	// An OpenJev-style answer: no type fields, a score distribution as an
	// array, no id, no cost.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		_, _ = io.WriteString(w, `{"model":"dgemma","answers":{
			"ok":{"noul":0.83},
			"pick":{"choice":"b","probabilities":{"a":0.1,"b":0.9},"confidence":0.8},
			"level":{"probabilities":[0.1,0.2,0.7],"legend":{"0":"low"}}
		},"usage":{"input_tokens":321,"output_tokens":0}}`)
	}))
	defer srv.Close()

	d, err := systemOneBackend{}.New(Endpoint{BaseURL: srv.URL + "/v1", Authorize: bearer("tk")}, ClientOptions{Model: OpenJevModel})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := d.Decide(context.Background(), Request{State: "s", Questions: map[string]Question{
		"ok":    Noul("fine?", "yes", ""),
		"pick":  Choice("which?", map[string]string{"a": "A", "b": "B"}),
		"level": Score("how much?", "low", "mid", "high"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1/systemone" || auth != "Bearer tk" || got["model"] != OpenJevModel {
		t.Errorf("path = %q auth = %q model = %v", path, auth, got["model"])
	}
	if c := got["questions"].(map[string]any)["ok"].(map[string]any)["criteria"].(map[string]any); c["false"] == "" {
		t.Errorf("half noul criteria not completed: %v", c)
	}
	if resp.Answers["ok"].Probability != 0.83 || resp.Answers["pick"].Choice != "b" {
		t.Errorf("answers = %+v", resp.Answers)
	}
	if lv := resp.Answers["level"]; lv.Level() != 2 || lv.Probabilities["2"] != 0.7 {
		t.Errorf("array score distribution = %+v", lv)
	}
	if resp.Backend != SystemOneBackendID || resp.ServedModel != "dgemma" || resp.BillingProvider != billingLocal || resp.Usage.InputTokens != 321 {
		t.Errorf("accounting = %+v", resp)
	}
}

func TestSystemOneURL(t *testing.T) {
	cases := map[string]string{
		"":                                  "https://api.typesafe.ai/v1/systemone",
		"https://api.typesafe.ai/v1":        "https://api.typesafe.ai/v1/systemone",
		"https://api.typesafe.ai":           "https://api.typesafe.ai/v1/systemone",
		"http://127.0.0.1:8080/v1/":         "http://127.0.0.1:8080/v1/systemone",
		"https://openrouter.ai/api/v1":      "https://openrouter.ai/api/v1/systemone",
		"http://gpu.lan:3000/v1/systemone/": "http://gpu.lan:3000/v1/systemone",
	}
	for in, want := range cases {
		got, err := SystemOneURL(in)
		if err != nil || got != want {
			t.Errorf("SystemOneURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := SystemOneURL("api.typesafe.ai"); err == nil {
		t.Error("a base without a scheme was accepted")
	}
}

func TestSystemOneBillingFollowsTheHost(t *testing.T) {
	cases := []struct {
		url, kind, want string
	}{
		{"https://api.typesafe.ai/v1/systemone", "", billingTypeSafe},
		{"https://openrouter.ai/api/v1/systemone", "openrouter", billingOpenRouter},
		{"http://127.0.0.1:8080/v1/systemone", "", billingLocal},
		{"http://192.168.1.20:8080/v1/systemone", "", billingLocal},
		{"http://gpu.local:8080/v1/systemone", "", billingLocal},
		{"https://decisions.example.com/v1/systemone", "", ""},
		{"https://proxy.example.com/v1/systemone", "openai-compat", "openai-compat"},
	}
	for _, c := range cases {
		if got := billingProviderFor(c.url, c.kind); got != c.want {
			t.Errorf("billingProviderFor(%q, %q) = %q, want %q", c.url, c.kind, got, c.want)
		}
	}
	// On OpenRouter a bare TypeSafe id is priced under the typesafe/ namespace.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"answers":{"q":{"type":"noul","noul":0.5}}}`)
	}))
	defer srv.Close()
	c := &systemOneClient{call: systemOneCall{prefix: systemOneErrPrefix, url: srv.URL, model: JevPinnedModel}, billing: billingOpenRouter}
	resp, err := c.Decide(context.Background(), Request{State: "s", Questions: map[string]Question{"q": Noul("?", "", "")}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.BilledModel() != "typesafe/jev-1.13" {
		t.Errorf("billed model = %q", resp.BilledModel())
	}
}

func TestSystemOneAcceptsOnlyOpenRouterCredentials(t *testing.T) {
	b := systemOneBackend{}
	if !b.Accepts("openrouter", "") || b.Accepts("lmstudio", "http://localhost:1234/v1") {
		t.Error("wrong provider kinds accepted")
	}
}

// A borrowed provider endpoint with an empty base URL means the provider
// kind's default host, never the backend's: an OpenRouter key must not be
// sent to TypeSafe because the provider form left the field empty.
func TestBorrowedEndpointNeverFallsBackToTheBackendDefault(t *testing.T) {
	so, err := systemOneBackend{}.New(Endpoint{InstanceID: "PRV1", Kind: "openrouter"}, ClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got := so.(*systemOneClient).call.url; got != "https://openrouter.ai/api/v1/systemone" {
		t.Errorf("borrowed OpenRouter endpoint = %q", got)
	}
	if got := so.(*systemOneClient).billing; got != billingOpenRouter {
		t.Errorf("billing = %q", got)
	}
	own, _ := systemOneBackend{}.New(Endpoint{}, ClientOptions{})
	if got := own.(*systemOneClient).call.url; got != "https://api.typesafe.ai/v1/systemone" {
		t.Errorf("own endpoint default = %q", got)
	}
	lp, err := logprobsBackend{}.New(Endpoint{InstanceID: "PRV2", Kind: "lmstudio"}, ClientOptions{Model: "qwen3-8b"})
	if err != nil {
		t.Fatal(err)
	}
	if got := lp.(*logprobsClient).url; got != "http://127.0.0.1:1234/v1/chat/completions" {
		t.Errorf("borrowed LM Studio endpoint = %q", got)
	}
	if _, err := (logprobsBackend{}).New(Endpoint{InstanceID: "PRV3", Kind: "openai-compat"}, ClientOptions{Model: "m"}); err == nil {
		t.Error("a borrowed endpoint without any base URL fell back to a default host")
	}
}
