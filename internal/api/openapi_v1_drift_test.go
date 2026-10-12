package api

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/api/contract"
	"github.com/fantasticyi/distributed-task-lab/internal/application"
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
)

type openAPIDocument map[string]any

func TestOpenAPIV1OperationsMatchRouterAndHaveTypedContracts(t *testing.T) {
	document := readOpenAPIDocument(t)
	paths := objectAt(t, document, "paths")
	routes := stableV1RouterOperations(t)
	if !reflect.DeepEqual(sortedMapKeys(paths), sortedMapKeys(routes)) {
		t.Fatalf("OpenAPI paths differ from stable router paths: openapi=%v router=%v", sortedMapKeys(paths), sortedMapKeys(routes))
	}

	bindings := []struct {
		method, path, operationID, role string
		requestType                     reflect.Type
		requestRequired                 []string
		requestOptional                 bool
		responseType                    reflect.Type
		responseMediaType               string
	}{
		{http.MethodGet, "/v1/tasks", "listTasks", "reader", nil, nil, false, reflect.TypeOf(contract.TaskListResponse{}), "application/json"},
		{http.MethodPost, "/v1/tasks", "createTask", "operator", reflect.TypeOf(application.CreateTaskRequest{}), []string{"payload"}, false, reflect.TypeOf(application.CreateTaskResponse{}), "application/json"},
		{http.MethodGet, "/v1/tasks/{id}", "getTask", "reader", nil, nil, false, reflect.TypeOf(domain.Task{}), "application/json"},
		{http.MethodGet, "/v1/tasks/{id}/events", "getTaskEvents", "reader", nil, nil, false, reflect.TypeOf([]domain.TaskEvent{}), "application/json"},
		{http.MethodPost, "/v1/tasks/{id}/cancel", "cancelTask", "operator", reflect.TypeOf(contract.OptionalReasonRequest{}), nil, true, reflect.TypeOf(contract.TaskActionResponse{}), "application/json"},
		{http.MethodPost, "/v1/tasks/{id}/requeue", "requeueTask", "operator", reflect.TypeOf(contract.OptionalReasonRequest{}), nil, true, reflect.TypeOf(contract.TaskActionResponse{}), "application/json"},
		{http.MethodGet, "/v1/dlq", "listDLQ", "operator", nil, nil, false, reflect.TypeOf([]domain.Task{}), "application/json"},
		{http.MethodGet, "/v1/stats", "getStats", "operator", nil, nil, false, reflect.TypeOf(contract.TaskStatsResponse{}), "application/json"},
		{http.MethodGet, "/v1/metrics", "getMetrics", "operator", nil, nil, false, reflect.TypeOf(""), "text/plain"},
		{http.MethodGet, "/v1/queues", "listQueues", "operator", nil, nil, false, reflect.TypeOf([]domain.QueueConfig{}), "application/json"},
		{http.MethodPut, "/v1/queues/{name}", "setQueueConfig", "admin", reflect.TypeOf(contract.QueueConfigRequest{}), nil, true, reflect.TypeOf(contract.QueueActionResponse{}), "application/json"},
		{http.MethodPost, "/v1/queues/{name}/pause", "pauseQueue", "admin", nil, nil, false, reflect.TypeOf(contract.QueueActionResponse{}), "application/json"},
		{http.MethodPost, "/v1/queues/{name}/resume", "resumeQueue", "admin", nil, nil, false, reflect.TypeOf(contract.QueueActionResponse{}), "application/json"},
		{http.MethodGet, "/v1/effects", "listUnresolvedEffects", "operator", nil, nil, false, reflect.TypeOf(contract.EffectsListResponse{}), "application/json"},
		{http.MethodPost, "/v1/effects/{id}/resolve", "resolveEffectManually", "admin", reflect.TypeOf(contract.OptionalReasonRequest{}), []string{"reason"}, false, reflect.TypeOf(contract.EffectResolutionResponse{}), "application/json"},
	}
	if len(bindings) != 15 {
		t.Fatalf("test contract binds %d operations, want 15", len(bindings))
	}
	for _, binding := range bindings {
		binding := binding
		t.Run(binding.operationID, func(t *testing.T) {
			pathItem := objectAt(t, document, "paths", binding.path)
			operation, ok := pathItem[strings.ToLower(binding.method)].(map[string]any)
			if !ok {
				t.Fatalf("OpenAPI missing %s %s", binding.method, binding.path)
			}
			if operation["operationId"] != binding.operationID {
				t.Errorf("operationId=%v, want %s", operation["operationId"], binding.operationID)
			}
			if operation["x-required-role"] != binding.role {
				t.Errorf("x-required-role=%v, want %s", operation["x-required-role"], binding.role)
			}
			if binding.requestType != nil {
				body := resolveObjectRef(t, document, objectAt(t, operation, "requestBody"))
				if body["required"] != !binding.requestOptional {
					t.Errorf("requestBody.required=%v, want %v", body["required"], !binding.requestOptional)
				}
				content := objectAt(t, body, "content")
				media := objectAt(t, content, "application/json")
				schema := resolveObjectRef(t, document, objectAt(t, media, "schema"))
				required := binding.requestRequired
				if required == nil {
					required = []string{}
				}
				assertSchemaMatchesGoType(t, document, schema, binding.requestType, required)
			}
			responses := objectAt(t, operation, "responses")
			successStatus := "200"
			if binding.method == http.MethodPost && binding.path == "/v1/tasks" {
				successStatus = "201"
			}
			success := resolveObjectRef(t, document, objectAt(t, responses, successStatus))
			content := objectAt(t, success, "content")
			if _, ok := content[binding.responseMediaType]; !ok {
				t.Fatalf("%s %s success response missing %s content", binding.method, binding.path, binding.responseMediaType)
			}
			media := objectAt(t, content, binding.responseMediaType)
			schema := resolveObjectRef(t, document, objectAt(t, media, "schema"))
			assertSchemaMatchesGoType(t, document, schema, binding.responseType, nil)
			for _, status := range []string{"401", "403", "429", "500"} {
				if _, ok := responses[status]; !ok {
					t.Errorf("%s %s missing response %s", binding.method, binding.path, status)
				}
			}
			if responses["401"] != nil {
				assertResponseSchemaRef(t, document, responses["401"], "#/components/schemas/ErrorResponse")
			}
			if responses["403"] != nil {
				assertResponseSchemaRef(t, document, responses["403"], "#/components/schemas/ErrorResponse")
			}
			if responses["429"] != nil {
				limited := resolveObjectRef(t, document, objectAt(t, responses, "429"))
				headers := objectAt(t, limited, "headers")
				if _, ok := headers["Retry-After"]; !ok {
					t.Error("429 response must document Retry-After")
				}
				assertResponseSchemaRef(t, document, responses["429"], "#/components/schemas/ErrorResponse")
			}
			if successStatus != "200" {
				if _, ok := responses["400"]; !ok {
					t.Error("JSON-body operation must document malformed-request response")
				}
			}
		})
	}

	t.Run("stable-enums-match-domain", func(t *testing.T) {
		assertEnumMatches(t, document, "TaskStatus", mapKeys(domain.ValidTaskStatuses))
		assertEnumMatches(t, document, "TaskType", mapKeys(domain.ValidTaskTypes))
		assertEnumMatches(t, document, "TaskEventType", []string{
			string(domain.EventSubmitted), string(domain.EventClaimed), string(domain.EventSucceeded),
			string(domain.EventFailed), string(domain.EventRetried), string(domain.EventMovedToDLQ),
			string(domain.EventCancelled), string(domain.EventRequeued), string(domain.EventPriorityPromoted),
		})
	})

	t.Run("create-task-optional-and-nullable-semantics", func(t *testing.T) {
		if got := objectAt(t, document, "components", "schemas", "CreateTaskRequest")["required"].([]any); !reflect.DeepEqual(stringSlice(got), []string{"payload"}) {
			t.Fatalf("CreateTaskRequest required=%v, want only payload (type defaults in handler)", got)
		}
		schemas := objectAt(t, document, "components", "schemas")
		for _, field := range []string{"type", "queue", "priority", "max_attempts", "timeout_seconds", "retry_delay_seconds", "retry_max_delay_seconds", "retry_backoff_multiplier", "retry_jitter", "idempotency_key"} {
			property := objectAt(t, schemas, "CreateTaskRequest", "properties", field)
			types := stringSlice(mustAnySlice(t, property["type"]))
			if !contains(types, "null") {
				t.Errorf("CreateTaskRequest.%s must accept explicit JSON null per encoding/json behavior", field)
			}
		}
		if !contains(stringSlice(mustAnySlice(t, objectAt(t, schemas, "CreateTaskRequest", "properties", "type")["enum"])), "") {
			t.Error("CreateTaskRequest.type must document the empty-string default behavior")
		}
		for _, field := range []string{"max_concurrency", "rate_limit_per_second", "burst", "aging_after_seconds", "aging_priority_boost"} {
			property := objectAt(t, schemas, "QueueConfigRequest", "properties", field)
			if !contains(stringSlice(mustAnySlice(t, property["type"])), "null") {
				t.Errorf("QueueConfigRequest.%s must accept explicit null as its zero/omitted value", field)
			}
		}
		if !contains(stringSlice(mustAnySlice(t, objectAt(t, schemas, "OptionalReasonRequest", "properties", "reason")["type"])), "null") {
			t.Error("OptionalReasonRequest.reason must document null as the omitted/default value")
		}
	})
}

