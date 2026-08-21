package agent

import "sync"

// liveCard owns one replaceable live trace card. Its emitter may be called from
// streaming callbacks while another goroutine updates the card, so all state
// mutations are serialized.
type liveCard struct {
	mu     sync.Mutex
	emit   func(TurnStep)
	id     string
	base   TurnStep
	closed bool
}

func serializeStepEmitter(emit func(TurnStep)) func(TurnStep) {
	var mu sync.Mutex
	return func(st TurnStep) {
		mu.Lock()
		defer mu.Unlock()
		emit(st)
	}
}

func cancelLiveCards(cards map[string]*liveCard) {
	for _, card := range cards {
		card.Cancel()
	}
}

func cancelLiveCardsOnPanic(cards *map[string]*liveCard) {
	if recovered := recover(); recovered != nil {
		cancelLiveCards(*cards)
		panic(recovered)
	}
}

// openLive publishes the initial running card.
func openLive(emit func(TurnStep), id string, base TurnStep) *liveCard {
	c := &liveCard{emit: emit, id: id, base: base}
	c.publish(func(st *TurnStep) { st.Running = true })
	return c
}

// Chunk appends streamed text or tool output to the card.
func (c *liveCard) Chunk(text string) {
	c.publish(func(st *TurnStep) {
		st.Running = true
		st.Append = true
		if st.Kind == StepThinking {
			st.Text = text
			st.Output = ""
		} else {
			st.Text = ""
			st.Output = text
		}
	})
}

// Update mutates and republishes the complete running card.
func (c *liveCard) Update(mut func(*TurnStep)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	mut(&c.base)
	st := c.base
	st.ID = c.id
	st.Running = true
	st.Append = false
	c.emit(st)
}

// Close publishes the final replacement and clears all ephemeral flags.
func (c *liveCard) Close(final TurnStep) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	final.ID = c.id
	final.Running = false
	final.Append = false
	c.emit(final)
}

// Cancel retracts an unfinished card and suppresses late updates from workers
// that are still unwinding their cancelled contexts.
func (c *liveCard) Cancel() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	c.emit(TurnStep{Kind: StepTombstone, Ref: c.id})
}

func (c *liveCard) publish(mut func(*TurnStep)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	st := c.base
	st.ID = c.id
	mut(&st)
	c.emit(st)
}
