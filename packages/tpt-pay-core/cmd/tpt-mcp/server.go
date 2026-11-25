package main

// A Model Context Protocol server over stdio (newline-delimited JSON-RPC 2.0). It gives an
// assistant read access to the sandbox's test tokens, webhook event types and the published
// docs index, and lets it run test payments with a test secret key. It is written against the
// standard library, like cmd/tpt, so it adds no dependency.
//
// Live keys are refused, both by prefix and by the tool itself: an assistant should never
// move real money.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/adapter/sandbox"
	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/webhook"
)

const protocolVersion = "2025-06-18"

// Config is what the server needs at runtime.
type Config struct {
	APIBase  string // e.g. https://api.tpt-payments.com/v1
	APIKey   string // a test secret key (sk_test_...); empty disables test payments
	DocsPath string // optional path to the llms.txt index; empty disables read_docs
	Client   *http.Client
}

type server struct {
	cfg Config
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

const (
	errParse          = -32700
	errInvalidRequest = -32600
	errMethodNotFound = -32601
	errInvalidParams  = -32602
)

// Serve reads requests from in and writes responses to out until in closes or ctx is done.
func Serve(ctx context.Context, cfg Config, in io.Reader, out io.Writer) error {
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 30 * time.Second}
	}
	s := &server{cfg: cfg}
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	enc := json.NewEncoder(out)
	for sc.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		resp := s.handleLine(ctx, line)
		if resp == nil {
			continue // notifications get no response
		}
		if err := enc.Encode(resp); err != nil {
			return fmt.Errorf("write response: %w", err)
		}
	}
	return sc.Err()
}

func (s *server) handleLine(ctx context.Context, line []byte) *rpcResponse {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return &rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{errParse, "invalid JSON"}}
	}
	if req.JSONRPC != "2.0" || req.Method == "" {
		return &rpcResponse{JSONRPC: "2.0", ID: nullIfEmpty(req.ID), Error: &rpcError{errInvalidRequest, "invalid request"}}
	}
	isNotification := len(req.ID) == 0
	if isNotification {
		return nil
	}

	result, rerr := s.dispatch(ctx, req)
	resp := &rpcResponse{JSONRPC: "2.0", ID: req.ID}
	if rerr != nil {
		resp.Error = rerr
	} else {
		resp.Result = result
	}
	return resp
}

func nullIfEmpty(id json.RawMessage) json.RawMessage {
	if len(id) == 0 {
		return json.RawMessage("null")
	}
	return id
}

func (s *server) dispatch(ctx context.Context, req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "tpt-mcp", "version": "0.1.0"},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": s.tools()}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil || p.Name == "" {
			return nil, &rpcError{errInvalidParams, "tools/call needs a tool name"}
		}
		text, err := s.callTool(ctx, p.Name, p.Arguments)
		if err != nil {
			// Tool failures are reported inside the result, so the model can read and react to them.
			return map[string]any{
				"content": []map[string]any{{"type": "text", "text": err.Error()}},
				"isError": true,
			}, nil
		}
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"isError": false,
		}, nil
	default:
		return nil, &rpcError{errMethodNotFound, "method not found: " + req.Method}
	}
}

type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func (s *server) tools() []toolDef {
	noArgs := map[string]any{"type": "object", "properties": map[string]any{}}
	tools := []toolDef{
		{
			Name:        "list_test_payment_methods",
			Description: "List the sandbox test payment method tokens and what each one does. Read-only.",
			InputSchema: noArgs,
		},
		{
			Name:        "list_webhook_event_types",
			Description: "List every webhook event type the platform emits. Read-only.",
			InputSchema: noArgs,
		},
		{
			Name: "create_test_payment",
			Description: "Create and confirm a payment intent in the sandbox with a test key. " +
				"Refuses live keys. Use a pm_card_* or pm_test_* token to choose the outcome.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"amount":         map[string]any{"type": "integer", "minimum": 1, "description": "amount in the smallest currency unit, e.g. cents"},
					"currency":       map[string]any{"type": "string", "description": "ISO 4217 code, default USD"},
					"payment_method": map[string]any{"type": "string", "description": "test token, default pm_card_visa"},
				},
				"required": []string{"amount"},
			},
		},
	}
	if s.cfg.DocsPath != "" {
		tools = append(tools, toolDef{
			Name:        "read_docs",
			Description: "Return the published docs index (llms.txt): endpoints, webhook events and sandbox tokens.",
			InputSchema: noArgs,
		})
	}
	return tools
}