func TestOpenAPIRouterASTFindsRouteMethodDrift(t *testing.T) {
	document := readOpenAPIDocument(t)
	paths := objectAt(t, document, "paths")
	routes := stableV1RouterOperations(t)
	for path, methods := range routes {
		item := objectAt(t, paths, path)
		for method := range methods {
			if _, ok := item[method]; !ok {
				t.Errorf("router has %s %s but OpenAPI does not", method, path)
			}
		}
		for method := range item {
			if _, ok := methods[method]; !ok {
				t.Errorf("OpenAPI has %s %s but router does not", method, path)
			}
		}
	}
}

func TestOpenAPIV1ParametersAndStatusCodesMatchHandlerBehavior(t *testing.T) {
	document := readOpenAPIDocument(t)
	paths := objectAt(t, document, "paths")
	parameters := map[string][]string{
		"GET /v1/tasks":                 {"query:status", "query:type", "query:queue", "query:page_size", "query:cursor"},
		"POST /v1/tasks":                {"header:Idempotency-Key"},
		"GET /v1/tasks/{id}":            {"path:id"},
		"GET /v1/tasks/{id}/events":     {"path:id"},
		"POST /v1/tasks/{id}/cancel":    {"path:id"},
		"POST /v1/tasks/{id}/requeue":   {"path:id"},
		"GET /v1/dlq":                   {},
		"GET /v1/stats":                 {},
		"GET /v1/metrics":               {},
		"GET /v1/queues":                {},
		"PUT /v1/queues/{name}":         {"path:name"},
		"POST /v1/queues/{name}/pause":  {"path:name"},
		"POST /v1/queues/{name}/resume": {"path:name"},
		"GET /v1/effects":               {"query:limit"},
		"POST /v1/effects/{id}/resolve": {"path:id"},
	}
	statuses := map[string][]string{
		"GET /v1/tasks":                 {"200", "401", "403", "422", "429", "500", "503"},
		"POST /v1/tasks":                {"201", "400", "401", "403", "422", "429", "500", "503"},
		"GET /v1/tasks/{id}":            {"200", "401", "403", "404", "429", "500", "503"},
		"GET /v1/tasks/{id}/events":     {"200", "401", "403", "404", "429", "500", "503"},
		"POST /v1/tasks/{id}/cancel":    {"200", "400", "401", "403", "404", "409", "429", "500", "503"},
		"POST /v1/tasks/{id}/requeue":   {"200", "400", "401", "403", "404", "409", "429", "500", "503"},
		"GET /v1/dlq":                   {"200", "401", "403", "429", "500", "503"},
		"GET /v1/stats":                 {"200", "401", "403", "429", "500", "503"},
		"GET /v1/metrics":               {"200", "401", "403", "429", "500"},
		"GET /v1/queues":                {"200", "401", "403", "429", "500", "503"},
		"PUT /v1/queues/{name}":         {"200", "400", "401", "403", "422", "429", "500", "503"},
		"POST /v1/queues/{name}/pause":  {"200", "401", "403", "404", "429", "500", "503"},
		"POST /v1/queues/{name}/resume": {"200", "401", "403", "404", "429", "500", "503"},
		"GET /v1/effects":               {"200", "401", "403", "422", "429", "500", "503"},
		"POST /v1/effects/{id}/resolve": {"200", "401", "403", "409", "422", "429", "500", "503"},
	}
	for route, wantParameters := range parameters {
		parts := strings.SplitN(route, " ", 2)
		operation := objectAt(t, paths, parts[1], strings.ToLower(parts[0]))
		got := []string{}
		for _, raw := range mustAnySliceIfPresent(operation["parameters"]) {
			parameter := resolveObjectRef(t, document, raw.(map[string]any))
			got = append(got, parameter["in"].(string)+":"+parameter["name"].(string))
		}
		sort.Strings(got)
		sort.Strings(wantParameters)
		if !reflect.DeepEqual(got, wantParameters) {
			t.Errorf("%s parameters=%v, want %v", route, got, wantParameters)
		}
		responseObject := objectAt(t, operation, "responses")
		gotStatuses := sortedMapKeys(responseObject)
		wantStatuses := append([]string(nil), statuses[route]...)
		sort.Strings(wantStatuses)
		if !reflect.DeepEqual(gotStatuses, wantStatuses) {
			t.Errorf("%s response statuses=%v, want %v", route, gotStatuses, wantStatuses)
		}
	}
	if len(parameters) != 15 || len(statuses) != 15 {
		t.Fatalf("parameter/status catalog must cover all 15 operations; got %d and %d", len(parameters), len(statuses))
	}
}

