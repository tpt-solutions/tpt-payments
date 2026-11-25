// Command tpt is the command-line client for TPT Pay. It needs only the standard library.
//
//	tpt login --api-key sk_test_... [--base-url URL]   save credentials (0600)
//	tpt keys create [--type secret|publishable] [--mode test|live] [--name NAME]
//	tpt trigger EVENT --forward-to URL --secret whsec_...   send a signed test event
//	tpt events                                          list the event types the platform sends
//
// Environment: TPT_API_KEY and TPT_BASE_URL override the saved profile.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/webhook"
)

const defaultBaseURL = "https://api.tpt-payments.com/v1"

// profile is the saved configuration.
type profile struct {
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stderr)
		return errors.New("a command is required")
	}
	ctx := context.Background()
	switch args[0] {
	case "login":
		return cmdLogin(args[1:], stdout)
	case "keys":
		if len(args) < 2 || args[1] != "create" {
			return errors.New("usage: tpt keys create [--type secret|publishable] [--mode test|live] [--name NAME]")
		}
		return cmdKeysCreate(ctx, args[2:], stdout)
	case "trigger":
		return cmdTrigger(ctx, args[1:], stdout)
	case "logs":
		return cmdLogs(ctx, args[1:], stdout)
	case "replay":
		return cmdReplay(ctx, args[1:], stdout)
	case "listen":
		return cmdListen(ctx, args[1:], stdout)
	case "events":
		for _, e := range eventTypes {
			fmt.Fprintln(stdout, e)
		}
		return nil
	case "help", "-h", "--help":
		usage(stdout)
		return nil
	default:
		usage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `tpt: command-line client for TPT Pay

  tpt login --api-key KEY [--base-url URL]
  tpt keys create [--type secret|publishable] [--mode test|live] [--name NAME]
  tpt trigger EVENT --forward-to URL --secret whsec_...
  tpt listen --forward-to URL --secret whsec_... [--endpoint ID] [--interval 2s]
  tpt logs [--limit N] [--endpoint ID]
  tpt replay DELIVERY_ID
  tpt events`)
}

// configPath is the profile file: <user config dir>/tpt/config.json.
func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find config directory: %w", err)
	}
	return filepath.Join(dir, "tpt", "config.json"), nil
}

func loadProfile() (profile, error) {
	var p profile
	path, err := configPath()
	if err != nil {
		return p, err
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &p); err != nil {
			return p, fmt.Errorf("read %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return p, err
	}
	if v := os.Getenv("TPT_API_KEY"); v != "" {
		p.APIKey = v
	}
	if v := os.Getenv("TPT_BASE_URL"); v != "" {
		p.BaseURL = v
	}
	if p.BaseURL == "" {
		p.BaseURL = defaultBaseURL
	}
	return p, nil
}

func cmdLogin(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	apiKey := fs.String("api-key", "", "secret (sk_...) or publishable (pk_...) key")
	baseURL := fs.String("base-url", "", "API base URL (default "+defaultBaseURL+")")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !strings.HasPrefix(*apiKey, "sk_") && !strings.HasPrefix(*apiKey, "pk_") {
		return errors.New("--api-key must start with sk_ or pk_")
	}
	if *baseURL != "" {
		if err := checkURL(*baseURL); err != nil {
			return err
		}
	}
	p := profile{APIKey: *apiKey, BaseURL: *baseURL}
	if p.BaseURL == "" {
		p.BaseURL = defaultBaseURL
	}
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(p, "", "  ")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("save profile: %w", err)
	}
	fmt.Fprintf(stdout, "saved credentials to %s\n", path)
	return nil
}

// checkURL requires http or https. Plain http is allowed only for loopback, so a key is never
// sent to a remote host in clear.
func checkURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%q is not a URL", raw)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1" {
			return nil
		}
		return errors.New("plain http is allowed only for localhost")
	default:
		return fmt.Errorf("unsupported URL scheme %q", u.Scheme)
	}
}

func cmdKeysCreate(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("keys create", flag.ContinueOnError)
	typ := fs.String("type", "secret", "secret or publishable")
	mode := fs.String("mode", "", "test or live (default: the mode of the saved key)")
	name := fs.String("name", "", "a label for the key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := loadProfile()
	if err != nil {
		return err
	}
	if p.APIKey == "" {
		return errors.New("not logged in: run tpt login --api-key KEY")
	}
	body := map[string]string{"type": *typ, "mode": *mode, "name": *name}
	resp, err := do(ctx, p, http.MethodPost, "/api_keys", body, nil)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, string(resp))
	return nil
}

// do sends one authenticated request and returns the body. Non-2xx responses become errors that
// include the API's message, not the request.
func do(ctx context.Context, p profile, method, path string, body any, headers map[string]string) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(p.BaseURL, "/")+path, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "tpt-cli/1")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(raw, &e)
		msg := e.Error.Message
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, msg)
	}
	return raw, nil
}

