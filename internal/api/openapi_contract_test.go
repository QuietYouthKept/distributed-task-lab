package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenAPIContainsEveryStableRoute(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.json"))
	if err != nil {
		t.Fatalf("read OpenAPI contract: %v", err)
	}
	var document struct {
		OpenAPI string                                `json:"openapi"`
		Paths   map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("parse OpenAPI contract: %v", err)
	}
	if document.OpenAPI != "3.1.0" {
		t.Fatalf("openapi=%q, want 3.1.0", document.OpenAPI)
	}

	want := map[string][]string{
		"/v1/tasks":                {"get", "post"},
		"/v1/tasks/{id}":           {"get"},
		"/v1/tasks/{id}/events":    {"get"},
		"/v1/tasks/{id}/cancel":    {"post"},
		"/v1/tasks/{id}/requeue":   {"post"},
		"/v1/dlq":                  {"get"},
		"/v1/stats":                {"get"},
		"/v1/metrics":              {"get"},
		"/v1/queues":               {"get"},
		"/v1/queues/{name}":        {"put"},
		"/v1/queues/{name}/pause":  {"post"},
		"/v1/queues/{name}/resume": {"post"},
		"/v1/effects":              {"get"},
		"/v1/effects/{id}/resolve": {"post"},
	}
	for path, methods := range want {
		operations, ok := document.Paths[path]
		if !ok {
			t.Errorf("OpenAPI missing path %s", path)
			continue
		}
		for _, method := range methods {
			if _, ok := operations[method]; !ok {
				t.Errorf("OpenAPI missing operation %s %s", method, path)
			}
		}
	}
}

func TestOpenAPICreateContractIsStrictAndIdempotent(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	components := document["components"].(map[string]any)
	schemas := components["schemas"].(map[string]any)
	create := schemas["CreateTaskRequest"].(map[string]any)
	if additional, ok := create["additionalProperties"].(bool); !ok || additional {
		t.Fatal("CreateTaskRequest must reject unknown top-level fields")
	}
	paths := document["paths"].(map[string]any)
	post := paths["/v1/tasks"].(map[string]any)["post"].(map[string]any)
	parameters := post["parameters"].([]any)
	found := false
	for _, value := range parameters {
		parameter := value.(map[string]any)
		if parameter["name"] == "Idempotency-Key" && parameter["in"] == "header" {
			found = true
		}
	}
	if !found {
		t.Fatal("POST /v1/tasks must document Idempotency-Key")
	}
	list := paths["/v1/tasks"].(map[string]any)["get"].(map[string]any)
	listParameters := list["parameters"].([]any)
	for _, expected := range []string{"page_size", "cursor"} {
		found := false
		for _, value := range listParameters {
			if value.(map[string]any)["name"] == expected {
				found = true
			}
		}
		if !found {
			t.Fatalf("GET /v1/tasks must document %s", expected)
		}
	}
}

func TestOpenAPIV1RolesAuthErrorsAndRouteSet(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "openapi.json"))
	if err != nil {
		t.Fatalf("read OpenAPI contract: %v", err)
	}
	var document struct {
		Security []map[string][]string `json:"security"`
		Paths    map[string]map[string]struct {
			OperationID string                     `json:"operationId"`
			Role        string                     `json:"x-required-role"`
			Responses   map[string]json.RawMessage `json:"responses"`
		} `json:"paths"`
		Components struct {
			SecuritySchemes map[string]struct {
				Type   string `json:"type"`
				Scheme string `json:"scheme"`
			} `json:"securitySchemes"`
			Responses map[string]struct {
				Description string `json:"description"`
				Content     map[string]struct {
					Schema struct {
						Ref string `json:"$ref"`
					} `json:"schema"`
				} `json:"content"`
			} `json:"responses"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("parse OpenAPI contract: %v", err)
	}
	if len(document.Security) != 1 || len(document.Security[0]) != 1 {
		t.Fatalf("global security=%v, want bearerAuth", document.Security)
	}
	if _, ok := document.Security[0]["bearerAuth"]; !ok {
		t.Fatalf("global security=%v, want bearerAuth", document.Security)
	}
	bearer := document.Components.SecuritySchemes["bearerAuth"]
	if bearer.Type != "http" || bearer.Scheme != "bearer" {
		t.Fatalf("bearerAuth=%+v, want HTTP Bearer", bearer)
	}

	want := map[string]map[string]string{
		"/v1/tasks":                {"get": "reader", "post": "operator"},
		"/v1/tasks/{id}":           {"get": "reader"},
		"/v1/tasks/{id}/events":    {"get": "reader"},
		"/v1/tasks/{id}/cancel":    {"post": "operator"},
		"/v1/tasks/{id}/requeue":   {"post": "operator"},
		"/v1/dlq":                  {"get": "operator"},
		"/v1/stats":                {"get": "operator"},
		"/v1/metrics":              {"get": "operator"},
		"/v1/queues":               {"get": "operator"},
		"/v1/queues/{name}":        {"put": "admin"},
		"/v1/queues/{name}/pause":  {"post": "admin"},
		"/v1/queues/{name}/resume": {"post": "admin"},
		"/v1/effects":              {"get": "operator"},
		"/v1/effects/{id}/resolve": {"post": "admin"},
	}
	if len(document.Paths) != len(want) {
		t.Fatalf("OpenAPI path count=%d, want exactly %d stable v1 paths", len(document.Paths), len(want))
	}
	operationIDs := map[string]string{}
	for path, methods := range document.Paths {
		wantMethods, knownPath := want[path]
		if !knownPath {
			t.Errorf("OpenAPI contains undocumented stable path %s", path)
			continue
		}
		if len(methods) != len(wantMethods) {
			t.Errorf("OpenAPI methods for %s=%d, want exactly %d", path, len(methods), len(wantMethods))
		}
		for method, operation := range methods {
			role, knownMethod := wantMethods[method]
			if !knownMethod {
				t.Errorf("OpenAPI contains undocumented operation %s %s", method, path)
				continue
			}
			if operation.Role != role {
				t.Errorf("%s %s x-required-role=%q, want %q", method, path, operation.Role, role)
			}
			if operation.OperationID == "" {
				t.Errorf("%s %s has no operationId", method, path)
			} else if previous, duplicate := operationIDs[operation.OperationID]; duplicate {
				t.Errorf("operationId %q is duplicated by %s and %s %s", operation.OperationID, previous, method, path)
			} else {
				operationIDs[operation.OperationID] = method + " " + path
			}
			for _, status := range []string{"401", "403", "429"} {
				if _, ok := operation.Responses[status]; !ok {
					t.Errorf("%s %s is missing documented %s response", method, path, status)
				}
			}
		}
	}
	for _, name := range []string{"Unauthorized", "Forbidden", "RateLimited"} {
		response, ok := document.Components.Responses[name]
		if !ok {
			t.Errorf("missing common response %s", name)
			continue
		}
		content, ok := response.Content["application/json"]
		if !ok || content.Schema.Ref != "#/components/schemas/ErrorResponse" {
			t.Errorf("common response %s does not use ErrorResponse envelope", name)
		}
	}
}