func readOpenAPIDocument(t *testing.T) openAPIDocument {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.json"))
	if err != nil {
		t.Fatalf("read OpenAPI contract: %v", err)
	}
	var document openAPIDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("parse OpenAPI contract: %v", err)
	}
	return document
}

func stableV1RouterOperations(t *testing.T) map[string]map[string]struct{} {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "router.go", nil, 0)
	if err != nil {
		t.Fatalf("parse router.go: %v", err)
	}
	routes := map[string]map[string]struct{}{}
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "HandleFunc" {
			return true
		}
		literal, ok := call.Args[0].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		pattern, err := strconvUnquote(literal.Value)
		if err != nil || !strings.HasPrefix(pattern, "GET /v1/") && !strings.HasPrefix(pattern, "POST /v1/") && !strings.HasPrefix(pattern, "PUT /v1/") && !strings.HasPrefix(pattern, "DELETE /v1/") && !strings.HasPrefix(pattern, "PATCH /v1/") {
			return true
		}
		parts := strings.SplitN(pattern, " ", 2)
		if len(parts) != 2 {
			return true
		}
		if routes[parts[1]] == nil {
			routes[parts[1]] = map[string]struct{}{}
		}
		routes[parts[1]][strings.ToLower(parts[0])] = struct{}{}
		return true
	})
	return routes
}

