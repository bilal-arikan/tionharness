package agent

import (
	"errors"
	"testing"

	"github.com/bilal/swarmgo/internal/providers"
)

func TestDecideRecovery(t *testing.T) {
	overflow := errors.New("prompt is too long: 250000 tokens > 200000 maximum")
	other := errors.New("connection refused")

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
			st:       loopState{maxTokenRetries: maxTokenRetryLimit},
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
			d := decideRecovery(tc.resp, tc.err, tc.st)
			if d.cont != tc.wantCont {
				t.Errorf("cont = %v, want %v", d.cont, tc.wantCont)
			}
			if d.compact != tc.wantCompact {
				t.Errorf("compact = %v, want %v", d.compact, tc.wantCompact)
			}
			if (tc.wantCont || tc.wantCompact) && d.reason != tc.wantReason {
				t.Errorf("reason = %q, want %q", d.reason, tc.wantReason)
			}
			if (d.inject != nil) != tc.wantInject {
				t.Errorf("inject present = %v, want %v", d.inject != nil, tc.wantInject)
			}
			if !tc.wantCont && !tc.wantCompact && d.term != tc.wantTerm {
				t.Errorf("term = %q, want %q", d.term, tc.wantTerm)
			}
			// A terminal provider error must carry the error through.
			if tc.wantTerm == termProviderErr && d.err == nil {
				t.Errorf("provider_error decision dropped the error")
			}
		})
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
