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
	"os/signal"
	"strings"
	"time"

	"github.com/tpt-solutions/tpt-payments/packages/tpt-pay-core/internal/webhook"
)

// listenSeenLimit bounds the memory used to remember forwarded events. When it is reached the
// memory is cleared, so an event forwarded long ago could in principle be forwarded again.
const listenSeenLimit = 10_000

// cmdListen forwards the webhook events your account receives to a local URL, signed with the
// local secret. It needs no tunnel: it polls the delivery log (with payloads) over the API.
//
// It works through registered endpoints, so create one first (its URL can be anything; the
// server's own attempts to it will fail, which is expected). Events that already exist when the
// listener starts are not forwarded.
func cmdListen(ctx context.Context, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("listen", flag.ContinueOnError)
	forward := fs.String("forward-to", "", "the local URL to send events to")
	secret := fs.String("secret", "", "the signing secret to sign forwarded events with (whsec_...)")
	endpoint := fs.String("endpoint", "", "only events delivered to this endpoint ID")
	interval := fs.Duration("interval", 2*time.Second, "how often to ask the API for new events")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *forward == "" || *secret == "" {
		return errors.New("usage: tpt listen --forward-to URL --secret whsec_... [--endpoint ID] [--interval 2s]")
	}
	if err := checkURL(*forward); err != nil {
		return err
	}
	if !strings.HasPrefix(*secret, "whsec_") {
		return errors.New("--secret must start with whsec_")
	}
	if *interval < time.Second {
		return errors.New("--interval must be at least 1s")
	}
	p, err := loadProfile()
	if err != nil {
		return err
	}
	if p.APIKey == "" {
		return errors.New("not logged in: run tpt login --api-key KEY")
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	l := &listener{
		profile:   p,
		forwardTo: *forward,
		secret:    *secret,
		endpoint:  *endpoint,
		interval:  *interval,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
	fmt.Fprintf(stdout, "listening for events; forwarding to %s (Ctrl-C to stop)\n", *forward)
	return l.run(ctx, stdout)
}

type listener struct {
	profile   profile
	forwardTo string
	secret    string
	endpoint  string
	interval  time.Duration
	client    *http.Client
}

// listenEvent is one event from the delivery log, with the exact bytes that were sent.
type listenEvent struct {
	EventID   string          `json:"event_id"`
	EventType string          `json:"event_type"`
	Payload   json.RawMessage `json:"payload"`
}

// run polls until ctx is cancelled. The first poll only records what already exists.
func (l *listener) run(ctx context.Context, stdout io.Writer) error {
	seen := make(map[string]bool)
	primed := false
	for {
		events, err := l.fetch(ctx)
		switch {
		case ctx.Err() != nil:
			return nil
		case err != nil:
			fmt.Fprintf(stdout, "could not read events (retrying): %v\n", err)
		default:
			if len(seen) > listenSeenLimit {
				seen = make(map[string]bool)
			}
			for _, ev := range events { // oldest first
				if seen[ev.EventID] {
					continue
				}
				seen[ev.EventID] = true
				if primed {
					l.forward(ctx, ev, stdout)
				}
			}
			primed = true
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(l.interval):
		}
	}
}

// fetch returns the newest deliveries with payloads, oldest first. One event can appear more than
// once (one delivery per endpoint); the caller de-duplicates by event ID.
func (l *listener) fetch(ctx context.Context) ([]listenEvent, error) {
	path := "/webhook_deliveries?limit=100&include=payload"
	if l.endpoint != "" {
		path += "&endpoint_id=" + url.QueryEscape(l.endpoint)
	}
	raw, err := do(ctx, l.profile, http.MethodGet, path, nil, nil)
	if err != nil {
		return nil, err
	}
	var page struct {
		Data []listenEvent `json:"data"`
	}
	if err := json.Unmarshal(raw, &page); err != nil {
		return nil, fmt.Errorf("read deliveries: %w", err)
	}
	out := make([]listenEvent, 0, len(page.Data))
	for i := len(page.Data) - 1; i >= 0; i-- {
		out = append(out, page.Data[i])
	}
	return out, nil
}

// forward sends one event to the local URL, signed the way the API signs deliveries.
func (l *listener) forward(ctx context.Context, ev listenEvent, stdout io.Writer) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, l.forwardTo, bytes.NewReader(ev.Payload))
	if err != nil {
		fmt.Fprintf(stdout, "cannot forward %s: %v\n", ev.EventType, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "tpt-cli/1")
	req.Header.Set("TPT-Event-Id", ev.EventID)
	req.Header.Set(webhook.SignatureHeader, webhook.Sign(ev.Payload, l.secret, time.Now()))

	resp, err := l.client.Do(req)
	if err != nil {
		fmt.Fprintf(stdout, "forward %s (%s) failed: %v\n", ev.EventType, ev.EventID, err)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	fmt.Fprintf(stdout, "forwarded %s (%s): HTTP %d\n", ev.EventType, ev.EventID, resp.StatusCode)
}
