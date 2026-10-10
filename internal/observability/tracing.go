package observability

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

func SetupTracing(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	if !strings.EqualFold(os.Getenv("OTEL_ENABLED"), "true") {
		return func(context.Context) error { return nil }, nil
	}
	opts := []otlptracehttp.Option{}
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		baseURL, err := url.Parse(endpoint)
		if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
			return nil, fmt.Errorf("invalid OTEL_EXPORTER_OTLP_ENDPOINT")
		}
		baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/v1/traces"
		opts = append(opts, otlptracehttp.WithEndpointURL(baseURL.String()))
	}
	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}
	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(serviceName)))
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(provider)
	return provider.Shutdown, nil
}

// RecordSpanFailure marks an operation as failed without attaching the raw
// error string, which may contain request data, object keys, or credentials.
func RecordSpanFailure(span trace.Span, err error, description string) {
	if err == nil {
		return
	}
	span.SetStatus(codes.Error, description)
	span.SetAttributes(attribute.String("error.type", fmt.Sprintf("%T", err)))
}
