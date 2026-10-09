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
