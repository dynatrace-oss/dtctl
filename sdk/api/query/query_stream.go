package query

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/dynatrace-oss/dtctl/sdk/httpclient"
)

// recordSink routes decoded records to the right callback depending on where
// the JSON carried them: the older, synchronous shape puts them at the top
// level ("records"), the current shape nests them under "result"
// ("result.records"). A response only ever populates one of the two in
// practice, but both are wired independently so a caller cannot silently miss
// rows if that ever changes. Either field may be nil.
type recordSink struct {
	onTop    func(map[string]interface{}) error
	onResult func(map[string]interface{}) error
}

// ExecuteStream is Execute, but decodes the response body incrementally
// instead of buffering it whole, and does not accumulate the result records
// itself: onRecord is invoked once per row, in order, as it is decoded off
// the wire, and the row is then dropped. This is what makes it safe to run
// against an arbitrarily large result under a tight memory budget — Execute's
// buffered path costs 10-14x the printed output size in peak RSS, because it
// keeps the raw response body, resty's decoded copy, and the accumulated
// Records slice all live at once.
//
// The returned Response never carries decoded records: Records and
// Result.Records are always nil, whether or not the query actually returned
// rows, since every row was delivered through onRecord instead. Every other
// field (state, requestToken, metadata, types, progress) is populated exactly
// as Execute would populate it.
//
// onRecord may be nil, in which case records are decoded and discarded one at
// a time — useful for validating a query completes without holding its
// result. A non-nil error returned from onRecord aborts the decode and is
// returned from ExecuteStream, wrapped.
func (h *Handler) ExecuteStream(ctx context.Context, req ExecuteRequest, onRecord func(map[string]interface{}) error) (*Response, error) {
	resp, err := h.executeRaw(ctx, req, recordSink{onTop: onRecord, onResult: onRecord})
	if err != nil {
		return nil, err
	}
	resp.Records = nil
	if resp.Result != nil {
		resp.Result.Records = nil
	}
	return resp, nil
}

// PollStream is Poll, but decodes the response body incrementally instead of
// buffering it whole — see ExecuteStream for the memory rationale and the
// onRecord/nil-Records contract, both of which apply identically here.
func (h *Handler) PollStream(ctx context.Context, requestToken string, timeoutMs int64, enrich bool, onRecord func(map[string]interface{}) error) (*Response, error) {
	resp, err := h.pollRaw(ctx, requestToken, timeoutMs, enrich, recordSink{onTop: onRecord, onResult: onRecord})
	if err != nil {
		return nil, err
	}
	resp.Records = nil
	if resp.Result != nil {
		resp.Result.Records = nil
	}
	return resp, nil
}

// executeRaw performs the query:execute HTTP call and streams the response
// through decodeResponseStream, invoking sink's callbacks per record. It
// never accumulates rows on its own behalf: the returned Response.Records and
// Result.Records are non-nil only as an O(1) presence marker (an empty slice)
// when the corresponding JSON key held a non-null array — never populated
// with the actual rows, regardless of how many were streamed through sink.
// Callers that want the rows accumulate them via sink and reattach the result
// themselves (see Execute); callers that don't care clear the marker (see
// ExecuteStream).
func (h *Handler) executeRaw(ctx context.Context, req ExecuteRequest, sink recordSink) (*Response, error) {
	httpReq := h.client.HTTP().R().SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept-Encoding", "identity"). // RawBody() below must see the plain body, not gzip.
		SetBody(req).
		SetDoNotParseResponse(true)
	if req.EnrichMetricMetadata {
		httpReq.SetQueryParam("enrich", enrichMetricMetadataParam)
	}
	h.applyHeaders(httpReq)

	resp, err := httpReq.Post(basePath + ":execute")
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer func() { _ = resp.RawBody().Close() }()

	// 200 with completed state or 202 with RUNNING are both valid.
	if resp.StatusCode() == 200 || resp.StatusCode() == 202 {
		result, err := decodeResponseStream(resp.RawBody(), sink)
		if err != nil {
			return nil, fmt.Errorf("failed to execute query: %w", err)
		}
		return result, nil
	}

	if resp.IsError() {
		body, _ := io.ReadAll(resp.RawBody())
		return nil, parseError(resp.StatusCode(), body)
	}
	return nil, fmt.Errorf("unexpected status code %d", resp.StatusCode())
}

// pollRaw performs the query:poll HTTP call and streams the response through
// decodeResponseStream — see executeRaw for the accumulation contract, which
// is identical here.
func (h *Handler) pollRaw(ctx context.Context, requestToken string, timeoutMs int64, enrich bool, sink recordSink) (*Response, error) {
	httpReq := h.client.HTTP().R().SetContext(ctx).
		SetQueryParam("request-token", requestToken).
		SetQueryParam("request-timeout-milliseconds", fmt.Sprintf("%d", timeoutMs)).
		SetHeader("Accept-Encoding", "identity"). // RawBody() below must see the plain body, not gzip.
		SetDoNotParseResponse(true)
	if enrich {
		httpReq.SetQueryParam("enrich", enrichMetricMetadataParam)
	}
	h.applyHeaders(httpReq)

	resp, err := httpReq.Get(basePath + ":poll")
	if err != nil {
		return nil, fmt.Errorf("failed to poll query: %w", err)
	}
	defer func() { _ = resp.RawBody().Close() }()

	if resp.IsError() {
		body, _ := io.ReadAll(resp.RawBody())
		return nil, httpclient.NewAPIError(resp.StatusCode(), resp.Status(), string(body))
	}

	result, err := decodeResponseStream(resp.RawBody(), sink)
	if err != nil {
		return nil, fmt.Errorf("failed to poll query: %w", err)
	}
	return result, nil
}

