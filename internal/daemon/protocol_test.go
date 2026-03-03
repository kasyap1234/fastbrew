package daemon

import (
	"encoding/json"
	"testing"
)

func TestRequest_Serialization(t *testing.T) {
	req := Request{
		Type:    RequestSearch,
		Payload: json.RawMessage(`{"query":"wget"}`),
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var req2 Request
	if err := json.Unmarshal(data, &req2); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if req2.Type != req.Type {
		t.Errorf("Expected type %s, got %s", req.Type, req2.Type)
	}

	var payload SearchRequest
	if err := json.Unmarshal(req2.Payload, &payload); err != nil {
		t.Fatalf("Payload unmarshal failed: %v", err)
	}

	if payload.Query != "wget" {
		t.Errorf("Expected query \"wget\", got %q", payload.Query)
	}
}

func TestResponse_Serialization(t *testing.T) {
	resp := Response{
		OK:      true,
		Payload: json.RawMessage(`{"job_id":"123"}`),
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var resp2 Response
	if err := json.Unmarshal(data, &resp2); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if !resp2.OK {
		t.Error("Expected OK to be true")
	}

	var payload JobSubmitResponse
	if err := json.Unmarshal(resp2.Payload, &payload); err != nil {
		t.Fatalf("Payload unmarshal failed: %v", err)
	}

	if payload.JobID != "123" {
		t.Errorf("Expected job_id \"123\", got %q", payload.JobID)
	}
}

func TestResponse_ErrorSerialization(t *testing.T) {
	resp := Response{
		OK:    false,
		Code:  ResponseCodeErr,
		Error: "something went wrong",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var resp2 Response
	if err := json.Unmarshal(data, &resp2); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if resp2.OK {
		t.Error("Expected OK to be false")
	}
	if resp2.Code != ResponseCodeErr {
		t.Errorf("Expected code %s, got %s", ResponseCodeErr, resp2.Code)
	}
	if resp2.Error != "something went wrong" {
		t.Errorf("Expected error message, got %q", resp2.Error)
	}
}
