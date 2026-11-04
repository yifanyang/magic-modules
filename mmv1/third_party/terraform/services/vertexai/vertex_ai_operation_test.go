package vertexai

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hashicorp/terraform-provider-google/google/tpgresource"
	transport_tpg "github.com/hashicorp/terraform-provider-google/google/transport"
)

func TestUnitVertexAIOperationWaiter_ErrorWithDetails(t *testing.T) {
	t.Parallel()

	op := map[string]interface{}{
		"name": "projects/my-proj/locations/us-central1/reasoningEngines/123/operations/456",
		"done": true,
		"error": map[string]interface{}{
			"code":    3,
			"message": "The Reasoning Engine could not be deployed because its container image failed to build.",
			"details": []interface{}{
				map[string]interface{}{
					"@type":  "type.googleapis.com/google.rpc.ErrorInfo",
					"reason": "REASONING_ENGINE_BUILD_FAILED",
					"domain": "aiplatform.googleapis.com",
					"metadata": map[string]interface{}{
						"logs_url": "https://console.cloud.google.com/logs/query?project=my-proj",
					},
				},
				map[string]interface{}{
					"@type": "type.googleapis.com/google.rpc.Help",
					"links": []interface{}{
						map[string]interface{}{
							"description": "Troubleshooting build failures",
							"url":         "https://docs.cloud.google.com/gemini-enterprise-agent-platform/troubleshooting/agent-deployment#build-failed",
						},
					},
				},
			},
		},
	}

	w := &VertexAIOperationWaiter{}
	if err := w.SetOp(op); err != nil {
		t.Fatalf("SetOp failed: %v", err)
	}

	err := w.Error()
	if err == nil {
		t.Fatal("expected non-nil error")
	}

	var commonOpErr *tpgresource.CommonOpError
	if !errors.As(err, &commonOpErr) {
		t.Fatalf("expected error to unwrap to *tpgresource.CommonOpError, got %T", err)
	}

	got := err.Error()
	for _, want := range []string{
		"Error code 3, message: The Reasoning Engine could not be deployed because its container image failed to build.\nDetails:\n[",
		`"reason": "REASONING_ENGINE_BUILD_FAILED"`,
		`"logs_url": "https://console.cloud.google.com/logs/query?project=my-proj"`,
		`"url": "https://docs.cloud.google.com/gemini-enterprise-agent-platform/troubleshooting/agent-deployment#build-failed"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("error output missing %q\ngot:\n%s", want, got)
		}
	}
}

func TestUnitVertexAIOperationWaiter_ErrorWithoutDetails(t *testing.T) {
	t.Parallel()

	op := map[string]interface{}{
		"name": "projects/my-proj/locations/us-central1/reasoningEngines/123/operations/456",
		"done": true,
		"error": map[string]interface{}{
			"code":    3,
			"message": "Invalid spec.",
		},
	}

	w := &VertexAIOperationWaiter{}
	if err := w.SetOp(op); err != nil {
		t.Fatalf("SetOp failed: %v", err)
	}

	err := w.Error()
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if _, ok := err.(*tpgresource.CommonOpError); !ok {
		t.Fatalf("expected *tpgresource.CommonOpError when details are empty, got %T", err)
	}
	if want := "Error code 3, message: Invalid spec."; err.Error() != want {
		t.Errorf("got %q, want %q", err.Error(), want)
	}
}

func TestUnitVertexAIOperationWaiter_OperationWaitPollsAndIncludesDetails(t *testing.T) {
	t.Parallel()

	opName := "projects/my-proj/locations/us-central1/reasoningEngines/123/operations/456"
	var polls atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Content-Type", "application/json")
		if polls.Add(1) == 1 {
			_ = json.NewEncoder(rw).Encode(map[string]interface{}{
				"name": opName,
				"done": false,
			})
			return
		}
		_ = json.NewEncoder(rw).Encode(map[string]interface{}{
			"name": opName,
			"done": true,
			"error": map[string]interface{}{
				"code":    3,
				"message": "The Reasoning Engine could not be deployed because its container image failed to build.",
				"details": []interface{}{
					map[string]interface{}{
						"@type":  "type.googleapis.com/google.rpc.ErrorInfo",
						"reason": "REASONING_ENGINE_BUILD_FAILED",
						"domain": "aiplatform.googleapis.com",
						"metadata": map[string]interface{}{
							"logs_url": "https://console.cloud.google.com/logs/query?project=my-proj",
						},
					},
					map[string]interface{}{
						"@type": "type.googleapis.com/google.rpc.Help",
						"links": []interface{}{
							map[string]interface{}{
								"description": "Troubleshooting build failures",
								"url":         "https://docs.cloud.google.com/gemini-enterprise-agent-platform/troubleshooting/agent-deployment#build-failed",
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	cfg := &transport_tpg.Config{
		Client: srv.Client(),
		CustomEndpoints: map[string]string{
			Product.CustomEndpointField: srv.URL + "/v1beta1/",
		},
		PollInterval: time.Millisecond,
	}

	w, err := createVertexAIWaiter(
		cfg,
		map[string]interface{}{"name": opName, "done": false},
		"my-proj",
		"Creating ReasoningEngine",
		"test-agent",
	)
	if err != nil {
		t.Fatalf("createVertexAIWaiter failed: %v", err)
	}

	err = tpgresource.OperationWait(w, "Creating ReasoningEngine", 10*time.Second, time.Millisecond)
	if err == nil {
		t.Fatal("expected OperationWait to return an error")
	}

	got := err.Error()
	for _, want := range []string{
		"Error waiting for Creating ReasoningEngine: Error code 3, message: The Reasoning Engine could not be deployed because its container image failed to build.\nDetails:\n[",
		`"reason": "REASONING_ENGINE_BUILD_FAILED"`,
		`"logs_url": "https://console.cloud.google.com/logs/query?project=my-proj"`,
		`"url": "https://docs.cloud.google.com/gemini-enterprise-agent-platform/troubleshooting/agent-deployment#build-failed"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("OperationWait error missing %q\ngot:\n%s", want, got)
		}
	}
}
