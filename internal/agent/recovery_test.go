package agent

import (
	"errors"
	"testing"

	"github.com/bilal-arikan/swarmgo/internal/providers"
)

func TestDecideRecovery(t *testing.T) {
	overflow := errors.New("prompt is too long: 250000 tokens > 200000 maximum")
	other := errors.New("connection refused")

	cfg := recoveryConfig{maxTokenLimit: DefaultMaxTokenRetries, reactiveCompact: true}

	tests := []struct {
		name        string
		resp        *providers.Response
		err         error
		st          loopState
		wantCont    bool
		wantCompact bool
		wantReason  contReason
		wantInject  bool
		wantTerm    termReason
	}{
		{
			name:       "max tokens first attempt resumes",
			resp:       &providers.Response{StopReason: providers.StopMaxTok},
			st:         loopState{},
			wantCont:   true,
			wantReason: contMaxTokenResume,
			wantInject: true,
		},
		{
			name:     "max tokens exhausted surfaces partial",
			resp:     &providers.Response{StopReason: providers.StopMaxTok},
			st:       loopState{maxTokenRetries: DefaultMaxTokenRetries},
			wantTerm: termMaxTokenExhausted,
		},
		{
			name:     "normal end turn completes",
			resp:     &providers.Response{StopReason: providers.StopEndTurn},
			st:       loopState{},
			wantTerm: termCompleted,
		},
		{
			name:        "context overflow compacts once",
			err:         overflow,
			st:          loopState{},
			wantCompact: true,
			wantReason:  contCompactRetry,
		},
		{
			name:     "context overflow after compaction is terminal",
			err:      overflow,
			st:       loopState{compacted: true},
			wantTerm: termProviderErr,
		},
		{
			name:     "unrelated error is terminal",
			err:      other,
			st:       loopState{},
			wantTerm: termProviderErr,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := decideRecovery(tc.resp, tc.err, tc.st, cfg)
			assertDecision(t, d, tc.wantCont, tc.wantCompact, tc.wantReason, tc.wantInject, tc.wantTerm)
		})
	}

	// Toggle off: a context overflow is terminal even on the first attempt.
	t.Run("reactive compaction disabled is terminal", func(t *testing.T) {
		d := decideRecovery(nil, overflow, loopState{}, recoveryConfig{maxTokenLimit: DefaultMaxTokenRetries, reactiveCompact: false})
		if d.compact {
			t.Errorf("compact = true with reactiveCompact off, want terminal")
		}
		if d.term != termProviderErr {
			t.Errorf("term = %q, want %q", d.term, termProviderErr)
		}
	})

	// Resume budget 0: an output cap surfaces the partial immediately.
	t.Run("max token retries zero disables resume", func(t *testing.T) {
		d := decideRecovery(&providers.Response{StopReason: providers.StopMaxTok}, nil, loopState{}, recoveryConfig{maxTokenLimit: 0, reactiveCompact: true})
		if d.cont {
			t.Errorf("cont = true with maxTokenLimit 0, want terminal")
		}
		if d.term != termMaxTokenExhausted {
			t.Errorf("term = %q, want %q", d.term, termMaxTokenExhausted)
		}
	})
}

func assertDecision(t *testing.T, d decision, wantCont, wantCompact bool, wantReason contReason, wantInject bool, wantTerm termReason) {
	t.Helper()
	if d.cont != wantCont {
		t.Errorf("cont = %v, want %v", d.cont, wantCont)
	}
	if d.compact != wantCompact {
		t.Errorf("compact = %v, want %v", d.compact, wantCompact)
	}
	if (wantCont || wantCompact) && d.reason != wantReason {
		t.Errorf("reason = %q, want %q", d.reason, wantReason)
	}
	if (d.inject != nil) != wantInject {
		t.Errorf("inject present = %v, want %v", d.inject != nil, wantInject)
	}
	if !wantCont && !wantCompact && d.term != wantTerm {
		t.Errorf("term = %q, want %q", d.term, wantTerm)
	}
	// A terminal provider error must carry the error through.
	if wantTerm == termProviderErr && d.err == nil {
		t.Errorf("provider_error decision dropped the error")
	}
}

func TestIsContextOverflow(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"prompt is too long: 250000 tokens > 200000 maximum", true},
		{"This model's maximum context length is 200000 tokens", true},
		{"context_length_exceeded", true},
		{"connection refused", false},
		{"minimax HTTP 401: unauthorized", false},
	}
	for _, tc := range tests {
		if got := isContextOverflow(errors.New(tc.msg)); got != tc.want {
			t.Errorf("isContextOverflow(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
	if isContextOverflow(nil) {
		t.Errorf("isContextOverflow(nil) = true, want false")
	}
}
