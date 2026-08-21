package tools

import (
	"encoding/json"
	"testing"

	"github.com/Salvia95/graphin/internal/mcp"
)

// TestInputSchemasMarshalValid checks every registered tool at the shape a
// client actually receives, rather than checking objSchema.
//
// The distinction matters because the failure is not local. An InputSchema is
// a plain map that any registration may build by hand, and a client that
// validates it rejects the *entire* tools/list response when one entry is
// malformed — so a single bad schema empties the tool table for the whole
// session. Nothing in our logs says so; the server answered, and it answered
// what it meant to. The only place that is visible is here.
//
// v0.3.0 and v0.4.0 both shipped `"properties": null` on the one tool that
// takes no arguments, and every graphin tool was unreachable for the life of
// both releases.
func TestInputSchemasMarshalValid(t *testing.T) {
	reg := mcp.NewRegistry()
	// Handlers close over the workspace but none of them run here, and
	// registration itself never touches it.
	Register(reg, nil)

	tools := reg.List()
	if len(tools) == 0 {
		t.Fatal("no tools registered: the loop below would pass vacuously")
	}

	for _, tool := range tools {
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Errorf("%s: input schema does not marshal: %v", tool.Name, err)
			continue
		}
		var got struct {
			Type       string          `json:"type"`
			Properties json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Errorf("%s: input schema does not round-trip: %v", tool.Name, err)
			continue
		}
		if got.Type != "object" {
			t.Errorf("%s: type = %q, want %q", tool.Name, got.Type, "object")
		}
		// Omitting properties is valid JSON Schema and stays allowed. Emitting
		// it as null is what breaks the client.
		if string(got.Properties) == "null" {
			t.Errorf("%s: properties serialized as JSON null — a validating "+
				"client drops the whole tools/list, taking every other tool "+
				"with it", tool.Name)
		}
	}
}

// TestObjSchemaNilProps pins the direct cause. The registry test above would
// catch a regression here too, but only by way of whichever tool happens to
// pass nil that day; this one fails whether or not such a tool exists.
func TestObjSchemaNilProps(t *testing.T) {
	raw, err := json.Marshal(objSchema(nil, nil))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if want := `{"properties":{},"type":"object"}`; string(raw) != want {
		t.Errorf("objSchema(nil, nil) = %s, want %s", raw, want)
	}
}
