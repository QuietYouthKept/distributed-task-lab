// Package taskpayload validates the task-type-specific portion of an enqueue
// request before it is persisted. Workers invoke the same validation defensively
// so legacy rows cannot panic a handler.
package taskpayload

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strings"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const payloadVersion = "1"

// ExecutionPolicy is a deployment-time allowlist, evaluated separately from
// structural payload validation. A non-nil empty policy rejects all work.
type ExecutionPolicy map[domain.TaskType]map[string]struct{}

func (p ExecutionPolicy) Allows(taskType domain.TaskType, mode string) bool {
	if p == nil {
		return true // local development compatibility only
	}
	_, ok := p[taskType][mode]
	return ok
}

// ProductionExecutionPolicy intentionally remains empty until a handler has a
// complete verified provider, shared artifact, and recovery path. This is a
// fail-closed allowlist, not a claim that production execution is available.
func ProductionExecutionPolicy() ExecutionPolicy { return ExecutionPolicy{} }

// StagingSimulationExecutionPolicy enables only the candidate eval/gateway
// path used by the isolated local staging simulation. It is intentionally a
// separate policy from ProductionExecutionPolicy and must never be selected by
// the production profile.
func StagingSimulationExecutionPolicy() ExecutionPolicy {
	return ExecutionPolicy{
		domain.TaskTypeEval: {"gateway": {}},
	}
}

var sha256Pattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

// Validate validates an envelope and returns a canonical copy of its data.
func Validate(taskType domain.TaskType, payload domain.TaskPayloadEnvelope) (map[string]any, error) {
	if payload.Version != payloadVersion {
		return nil, fmt.Errorf("payload.version must be %q", payloadVersion)
	}
	if payload.Kind != string(taskType) {
		return nil, fmt.Errorf("payload.kind must match task type %q", taskType)
	}
	return ValidateData(taskType, payload.Data)
}

// ValidateData is used at the worker boundary for payload data read from an
// existing queue row. It deliberately has the same field rules as Validate.
func ValidateData(taskType domain.TaskType, data map[string]any) (map[string]any, error) {
	if data == nil {
		return nil, fmt.Errorf("payload.data is required")
	}
	copy := make(map[string]any, len(data))
	for key, value := range data {
		copy[key] = value
	}
	traceMetadata, err := takeTraceMetadata(copy)
	if err != nil {
		return nil, err
	}

	var validated map[string]any
	var validateErr error
	switch taskType {
	case domain.TaskTypeRAGIngestion:
		validated, validateErr = validateRAG(copy)
	case domain.TaskTypeEval:
		validated, validateErr = validateEval(copy)
	case domain.TaskTypeBenchmark:
		validated, validateErr = validateBenchmark(copy)
	case domain.TaskTypeStrategyReplay:
		validated, validateErr = validateStrategyReplay(copy)
	case domain.TaskTypePayment, domain.TaskTypeNotification:
		validated, validateErr = validateEffect(taskType, copy)
	default:
		return nil, fmt.Errorf("unsupported task type %q", taskType)
	}
	if validateErr != nil {
		return nil, validateErr
	}
	for key, value := range traceMetadata {
		validated[key] = value
	}
	return validated, nil
}

// takeTraceMetadata temporarily removes the API-owned propagation fields from
// strict task-type validation, then ValidateData restores them to the
// canonical payload. The HTTP handler discards any client values before it
// injects its authenticated server span, so these reserved keys cannot forge
// the producer context at the external API boundary.
func takeTraceMetadata(data map[string]any) (map[string]any, error) {
	metadata := make(map[string]any, 2)
	carrier := propagation.MapCarrier{}
	for _, field := range []struct{ key, header string }{
		{key: "_traceparent", header: "traceparent"},
		{key: "_tracestate", header: "tracestate"},
	} {
		value, exists := data[field.key]
		if !exists {
			continue
		}
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return nil, fmt.Errorf("reserved trace metadata %s must be a non-empty string", field.key)
		}
		metadata[field.key] = text
		carrier.Set(field.header, text)
		delete(data, field.key)
	}
	if _, hasTraceState := metadata["_tracestate"]; hasTraceState {
		if _, hasTraceParent := metadata["_traceparent"]; !hasTraceParent {
			return nil, fmt.Errorf("reserved trace metadata _tracestate requires _traceparent")
		}
	}
	if _, hasTraceParent := metadata["_traceparent"]; hasTraceParent {
		extracted := propagation.TraceContext{}.Extract(context.Background(), carrier)
		if !trace.SpanContextFromContext(extracted).IsValid() {
			return nil, fmt.Errorf("reserved trace metadata _traceparent is invalid")
		}
	}
	return metadata, nil
}