func strconvUnquote(value string) (string, error) {
	return strconv.Unquote(value)
}

func assertSchemaMatchesGoType(t *testing.T, document openAPIDocument, schema map[string]any, typ reflect.Type, requiredOverride []string) {
	t.Helper()
	if typ == reflect.TypeOf(contract.TaskStatsResponse{}) {
		assertSchemaType(t, schema, "object")
		properties := objectAt(t, schema, "properties")
		want := []string{"pending", "running", "success", "failed", "dlq"}
		if !reflect.DeepEqual(sortedMapKeys(properties), sortedStrings(want)) {
			t.Errorf("stats properties=%v, want %v", sortedMapKeys(properties), want)
		}
		gotRequired := stringSliceIfPresent(schema["required"])
		if !reflect.DeepEqual(sortedStrings(gotRequired), sortedStrings(want)) {
			t.Errorf("stats required=%v, want %v", gotRequired, want)
		}
		for _, field := range want {
			assertSchemaType(t, objectAt(t, properties, field), "integer")
		}
		if schema["additionalProperties"] != false {
			t.Errorf("stats additionalProperties=%v, want false", schema["additionalProperties"])
		}
		return
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	schema = resolveObjectRef(t, document, schema)
	if typ == reflect.TypeOf(time.Time{}) {
		assertSchemaType(t, schema, "string")
		if schema["format"] != "date-time" {
			t.Errorf("time.Time schema format=%v, want date-time", schema["format"])
		}
		return
	}
	switch typ.Kind() {
	case reflect.Struct:
		assertSchemaType(t, schema, "object")
		properties := objectAt(t, schema, "properties")
		wantFields, wantRequired := jsonFields(typ)
		if !reflect.DeepEqual(sortedMapKeys(properties), sortedMapKeys(wantFields)) {
			t.Errorf("schema fields=%v, Go JSON fields=%v", sortedMapKeys(properties), sortedMapKeys(wantFields))
		}
		if requiredOverride != nil {
			wantRequired = append([]string(nil), requiredOverride...)
		}
		gotRequired := stringSliceIfPresent(schema["required"])
		sort.Strings(wantRequired)
		sort.Strings(gotRequired)
		if !reflect.DeepEqual(gotRequired, wantRequired) {
			t.Errorf("required=%v, Go/API contract requires %v", gotRequired, wantRequired)
		}
		if schema["additionalProperties"] != false {
			t.Errorf("struct schema additionalProperties=%v, want false", schema["additionalProperties"])
		}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name, omit, skip := jsonField(field)
			if skip {
				continue
			}
			property := objectAt(t, properties, name)
			assertSchemaMatchesGoType(t, document, property, field.Type, nil)
			if field.Type.Kind() == reflect.Pointer && !omit {
				assertNullable(t, property)
			}
		}
	case reflect.Slice, reflect.Array:
		assertSchemaType(t, schema, "array")
		assertSchemaMatchesGoType(t, document, objectAt(t, schema, "items"), typ.Elem(), nil)
	case reflect.Map:
		assertSchemaType(t, schema, "object")
		additional, ok := schema["additionalProperties"]
		if !ok {
			t.Errorf("map schema has no additionalProperties")
			return
		}
		if typ.Elem().Kind() == reflect.Interface {
			if additional != true {
				t.Errorf("map[string]any additionalProperties=%v, want true", additional)
			}
			return
		}
		valueSchema, ok := additional.(map[string]any)
		if !ok {
			t.Errorf("map additionalProperties=%T, want schema object", additional)
			return
		}
		assertSchemaMatchesGoType(t, document, valueSchema, typ.Elem(), nil)
	case reflect.Interface:
		// Interface-valued payload data intentionally accepts any JSON value.
		if schema["type"] != nil && schema["type"] != "object" {
			t.Errorf("interface schema type=%v, expected unconstrained JSON", schema["type"])
		}
	case reflect.String:
		assertSchemaType(t, schema, "string")
	case reflect.Bool:
		assertSchemaType(t, schema, "boolean")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		assertSchemaType(t, schema, "integer")
	case reflect.Float32, reflect.Float64:
		assertSchemaType(t, schema, "number")
	default:
		t.Errorf("schema matcher does not support Go type %s", typ)
	}
}

