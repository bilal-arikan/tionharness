package api

import "github.com/bilal-arikan/tionharness/internal/providers"

const steerBufferFullMsg = "steer queue is full: the turn has not consumed the earlier guidance yet"
const steerFinishedMsg = "the turn has finished or stopped; send the message as a new turn"

// steerableForTurn requires a real delivery channel. CLI permission modes do
// not matter: ordinary Interaction MCP results can carry live user input.
func steerableForTurn(provider string, interactionAvailable bool) bool {
	return providers.TransportOf(provider) != providers.TransportCLI || interactionAvailable
}

func (r *chatRun) setSteerable(v bool) {
	r.mu.Lock()
	r.steerable = v
	r.mu.Unlock()
}

func (r *chatRun) steerableFor() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.steerable && !r.steerFinalized && !r.asyncStopped
}

// takeSteerQueue drains the shared FIFO without blocking. API-side consumers
// hold mu; the native model loop is the only other consumer.
func (r *chatRun) takeSteerQueue() []string {
	var out []string
	for {
		select {
		case m := <-r.steer:
			if m != "" {
				out = append(out, m)
			}
		default:
			return out
		}
	}
}

// takeCLISteer competes atomically with turn-end recovery. Ordinary MCP tools
// return these as separate text blocks; permission callbacks join them inside
// additionalContext to preserve their strict JSON contract.
func (r *chatRun) takeCLISteer() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if providers.TransportOf(r.provider) != providers.TransportCLI || r.steerFinalized || r.asyncStopped {
		return nil
	}
	messages := r.takeSteerQueue()
	for i := range messages {
		messages[i] = steerInjectPreamble + messages[i]
	}
	return messages
}

// finishSteer closes admission before draining, so a concurrent sender either
// enters recovery or receives an explicit refusal and keeps its draft.
func (r *chatRun) finishSteer() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.steerFinalized = true
	messages := r.takeSteerQueue()
	if r.asyncStopped {
		return nil // Explicit stop must not restart the session through recovery.
	}
	return messages
}
