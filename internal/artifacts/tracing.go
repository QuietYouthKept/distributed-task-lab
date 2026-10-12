package artifacts

import (
	"context"
	"errors"

	"github.com/fantasticyi/distributed-task-lab/internal/observability"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func startArtifactSpan(ctx context.Context, operation string) (context.Context, trace.Span) {
	return otel.Tracer("distributed-task-lab/artifacts").Start(ctx, "artifact."+operation,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("artifact.backend", "s3"),
			attribute.String("artifact.operation", operation),
		),
	)
}

func finishArtifactSpan(span trace.Span, err error) {
	observability.RecordSpanFailure(span, err, "artifact operation failed")
	if errors.Is(err, ErrUnknownOutcome) {
		span.SetAttributes(attribute.Bool("artifact.outcome_unknown", true))
	}
	span.End()
}
