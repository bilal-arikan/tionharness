package providers

// addUsage returns the field-by-field sum of two usage records.
//
// EVERY field is carried, including the derived ones — the counters by addition
// and ThinkingTokensMeasured by OR. CacheWrite5mTokens /
// CacheWrite1hTokens are a breakdown of CacheWriteTokens and ThinkingTokens is a
// share of OutputTokens: summing a breakdown across attempts keeps it a
// breakdown of the summed total, so the billing invariant ("never add these on
// top") still holds for the result.
func addUsage(a, b Usage) Usage {
	return Usage{
		InputTokens:        a.InputTokens + b.InputTokens,
		OutputTokens:       a.OutputTokens + b.OutputTokens,
		CacheReadTokens:    a.CacheReadTokens + b.CacheReadTokens,
		CacheWriteTokens:   a.CacheWriteTokens + b.CacheWriteTokens,
		CacheWrite5mTokens: a.CacheWrite5mTokens + b.CacheWrite5mTokens,
		CacheWrite1hTokens: a.CacheWrite1hTokens + b.CacheWrite1hTokens,
		ThinkingTokens:     a.ThinkingTokens + b.ThinkingTokens,
		// Not a counter: the flag says "a provider really reported this", so it
		// survives if EITHER side measured. Dropping it would let the agent layer
		// overwrite a measured value with an estimate (see
		// preserveOrDeriveThinkingTokens).
		ThinkingTokensMeasured: a.ThinkingTokensMeasured || b.ThinkingTokensMeasured,
	}
}

// foldFailedAttempts folds the billable usage of earlier, discarded attempts
// into the response a later attempt succeeded with.
//
// A retry loop returns (resp, nil) and drops the failed attempts' errors on the
// floor — together with the UsageError they carry. Those tokens were really
// spent: the request reached the provider, it answered with a rejection or died
// mid-stream, and the input (plus every completed internal round-trip) is billed
// either way. The successful Response is the ONLY thing the caller sees and the
// only thing it bills, so the lost usage has to arrive there or it is never
// billed at all.
//
// This cannot double-bill: the folded errors are the ones the retry loop is
// about to discard, so nothing upstream ever sees them as failures of their own.
func foldFailedAttempts(resp *Response, failed []error) {
	if resp == nil {
		return
	}
	for _, err := range failed {
		ue, ok := UsageFromError(err)
		if !ok {
			continue
		}
		resp.Usage = addUsage(resp.Usage, ue.Usage)
		resp.ProviderCalls += ue.ProviderCalls
	}
}
