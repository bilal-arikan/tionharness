package api

import (
	"context"
	"errors"

	"github.com/bilal-arikan/tionharness/internal/agent"
)

// Stable machine tags for a chat turn that ended without a clean reply. They are
// rendered as a badge on the persisted error card and carried on the terminal
// hub event, so keep them byte-stable.
const (
	reasonProviderError   = "provider_error"
	reasonTurnHardTimeout = "turn_hard_timeout"
	reasonTurnIdleTimeout = "turn_idle_timeout"
	reasonStopped         = "stopped"
)

// chatTurnFailure maps a failed chat turn to the user-visible detail text and the
// machine tag that go onto the persisted error card. cause is the context's
// cancellation cause (which is what distinguishes a watchdog cut from a user
// Stop — both surface as context.Canceled on ctx.Err()); err is the completion
// error the tool loop returned.
//
// The idle case is deliberately explicit about WHERE it stalled: the turn did not
// fail, the provider simply stopped sending — the case the inactivity watchdog
// exists for.
func chatTurnFailure(cause, err error) (detail, reason string) {
	switch {
	case errors.Is(cause, agent.ErrTurnHardTimeout):
		return "Sohbet turu mutlak süre sınırına ulaştı. O ana kadarki yanıt korundu.", reasonTurnHardTimeout
	case errors.Is(cause, agent.ErrTurnIdleTimeout):
		return "Sağlayıcı akışı takıldı: tur, etkinlik penceresi boyunca hiçbir adım üretmedi ve geri alındı. O ana kadarki yanıt korundu.", reasonTurnIdleTimeout
	case errors.Is(cause, context.Canceled):
		return "Tur manuel olarak durduruldu. O ana kadarki adımlar korundu.", reasonStopped
	default:
		return "provider error: " + err.Error(), reasonProviderError
	}
}
