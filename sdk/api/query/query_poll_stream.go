package query

import "context"

// ExecuteAndPollStream is ExecuteAndPoll with the terminal response's rows
// delivered to onRecord instead of accumulated, so peak memory does not grow
// with the result size. The returned Response carries state and metadata with
// Records / Result.Records left nil.
//
// Rows belonging to a still-running response (the preview snapshots
// EnablePreview asks for) are discarded rather than forwarded: a preview is a
// whole snapshot, not a delta, so forwarding one would deliver rows the final
// response repeats.
func (h *Handler) ExecuteAndPollStream(ctx context.Context, req ExecuteRequest, opts ExecuteAndPollOptions, onRecord func(map[string]interface{}) error) (*Response, error) {
	if onRecord == nil {
		onRecord = func(map[string]interface{}) error { return nil }
	}
	return h.executeAndPoll(ctx, req, opts, onRecord)
}

// executeFor runs query:execute, accumulating rows into the Response when
// stream is nil and streaming them through the terminal gate otherwise.
func (h *Handler) executeFor(ctx context.Context, req ExecuteRequest, stream func(map[string]interface{}) error) (*Response, error) {
	if stream == nil {
		return h.Execute(ctx, req)
	}
	gate := &terminalGate{onRecord: stream}
	resp, err := h.executeRaw(ctx, req, gate.sink())
	if err != nil {
		return nil, err
	}
	return resp, gate.finish(resp)
}

// pollFor is executeFor for query:poll.
func (h *Handler) pollFor(ctx context.Context, requestToken string, timeoutMs int64, enrich bool, stream func(map[string]interface{}) error) (*Response, error) {
	if stream == nil {
		return h.Poll(ctx, requestToken, timeoutMs, enrich)
	}
	gate := &terminalGate{onRecord: stream}
	resp, err := h.pollRaw(ctx, requestToken, timeoutMs, enrich, gate.sink())
	if err != nil {
		return nil, err
	}
	return resp, gate.finish(resp)
}

// terminalGate forwards streamed rows only for a response that carries the
// final result, dropping the preview rows of a still-running one.
//
// The decision needs the response's "state", which the decoder reports as it
// goes. Grail emits it ahead of the rows, so the gate normally decides on the
// first row and forwards from there. A response that puts the rows first
// instead is held until the state arrives, which costs the memory saving for
// that response but never correctness.
type terminalGate struct {
	onRecord func(map[string]interface{}) error
	held     []map[string]interface{}
	decided  bool
	forward  bool
}

func (g *terminalGate) sink() recordSink {
	return recordSink{onTop: g.observe, onResult: g.observe}
}

func (g *terminalGate) observe(state string, row map[string]interface{}) error {
	if !g.decided {
		if state == "" {
			g.held = append(g.held, row)
			return nil
		}
		if err := g.decide(state == StateSucceeded); err != nil {
			return err
		}
	}
	if !g.forward {
		return nil
	}
	return g.onRecord(row)
}

// finish settles a gate that never saw a state during decoding: the response is
// terminal if it succeeded, or if it declares no state and no request token
// (the pre-state synchronous shape, which carries its records directly).
func (g *terminalGate) finish(resp *Response) error {
	if g.decided {
		return nil
	}
	return g.decide(resp.State == StateSucceeded || (resp.State == "" && resp.RequestToken == ""))
}

func (g *terminalGate) decide(forward bool) error {
	g.decided, g.forward = true, forward
	held := g.held
	g.held = nil
	if !forward {
		return nil
	}
	for _, row := range held {
		if err := g.onRecord(row); err != nil {
			return err
		}
	}
	return nil
}