func validateRAG(data map[string]any) (map[string]any, error) {
	if err := allowOnly(data, "execution_mode", "document_id", "dataset_id", "source_uri", "object_key", "version", "chunking_profile", "embedding_profile", "index_name", "simulate", "duration_ms", "mock_duration_ms", "mock_failures_before_success", "mock_always_fail"); err != nil {
		return nil, err
	}
	mode, err := requiredEnum(data, "execution_mode", "mock", "real")
	if err != nil {
		return nil, err
	}
	if _, err := requiredString(data, "document_id", 256); err != nil {
		return nil, err
	}
	if err := optionalStringFields(data, 512, "dataset_id", "source_uri", "object_key", "version", "chunking_profile", "embedding_profile", "index_name"); err != nil {
		return nil, err
	}
	if mode == "real" && !hasNonEmptyString(data, "object_key") {
		return nil, fmt.Errorf("real rag_ingestion requires object_key")
	}
	return validateMockControls(data)
}

func validateEval(data map[string]any) (map[string]any, error) {
	if err := allowOnly(data, "execution_mode", "eval_set_id", "model_profile", "scorer_profile", "sample_limit", "dataset_version", "prompt_version", "run_id", "case_start", "case_end", "version", "dataset_uri", "dataset_sha256", "retriever_version", "evaluator_schema_version"); err != nil {
		return nil, err
	}
	mode, err := requiredEnum(data, "execution_mode", "mock", "deterministic", "gateway")
	if err != nil {
		return nil, err
	}
	if mode == "mock" {
		if _, err := requiredString(data, "eval_set_id", 256); err != nil {
			return nil, err
		}
		if _, err := requiredString(data, "model_profile", 256); err != nil {
			return nil, err
		}
		if err := optionalPositiveInt(data, "sample_limit", 1, 100000); err != nil {
			return nil, err
		}
		return data, nil
	}
	for _, field := range []string{"dataset_version", "model_profile", "prompt_version", "run_id"} {
		if _, err := requiredString(data, field, 256); err != nil {
			return nil, err
		}
	}
	start, err := requiredInt(data, "case_start", 0, 10000000)
	if err != nil {
		return nil, err
	}
	end, err := requiredInt(data, "case_end", 1, 10000000)
	if err != nil {
		return nil, err
	}
	if end <= start {
		return nil, fmt.Errorf("case_end must be greater than case_start")
	}
	if mode == "gateway" {
		if _, err := requiredEnum(data, "version", "2"); err != nil {
			return nil, fmt.Errorf("gateway eval requires version 2")
		}
		for _, field := range []string{"dataset_uri", "dataset_sha256", "retriever_version", "evaluator_schema_version"} {
			if _, err := requiredString(data, field, 512); err != nil {
				return nil, err
			}
		}
		if !strings.HasPrefix(data["dataset_uri"].(string), "artifact://") {
			return nil, fmt.Errorf("dataset_uri must use the artifact scheme")
		}
		if !sha256Pattern.MatchString(data["dataset_sha256"].(string)) {
			return nil, fmt.Errorf("dataset_sha256 must be 64 hexadecimal characters")
		}
	}
	return data, nil
}

func validateBenchmark(data map[string]any) (map[string]any, error) {
	if err := allowOnly(data, "execution_mode", "benchmark_name", "concurrency", "simulate", "duration_ms", "mock_duration_ms", "mock_failures_before_success", "mock_always_fail"); err != nil {
		return nil, err
	}
	if _, err := requiredEnum(data, "execution_mode", "mock"); err != nil {
		return nil, err
	}
	if _, err := requiredString(data, "benchmark_name", 256); err != nil {
		return nil, err
	}
	if err := optionalPositiveInt(data, "concurrency", 1, 100000); err != nil {
		return nil, err
	}
	return validateMockControls(data)
}

