package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/store"
)

// ── KV: durable per-guild / per-member values ────────────────────────────────

func hKVGet(ctx context.Context, h *Halt) error {
	var spec cc.SpecKV
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	if spec.Into == "" {
		return errors.New("kv_get: into required")
	}
	entry, err := kvEntry(ctx, h, spec)
	if err != nil {
		return err
	}
	got, err := h.Deps.Store.KVGet(ctx, entry)
	if err != nil {
		if len(spec.Default) > 0 {
			var v any
			_ = json.Unmarshal(spec.Default, &v)
			h.Scope.Set(spec.Into, v)
			h.SetOutput(v)
			return nil
		}
		h.Scope.Set(spec.Into, nil)
		return nil
	}
	var v any
	_ = json.Unmarshal(got.Value, &v)
	h.Scope.Set(spec.Into, v)
	h.SetOutput(v)
	return nil
}

func hKVSet(ctx context.Context, h *Halt) error {
	var spec cc.SpecKV
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	entry, err := kvEntry(ctx, h, spec)
	if err != nil {
		return err
	}
	raw, err := cc.EvalJSON(ctx, spec.Value, h.Scope)
	if err != nil {
		return err
	}
	entry.Value = raw
	if spec.TTL != "" {
		d, err := time.ParseDuration(spec.TTL)
		if err == nil && d > 0 {
			t := time.Now().Add(d)
			entry.ExpiresAt = &t
		}
	}
	return h.Deps.Store.KVSet(ctx, entry)
}

func hKVDelete(ctx context.Context, h *Halt) error {
	var spec cc.SpecKV
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	entry, err := kvEntry(ctx, h, spec)
	if err != nil {
		return err
	}
	return h.Deps.Store.KVDelete(ctx, entry)
}

func kvEntry(ctx context.Context, h *Halt, spec cc.SpecKV) (store.FeatureKVEntry, error) {
	key, err := cc.EvalTemplated(ctx, spec.Key, h.Scope)
	if err != nil {
		return store.FeatureKVEntry{}, err
	}
	if key == "" {
		return store.FeatureKVEntry{}, errors.New("kv: key required")
	}
	scope := spec.Scope
	if scope == "" {
		scope = "guild"
	}
	if scope != "guild" && scope != "member" {
		return store.FeatureKVEntry{}, errors.New("kv: scope must be guild or member")
	}
	gid, _ := event.ParseID(h.Run.GuildID)
	var owner int64
	if scope == "member" {
		if id, _ := cc.EvalSnowflake(ctx, spec.OwnerID, h.Scope); id != "" {
			owner, _ = event.ParseID(id)
		} else if h.Run.InvokerID != "" {
			owner, _ = event.ParseID(h.Run.InvokerID)
		}
	}
	return store.FeatureKVEntry{
		GuildID:   gid,
		CommandID: h.Run.CommandID,
		Scope:     scope,
		OwnerID:   owner,
		Key:       key,
	}, nil
}

// ── HTTP: SSRF-guarded outbound (response body capped) ───────────────────────

func hHTTPRequest(ctx context.Context, h *Halt) error {
	var spec cc.SpecHTTP
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	if h.Run.httpCalls.Add(1) > maxHTTPCallsPerRun {
		return errors.New("http_request: per-run HTTP budget exceeded")
	}
	method := strings.ToUpper(spec.Method)
	if method == "" {
		method = "GET"
	}
	url, err := cc.EvalTemplated(ctx, spec.URL, h.Scope)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return errors.New("http_request: URL must be http:// or https://")
	}
	var body io.Reader
	if len(spec.Body.Value) > 0 || spec.Body.Src != "" {
		raw, err := cc.EvalJSON(ctx, spec.Body, h.Scope)
		if err == nil && len(raw) > 0 {
			body = bytes.NewReader(raw)
		}
	}
	timeout := time.Duration(spec.TimeoutMs) * time.Millisecond
	if timeout <= 0 || timeout > 30*time.Second {
		timeout = 5 * time.Second
	}
	rctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, method, url, body)
	if err != nil {
		return err
	}
	for k, v := range spec.Headers {
		if rendered, err := cc.EvalTemplated(ctx, v, h.Scope); err == nil {
			req.Header.Set(k, rendered)
		} else {
			req.Header.Set(k, v)
		}
	}
	resp, err := h.Deps.HTTP.Do(rctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	const maxBody = 256 << 10 // 256 KiB
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if len(raw) > maxBody {
		return errors.New("http_request: response body exceeded 256 KiB")
	}
	result := map[string]any{
		"status":  resp.StatusCode,
		"headers": flatHeaders(resp.Header),
	}
	if spec.ParseJSON {
		var parsed any
		if err := json.Unmarshal(raw, &parsed); err == nil {
			result["json"] = parsed
		}
		result["body"] = string(raw)
	} else {
		result["body"] = string(raw)
	}
	if spec.Into != "" {
		h.Scope.Set(spec.Into, result)
	}
	h.SetOutput(result)
	return nil
}

func flatHeaders(h http.Header) map[string]string {
	out := map[string]string{}
	for k, v := range h {
		if len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out
}
