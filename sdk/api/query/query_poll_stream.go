package query

import "context"

// ExecuteAndPollStream is ExecuteAndPoll with the terminal response's rows
// delivered to onRecord instead of accumulated, so peak memory does not grow
// with the result size. The returned Response carries state and metadata with
// Records / Result.Records left nil.
//
// Rows belonging to a still-running response (the preview snapshots
// EnablePreview asks for) are not forwarded: a preview is a whole snapshot, not
// a delta, so forwarding one would deliver rows the final response repeats.
// They still reach OnUpdate as PollUpdate.Preview when the request set
// EnablePreview and OnUpdate is set, exactly as with ExecuteAndPollWithOptions;
// otherwise they are discarded as they decode.
func (h *Handler) ExecuteAndPollStream(ctx context.Context, req ExecuteRequest, opts ExecuteAndPollOptions, onRecord func(map[string]interface{}) error) (*Response, error) {
	if onRecord == nil {
		onRecord = func(map[string]interface{}) error { return nil }
	}
	return h.executeAndPoll(ctx, req, opts, onRecord)
}

// executeFor runs query:execute, accumulating rows into the Response when
// stream is nil and streaming them through the terminal gate otherwise.
// keepPreview holds a still-running response's rows in it, for OnUpdate.
func (h *Handler) executeFor(ctx context.Context, req ExecuteRequest, stream func(map[string]interface{}) error, keepPreview bool) (*Response, error) {
	if stream == nil {
		return h.Execute(ctx, req)
	}
	gate := &terminalGate{onRecord: stream, keepPreview: keepPreview}
	resp, err := h.executeRaw(ctx, req, gate.sink())
	if err != nil {
		return nil, err
	}
	return resp, gate.finish(resp)
}

// pollFor is executeFor for query:poll.
func (h *Handler) pollFor(ctx context.Context, requestToken string, timeoutMs int64, enrich bool, stream func(map[string]interface{}) error, keepPreview bool) (*Response, error) {
	if stream == nil {
		return h.Poll(ctx, requestToken, timeoutMs, enrich)
	}
	gate := &terminalGate{onRecord: stream, keepPreview: keepPreview}
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
	// keepPreview collects the rows of a response that is not forwarded into
	// preview, so the poll loop can still report them as PollUpdate.Preview.
	keepPreview bool
	preview     recordAccumulator
	held        []heldRow
	decided     bool
	forward     bool
}

// heldRow is a row waiting for the gate's decision, and where it was nested.
type heldRow struct {
	top bool
	row map[string]interface{}
}

func (g *terminalGate) sink() recordSink {
	return recordSink{
		onTop:    func(state string, row map[string]interface{}) error { return g.observe(state, row, true) },
		onResult: func(state string, row map[string]interface{}) error { return g.observe(state, row, false) },
	}
}

func (g *terminalGate) observe(state string, row map[string]interface{}, top bool) error {
	if !g.decided {
		if state == "" {
			g.held = append(g.held, heldRow{top: top, row: row})
			return nil
		}
		if err := g.decide(state == StateSucceeded); err != nil {
			return err
		}
	}
	if !g.forward {
		g.keep(top, row)
		return nil
	}
	return g.onRecord(row)
}

// keep holds a row of a response that is not forwarded, when previews are
// wanted, and drops it otherwise.
func (g *terminalGate) keep(top bool, row map[string]interface{}) {
	if !g.keepPreview {
		return
	}
	if top {
		g.preview.top = append(g.preview.top, row)
	} else {
		g.preview.result = append(g.preview.result, row)
	}
}

// finish settles a gate that never saw a state during decoding: the response is
// terminal if it succeeded, or if it declares no state and no request token
// (the pre-state synchronous shape, which carries its records directly). A
// response that was not forwarded gets the rows kept for its preview.
func (g *terminalGate) finish(resp *Response) error {
	if !g.decided {
		if err := g.decide(resp.State == StateSucceeded || (resp.State == "" && resp.RequestToken == "")); err != nil {
			return err
		}
	}
	if !g.forward && g.keepPreview {
		g.preview.applyTo(resp)
	}
	return nil
}

func (g *terminalGate) decide(forward bool) error {
	g.decided, g.forward = true, forward
	held := g.held
	g.held = nil
	for _, h := range held {
		if !forward {
			g.keep(h.top, h.row)
			continue
		}
		if err := g.onRecord(h.row); err != nil {
			return err
		}
	}
	return nil
}