// eventTypes mirrors webhook.KnownEventTypes. A test pins the two together.
var eventTypes = []string{
	"payment_intent.created",
	"payment_intent.succeeded",
	"payment_intent.payment_failed",
	"payment_intent.canceled",
	"charge.refunded",
	"subscription.created",
	"subscription.past_due",
	"subscription.canceled",
}

// eventObject returns a representative data object for a test event.
func eventObject(eventType string) map[string]any {
	switch eventType {
	case "charge.refunded":
		return map[string]any{
			"id": "rf_test_" + fmt.Sprint(time.Now().Unix()), "object": "refund",
			"amount": 1000, "currency": "usd", "payment_intent": "pi_test_fixture", "status": "succeeded",
		}
	default:
		status := map[string]string{
			"payment_intent.created":        "requires_payment_method",
			"payment_intent.succeeded":      "succeeded",
			"payment_intent.payment_failed": "requires_payment_method",
			"payment_intent.canceled":       "canceled",
		}[eventType]
		return map[string]any{
			"id": "pi_test_fixture", "object": "payment_intent",
			"amount": 2000, "currency": "usd", "status": status,
		}
	}
}

func cmdTrigger(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: tpt trigger EVENT --forward-to URL --secret whsec_...")
	}
	eventType := args[0]
	fs := flag.NewFlagSet("trigger", flag.ContinueOnError)
	forward := fs.String("forward-to", "", "the endpoint to send the event to")
	secret := fs.String("secret", "", "the endpoint's signing secret (whsec_...)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if !contains(eventTypes, eventType) {
		return fmt.Errorf("unknown event %q; run tpt events", eventType)
	}
	if *forward == "" || *secret == "" {
		return errors.New("--forward-to and --secret are required")
	}
	if err := checkURL(*forward); err != nil {
		return err
	}
	if !strings.HasPrefix(*secret, "whsec_") {
		return errors.New("--secret must start with whsec_")
	}

	event := map[string]any{
		"id":       fmt.Sprintf("evt_test_%d", time.Now().UnixNano()),
		"object":   "event",
		"type":     eventType,
		"created":  time.Now().Unix(),
		"livemode": false,
		"data":     map[string]any{"object": eventObject(eventType)},
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	header := webhook.Sign(payload, *secret, time.Now())

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, *forward, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "tpt-cli/1")
	req.Header.Set("TPT-Event-Id", event["id"].(string))
	req.Header.Set(webhook.SignatureHeader, header)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("delivery failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	fmt.Fprintf(stdout, "sent %s (%s) to %s: HTTP %d\n", eventType, event["id"], *forward, resp.StatusCode)
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("the endpoint answered HTTP %d", resp.StatusCode)
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// cmdLogs prints one page of recent webhook deliveries.
func cmdLogs(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	limit := fs.Int("limit", 10, "how many deliveries to show (1-100)")
	endpoint := fs.String("endpoint", "", "only deliveries for this endpoint ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	p, err := loadProfile()
	if err != nil {
		return err
	}
	if p.APIKey == "" {
		return errors.New("not logged in: run tpt login --api-key KEY")
	}
	path := fmt.Sprintf("/webhook_deliveries?limit=%d", *limit)
	if *endpoint != "" {
		path += "&endpoint_id=" + url.QueryEscape(*endpoint)
	}
	raw, err := do(ctx, p, http.MethodGet, path, nil, nil)
	if err != nil {
		return err
	}
	var page struct {
		Data []struct {
			ID         string `json:"id"`
			EventType  string `json:"event_type"`
			Status     string `json:"status"`
			Attempts   int    `json:"attempts"`
			LastStatus *int   `json:"last_http_status"`
			LastError  string `json:"last_error"`
		} `json:"data"`
		HasMore bool `json:"has_more"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return fmt.Errorf("read deliveries: %w", err)
	}
	for _, d := range page.Data {
		code := "-"
		if d.LastStatus != nil {
			code = fmt.Sprint(*d.LastStatus)
		}
		fmt.Fprintf(stdout, "%s  %-30s  %-9s  attempts=%d  http=%s  %s\n",
			d.ID, d.EventType, d.Status, d.Attempts, code, d.LastError)
	}
	if page.HasMore {
		fmt.Fprintln(stdout, "(more deliveries exist; raise --limit to see them)")
	}
	return nil
}

// cmdReplay queues a delivery to be sent again.
func cmdReplay(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) != 1 {
		return errors.New("usage: tpt replay DELIVERY_ID")
	}
	p, err := loadProfile()
	if err != nil {
		return err
	}
	if p.APIKey == "" {
		return errors.New("not logged in: run tpt login --api-key KEY")
	}
	if _, err := do(ctx, p, http.MethodPost, "/webhook_deliveries/"+url.PathEscape(args[0])+"/replay", map[string]string{}, nil); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "queued delivery %s for another attempt\n", args[0])
	return nil
}