func jsonFields(typ reflect.Type) (map[string]reflect.Type, []string) {
	fields := map[string]reflect.Type{}
	var required []string
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name, omit, skip := jsonField(field)
		if skip {
			continue
		}
		fields[name] = field.Type
		if !omit {
			required = append(required, name)
		}
	}
	return fields, required
}

func jsonField(field reflect.StructField) (name string, omitEmpty, skip bool) {
	tag := field.Tag.Get("json")
	parts := strings.Split(tag, ",")
	name = parts[0]
	if name == "-" {
		return "", false, true
	}
	if name == "" {
		name = field.Name
	}
	for _, option := range parts[1:] {
		if option == "omitempty" {
			omitEmpty = true
		}
	}
	return name, omitEmpty, false
}

func assertSchemaType(t *testing.T, schema map[string]any, want string) {
	t.Helper()
	switch value := schema["type"].(type) {
	case string:
		if value != want {
			t.Errorf("schema type=%q, want %q", value, want)
		}
	case []any:
		if !contains(stringSlice(value), want) {
			t.Errorf("schema types=%v, want %q", value, want)
		}
	default:
		t.Errorf("schema type=%v, want %q", schema["type"], want)
	}
}

func assertNullable(t *testing.T, schema map[string]any) {
	t.Helper()
	if types, ok := schema["type"].([]any); ok && contains(stringSlice(types), "null") {
		return
	}
	t.Errorf("pointer field schema must include null: %v", schema)
}

