package vertexai_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-provider-google/google/acctest"
	"github.com/hashicorp/terraform-provider-google/google/services/vertexai"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
	"google.golang.org/api/googleapi"
)

func TestUnitDataSourceVertexAIReasoningEngineQuery_404PreservesDetails(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || req.URL.Path != "/v1beta1/projects/my-proj/locations/us-central1/reasoningEngines/987654:query" {
			t.Errorf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(http.StatusNotFound)
		_, _ = rw.Write([]byte(`{
			"error": {
				"code": 404,
				"message": "Requested entity was not found.",
				"status": "NOT_FOUND",
				"details": [
					{
						"@type": "type.googleapis.com/google.rpc.ErrorInfo",
						"reason": "EXAMPLE_REASON",
						"domain": "example.googleapis.com",
						"metadata": {
							"logs_url": "https://console.cloud.google.com/logs/query"
						}
					},
					{
						"@type": "type.googleapis.com/google.rpc.Help",
						"links": [
							{
								"description": "Troubleshooting",
								"url": "https://cloud.google.com/troubleshooting"
							}
						]
					}
				]
			}
		}`))
	}))
	defer srv.Close()

	ds := vertexai.DataSourceVertexAIReasoningEngineQuery()
	d := ds.TestResourceData()
	if err := d.Set("project", "my-proj"); err != nil {
		t.Fatalf("d.Set(project): %v", err)
	}
	if err := d.Set("region", "us-central1"); err != nil {
		t.Fatalf("d.Set(region): %v", err)
	}
	if err := d.Set("reasoning_engine_id", "987654"); err != nil {
		t.Fatalf("d.Set(reasoning_engine_id): %v", err)
	}

	cfg := &transport_tpg.Config{
		Client: srv.Client(),
		CustomEndpoints: map[string]string{
			vertexai.Product.CustomEndpointField: srv.URL + "/v1beta1/",
		},
	}

	err := ds.Read(d, cfg)
	if err == nil {
		t.Fatal("expected non-nil error on HTTP 404")
	}

	var gerr *googleapi.Error
	if !errors.As(err, &gerr) {
		t.Fatalf("expected error to wrap *googleapi.Error, got %T: %v", err, err)
	}
	if gerr.Code != http.StatusNotFound {
		t.Errorf("got status code %d, want 404", gerr.Code)
	}

	got := err.Error()
	for _, want := range []string{
		`Error querying Vertex AI Reasoning Engine "projects/my-proj/locations/us-central1/reasoningEngines/987654": googleapi: Error 404: Requested entity was not found.`,
		`"reason": "EXAMPLE_REASON"`,
		`"logs_url": "https://console.cloud.google.com/logs/query"`,
		`"url": "https://cloud.google.com/troubleshooting"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("error output missing %q\ngot:\n%s", want, got)
		}
	}
}

func TestAccDataSourceVertexAIReasoningEngineQuery_basic(t *testing.T) {
	t.Parallel()

	context := map[string]interface{}{"random_suffix": acctest.RandString(t, 10)}

	acctest.VcrTest(t, resource.TestCase{
		PreCheck:                 func() { acctest.AccTestPreCheck(t) },
		ProtoV5ProviderFactories: acctest.ProtoV5ProviderFactories(t),
		Steps: []resource.TestStep{
			{
				Config: testAccDataSourceVertexAIReasoningEngineQuery_basic(context),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrSet("data.google_vertex_ai_reasoning_engine_query.default", "output"),
				),
			},
		},
	})
}

func testAccDataSourceVertexAIReasoningEngineQuery_basic(context map[string]interface{}) string {
	return acctest.Nprintf(`
resource "google_vertex_ai_reasoning_engine" "default" {
  display_name = "tf-test-vertex-mds-%{random_suffix}"
  description  = "Retrieves tenant project number"
  region       = "us-central1"

  spec {
    source_code_spec {
      inline_source {
        source_archive = filebase64("./test-fixtures/mds_agent_src.tar.gz")
      }

      python_spec {
        entrypoint_module = "metadata_agent"
        entrypoint_object = "root_agent"
      }
    }
  }
}

data "google_vertex_ai_reasoning_engine_query" "default" {
  reasoning_engine_id = google_vertex_ai_reasoning_engine.default.name
  region              = "us-central1"
  class_method        = "query"
}
`, context)
}