// decodeResponseStream decodes a query Execute/Poll JSON response from r,
// invoking sink.onTop/sink.onResult for each record as it is decoded instead
// of buffering the whole records array in memory. Every other field is
// decoded normally into the returned Response.
//
// Response.Records and Result.Records are set to a non-nil empty slice when
// the corresponding JSON key held a present, non-null array — an O(1)
// presence marker, matching encoding/json's own nil-vs-empty-slice fidelity
// for that field, without accumulating the rows a second time. They are never
// populated with the decoded rows themselves; a caller that needs them
// collects rows via sink and reattaches them (see Execute/Poll in query.go).
//
// The set of keys handled here must be kept in sync with the Response and
// Result struct definitions in query.go — see TestDecodeResponseStream_AllFields
// for the regression guard. A key this function does not recognize is decoded
// into a throwaway json.RawMessage and dropped, matching encoding/json's
// default behavior of silently ignoring unknown fields.
func decodeResponseStream(r io.Reader, sink recordSink) (*Response, error) {
	dec := json.NewDecoder(r)

	if err := expectDelim(dec, '{'); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	var resp Response
	for dec.More() {
		key, err := decodeObjectKey(dec)
		if err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}

		switch key {
		case "state":
			if err := dec.Decode(&resp.State); err != nil {
				return nil, fmt.Errorf("decode response.state: %w", err)
			}
		case "requestToken":
			if err := dec.Decode(&resp.RequestToken); err != nil {
				return nil, fmt.Errorf("decode response.requestToken: %w", err)
			}
		case "progress":
			if err := dec.Decode(&resp.Progress); err != nil {
				return nil, fmt.Errorf("decode response.progress: %w", err)
			}
		case "metadata":
			if err := dec.Decode(&resp.Metadata); err != nil {
				return nil, fmt.Errorf("decode response.metadata: %w", err)
			}
		case "records":
			present, err := streamRecordsArray(dec, sink.onTop)
			if err != nil {
				return nil, fmt.Errorf("decode response.records: %w", err)
			}
			if present {
				resp.Records = []map[string]interface{}{}
			}
		case "result":
			result, err := decodeResultStream(dec, sink.onResult)
			if err != nil {
				return nil, fmt.Errorf("decode response.result: %w", err)
			}
			resp.Result = result
		default:
			if err := discardValue(dec); err != nil {
				return nil, fmt.Errorf("decode response.%s: %w", key, err)
			}
		}
	}

	if err := expectDelim(dec, '}'); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &resp, nil
}

// decodeResultStream decodes a "result" object, streaming its nested
// "records" array through onRecord. dec must be positioned right after the
// "result" key token. Returns (nil, nil) for a JSON null result. See
// decodeResponseStream for the Records presence-marker contract, which
// applies identically to the returned Result.Records.
func decodeResultStream(dec *json.Decoder, onRecord func(map[string]interface{}) error) (*Result, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if tok == nil {
		return nil, nil
	}
	d, ok := tok.(json.Delim)
	if !ok || d != '{' {
		return nil, fmt.Errorf("expected object, got %v", tok)
	}

	var result Result
	for dec.More() {
		key, err := decodeObjectKey(dec)
		if err != nil {
			return nil, err
		}

		switch key {
		case "records":
			present, err := streamRecordsArray(dec, onRecord)
			if err != nil {
				return nil, err
			}
			if present {
				result.Records = []map[string]interface{}{}
			}
		case "types":
			if err := dec.Decode(&result.Types); err != nil {
				return nil, err
			}
		case "metadata":
			if err := dec.Decode(&result.Metadata); err != nil {
				return nil, err
			}
		default:
			if err := discardValue(dec); err != nil {
				return nil, err
			}
		}
	}

	if _, err := dec.Token(); err != nil { // closing '}'
		return nil, err
	}
	return &result, nil
}

// streamRecordsArray decodes a JSON array of row objects positioned right
// after the array's key token, invoking onRecord for each row as it is
// decoded. It does not accumulate rows itself, so memory stays bounded to one
// row at a time whenever onRecord does not retain it.
//
// Returns present=false for a JSON null array (key existed but was null), and
// present=true (with zero calls to onRecord) for "[]".
func streamRecordsArray(dec *json.Decoder, onRecord func(map[string]interface{}) error) (present bool, err error) {
	tok, err := dec.Token()
	if err != nil {
		return false, err
	}
	if tok == nil {
		return false, nil
	}
	d, ok := tok.(json.Delim)
	if !ok || d != '[' {
		return false, fmt.Errorf("expected array, got %v", tok)
	}

	for dec.More() {
		var row map[string]interface{}
		if err := dec.Decode(&row); err != nil {
			return false, err
		}
		if onRecord != nil {
			if err := onRecord(row); err != nil {
				return false, err
			}
		}
	}

	if _, err := dec.Token(); err != nil { // closing ']'
		return false, err
	}
	return true, nil
}

// expectDelim reads the next token from dec and requires it to be the given
// JSON delimiter (e.g. '{' or '}').
func expectDelim(dec *json.Decoder, want json.Delim) error {
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	d, ok := tok.(json.Delim)
	if !ok || d != want {
		return fmt.Errorf("expected %q, got %v", want, tok)
	}
	return nil
}

// decodeObjectKey reads the next token from dec and requires it to be a
// string (an object key). Used while walking an object with dec.More().
func decodeObjectKey(dec *json.Decoder) (string, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", err
	}
	key, ok := tok.(string)
	if !ok {
		return "", fmt.Errorf("expected object key, got %v", tok)
	}
	return key, nil
}

// discardValue consumes and drops the next JSON value (of any shape) from
// dec, for a key this decoder does not otherwise handle.
func discardValue(dec *json.Decoder) error {
	var discard json.RawMessage
	return dec.Decode(&discard)
}