func validateStrategyReplay(data map[string]any) (map[string]any, error) {
	if err := allowOnly(data, "execution_mode", "proposal_id", "job_schema_version", "replay_result_schema_version", "simulator_version", "execution_fingerprint", "strategy_key", "candidate_config", "case_count", "proposal_version", "scenario_set", "scenario_set_version", "policy_version", "seed", "guardrail_config"); err != nil {
		return nil, err
	}
	if _, err := requiredEnum(data, "execution_mode", "replay"); err != nil {
		return nil, err
	}
	for _, field := range []string{"proposal_id", "job_schema_version", "replay_result_schema_version", "simulator_version", "execution_fingerprint", "strategy_key"} {
		if _, err := requiredString(data, field, 256); err != nil {
			return nil, err
		}
	}
	if _, ok := data["candidate_config"].(map[string]any); !ok {
		return nil, fmt.Errorf("candidate_config must be an object")
	}
	if err := optionalPositiveInt(data, "case_count", 1, 1000000); err != nil {
		return nil, err
	}
	return data, nil
}

func validateEffect(taskType domain.TaskType, data map[string]any) (map[string]any, error) {
	if err := allowOnly(data, "execution_mode", "operation", "operation_id", "target", "simulate_timeout_before_apply", "simulate_response_loss"); err != nil {
		return nil, err
	}
	if _, err := requiredEnum(data, "execution_mode", "effect"); err != nil {
		return nil, err
	}
	operation, err := requiredEnum(data, "operation", "payment", "notification")
	if err != nil {
		return nil, err
	}
	if operation != string(taskType) {
		return nil, fmt.Errorf("operation must match task type %q", taskType)
	}
	for _, field := range []string{"operation_id", "target"} {
		if _, err := requiredString(data, field, 512); err != nil {
			return nil, err
		}
	}
	for _, field := range []string{"simulate_timeout_before_apply", "simulate_response_loss"} {
		if value, ok := data[field]; ok {
			if _, ok := value.(bool); !ok {
				return nil, fmt.Errorf("%s must be boolean", field)
			}
		}
	}
	return data, nil
}

func validateMockControls(data map[string]any) (map[string]any, error) {
	if value, ok := data["simulate"]; ok {
		if _, ok := value.(string); !ok {
			return nil, fmt.Errorf("simulate must be a string")
		}
	}
	for _, field := range []string{"duration_ms", "mock_duration_ms", "mock_failures_before_success"} {
		if err := optionalInteger(data, field, 0, 3600000); err != nil {
			return nil, err
		}
	}
	if value, ok := data["mock_always_fail"]; ok {
		if _, ok := value.(bool); !ok {
			return nil, fmt.Errorf("mock_always_fail must be boolean")
		}
	}
	return data, nil
}

func allowOnly(data map[string]any, fields ...string) error {
	allowed := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		allowed[field] = struct{}{}
	}
	for field := range data {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("payload.data contains unknown field %q", field)
		}
	}
	return nil
}
func requiredString(data map[string]any, field string, max int) (string, error) {
	value, ok := data[field].(string)
	value = strings.TrimSpace(value)
	if !ok || value == "" || len(value) > max {
		return "", fmt.Errorf("%s must be a non-empty string up to %d characters", field, max)
	}
	data[field] = value
	return value, nil
}
func optionalStringFields(data map[string]any, max int, fields ...string) error {
	for _, field := range fields {
		if _, ok := data[field]; ok {
			if _, err := requiredString(data, field, max); err != nil {
				return err
			}
		}
	}
	return nil
}
func hasNonEmptyString(data map[string]any, field string) bool {
	value, ok := data[field].(string)
	return ok && strings.TrimSpace(value) != ""
}
func requiredEnum(data map[string]any, field string, allowed ...string) (string, error) {
	value, err := requiredString(data, field, 64)
	if err != nil {
		return "", err
	}
	for _, candidate := range allowed {
		if value == candidate {
			return value, nil
		}
	}
	return "", fmt.Errorf("%s has unsupported value %q", field, value)
}
func requiredInt(data map[string]any, field string, min, max int) (int, error) {
	value, ok := integer(data[field])
	if !ok || value < min || value > max {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", field, min, max)
	}
	data[field] = value
	return value, nil
}
func optionalInteger(data map[string]any, field string, min, max int) error {
	if _, ok := data[field]; !ok {
		return nil
	}
	_, err := requiredInt(data, field, min, max)
	return err
}
func optionalPositiveInt(data map[string]any, field string, min, max int) error {
	return optionalInteger(data, field, min, max)
}
func integer(value any) (int, bool) {
	switch n := value.(type) {
	case int:
		return n, true
	case float64:
		if math.Trunc(n) == n && n >= math.MinInt && n <= math.MaxInt {
			return int(n), true
		}
	}
	return 0, false
}
