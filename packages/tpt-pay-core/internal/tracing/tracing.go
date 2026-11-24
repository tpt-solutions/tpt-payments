// Package tracing sets up OpenTelemetry tracing for the server.
//
// Tracing is off unless an OTLP endpoint is configured through the standard environment
// variables (OTEL_EXPORTER_OTLP_ENDPOINT or OTEL_EXPORTER_OTLP_TRACES_ENDPOINT). Without one,
// spans are still created but nothing is exported, so the instrumentation costs little and
// the code paths are the same in every environment.
package tracing

import (
	"context"
	"os"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Provider is a tracer provider with its shutdown. Call Shutdown on exit so queued spans
// are flushed.
type Provider struct {
	*sdktrace.TracerProvider
	exporting bool
}

// Exporting reports whether spans leave the process.
func (p *Provider) Exporting() bool { return p.exporting }

// New builds a tracer provider for serviceName. It exports over OTLP/HTTP when an endpoint is
// set in the environment, and keeps spans in-process otherwise.
func New(ctx context.Context, serviceName, version string) (*Provider, error) {
	// Schemaless: resource.Default() carries its own semantic-conventions schema, and merging in
	// a differently versioned one fails. service.name and service.version are the standard keys.
	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(
		attribute.String("service.name", serviceName),
		attribute.String("service.version", version),
	))
	if err != nil {
		return nil, err
	}

	opts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	exporting := endpointConfigured()
	if exporting {
		exp, err := otlptracehttp.New(ctx) // reads the OTEL_EXPORTER_OTLP_* variables
		if err != nil {
			return nil, err
		}
		opts = append(opts, sdktrace.WithBatcher(exp))
	}
	return &Provider{TracerProvider: sdktrace.NewTracerProvider(opts...), exporting: exporting}, nil
}

func endpointConfigured() bool {
	return os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "" || os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != ""
}
