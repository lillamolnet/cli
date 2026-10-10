package api

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// The checked-in copy of the API's OpenAPI spec. The responses in testdata
// must follow it, so that the tests here fail when the CLI and the API no
// longer agree. Update it with scripts/update-openapi.sh.
const specPath = "../../api/openapi.json"

var (
	specOnce sync.Once
	spec     *openapi3.T
	specErr  error
)

func loadSpec(t *testing.T) *openapi3.T {
	t.Helper()
	specOnce.Do(func() {
		spec, specErr = openapi3.NewLoader().LoadFromFile(specPath)
		if specErr == nil {
			specErr = spec.Validate(context.Background())
		}
	})
	if specErr != nil {
		t.Fatalf("%s: %v", specPath, specErr)
	}
	return spec
}

func TestSpecIsValidOpenAPI(t *testing.T) {
	loadSpec(t)
}

// assertFollowsSpec checks a successful GET response body against the spec.
func assertFollowsSpec(t *testing.T, path string, body []byte) {
	t.Helper()
	assertFollowsSpecFor(t, "GET", path, 200, body)
}

// assertFollowsSpecFor checks a response body against the spec.
func assertFollowsSpecFor(t *testing.T, method, path string, status int, body []byte) {
	t.Helper()
	doc := loadSpec(t)

	item := doc.Paths.Find(path)
	if item == nil || item.GetOperation(method) == nil {
		t.Fatalf("the spec has no %s %s", method, path)
	}
	response := item.GetOperation(method).Responses.Status(status)
	if response == nil {
		t.Fatalf("the spec has no %d response for %s %s", status, method, path)
	}
	media := response.Value.Content.Get(mediaType)
	if media == nil {
		t.Fatalf("the spec has no %s response for %s %s", mediaType, method, path)
	}

	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if err := media.Schema.Value.VisitJSON(value); err != nil {
		t.Errorf("%s %s does not follow the spec: %v", method, path, err)
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