func resolveObjectRef(t *testing.T, document openAPIDocument, object map[string]any) map[string]any {
	t.Helper()
	ref, ok := object["$ref"].(string)
	if !ok {
		return object
	}
	if !strings.HasPrefix(ref, "#/components/") {
		t.Fatalf("unsupported OpenAPI ref %q", ref)
	}
	var value any = map[string]any(document)
	for _, segment := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
		current, ok := value.(map[string]any)
		if !ok {
			t.Fatalf("cannot resolve OpenAPI ref %q", ref)
		}
		value, ok = current[strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~")]
		if !ok {
			t.Fatalf("cannot resolve OpenAPI ref %q", ref)
		}
	}
	resolved, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI ref %q points to %T, want object", ref, value)
	}
	return resolved
}

func assertResponseSchemaRef(t *testing.T, document openAPIDocument, response any, want string) {
	t.Helper()
	resolved := resolveObjectRef(t, document, response.(map[string]any))
	content, ok := resolved["content"].(map[string]any)
	if !ok {
		t.Fatalf("response %v has no content", response)
	}
	media, ok := content["application/json"].(map[string]any)
	if !ok {
		t.Fatalf("response %v has no application/json", response)
	}
	schema := media["schema"].(map[string]any)
	if schema["$ref"] != want {
		t.Errorf("response schema ref=%v, want %s", schema["$ref"], want)
	}
}

func assertEnumMatches(t *testing.T, document openAPIDocument, name string, want []string) {
	t.Helper()
	schema := objectAt(t, document, "components", "schemas", name)
	got := stringSlice(mustAnySlice(t, schema["enum"]))
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s enum=%v, Go values=%v", name, got, want)
	}
}

func objectAt(t *testing.T, root map[string]any, path ...string) map[string]any {
	t.Helper()
	var current any = root
	for _, part := range path {
		object, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("%s parent has type %T, want object", strings.Join(path, "."), current)
		}
		current, ok = object[part]
		if !ok {
			t.Fatalf("missing OpenAPI object path %s", strings.Join(path, "."))
		}
	}
	object, ok := current.(map[string]any)
	if !ok {
		t.Fatalf("OpenAPI path %s has type %T, want object", strings.Join(path, "."), current)
	}
	return object
}

func mustAnySlice(t *testing.T, value any) []any {
	t.Helper()
	values, ok := value.([]any)
	if !ok {
		t.Fatalf("value has type %T, want array", value)
	}
	return values
}

func mustAnySliceIfPresent(value any) []any {
	if value == nil {
		return nil
	}
	values, _ := value.([]any)
	return values
}

func stringSlice(values []any) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func stringSliceIfPresent(value any) []string {
	if value == nil {
		return nil
	}
	if values, ok := value.([]any); ok {
		return stringSlice(values)
	}
	return nil
}

func sortedMapKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mapKeys[K ~string, V any](values map[K]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, string(key))
	}
	return keys
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func TestOpenAPIErrorStatusesMatchErrorClassifier(t *testing.T) {
	document := readOpenAPIDocument(t)
	responses := objectAt(t, document, "components", "responses")
	statuses := map[string]string{
		"400": "MalformedRequest", "401": "Unauthorized", "403": "Forbidden", "404": "NotFound",
		"409": "Conflict", "422": "InvalidArgument", "429": "RateLimited", "500": "InternalError", "503": "Unavailable",
	}
	for status, name := range statuses {
		response := objectAt(t, responses, name)
		content := objectAt(t, response, "content")
		media := objectAt(t, content, "application/json")
		schema := objectAt(t, media, "schema")
		if schema["$ref"] != "#/components/schemas/ErrorResponse" {
			t.Errorf("%s response schema=%v, want ErrorResponse", status, schema["$ref"])
		}
	}
}