func (s *server) callTool(ctx context.Context, name string, args json.RawMessage) (string, error) {
	switch name {
	case "list_test_payment_methods":
		return testPaymentMethods(), nil
	case "list_webhook_event_types":
		return webhookEventTypes(), nil
	case "read_docs":
		if s.cfg.DocsPath == "" {
			return "", fmt.Errorf("docs are not configured on this server")
		}
		raw, err := os.ReadFile(s.cfg.DocsPath)
		if err != nil {
			return "", fmt.Errorf("read docs: %w", err)
		}
		return string(raw), nil
	case "create_test_payment":
		return s.createTestPayment(ctx, args)
	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

// testPaymentMethods lists the scripted tokens from the sandbox itself, so the list cannot drift
// from what the sandbox does. pm_card_visa is the approving default and is not a scenario.
func testPaymentMethods() string {
	names := make([]string, 0, len(sandbox.Scenarios))
	for n := range sandbox.Scenarios {
		if strings.HasPrefix(n, "pm_card_") {
			names = append(names, n)
		}
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString("pm_card_visa: approves (default)\n")
	for _, n := range names {
		sc := sandbox.Scenarios[n]
		var outcome string
		switch {
		case sc.NetworkError:
			outcome = "unknown outcome (network error)"
		case sc.Delay > 0:
			outcome = fmt.Sprintf("approves after %s", sc.Delay)
		default:
			outcome = fmt.Sprintf("declines with %s", sc.DeclineCode)
		}
		fmt.Fprintf(&b, "%s (also %s): %s\n", n, strings.Replace(n, "pm_card_", "pm_test_", 1), outcome)
	}
	return b.String()
}

func webhookEventTypes() string {
	var b strings.Builder
	for _, e := range webhook.KnownEventTypes() {
		fmt.Fprintln(&b, string(e))
	}
	return b.String()
}

func (s *server) createTestPayment(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Amount        int64  `json:"amount"`
		Currency      string `json:"currency"`
		PaymentMethod string `json:"payment_method"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &in); err != nil {
			return "", fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if in.Amount <= 0 {
		return "", fmt.Errorf("amount must be a positive integer in the smallest currency unit")
	}
	if in.Currency == "" {
		in.Currency = "USD"
	}
	if in.PaymentMethod == "" {
		in.PaymentMethod = "pm_card_visa"
	}
	if !strings.HasPrefix(in.PaymentMethod, "pm_card_") && !strings.HasPrefix(in.PaymentMethod, "pm_test_") {
		return "", fmt.Errorf("payment_method must be a pm_card_* or pm_test_* sandbox token")
	}
	if s.cfg.APIKey == "" {
		return "", fmt.Errorf("no API key configured: set TPT_API_KEY to a test secret key")
	}
	if !strings.HasPrefix(s.cfg.APIKey, "sk_test_") {
		return "", fmt.Errorf("refusing to run: TPT_API_KEY is not a test secret key (sk_test_...)")
	}

	created, err := s.post(ctx, "/payment_intents", map[string]any{
		"amount":   in.Amount,
		"currency": strings.ToUpper(in.Currency),
	})
	if err != nil {
		return "", err
	}
	var intent struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(created, &intent); err != nil || intent.ID == "" {
		return "", fmt.Errorf("unexpected response creating payment intent")
	}

	confirmed, err := s.post(ctx, "/payment_intents/"+url.PathEscape(intent.ID)+"/confirm", map[string]any{
		"payment_method": in.PaymentMethod,
	})
	if err != nil {
		return "", fmt.Errorf("created %s but confirming failed: %w", intent.ID, err)
	}
	return string(confirmed), nil
}

func (s *server) post(ctx context.Context, path string, body any) ([]byte, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSuffix(s.cfg.APIBase, "/")+path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", fmt.Sprintf("tpt-mcp-%d", time.Now().UnixNano()))
	resp, err := s.cfg.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(out)))
	}
	return out, nil
}
