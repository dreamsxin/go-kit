package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dreamsxin/go-kit/v2/interaction"
)

// Multi round-trip requests: a tool that cannot finish without an answer returns
// the question, and the caller repeats the call with the answer attached. The
// state that travels with it is the part worth being careful about — it leaves
// the process and comes back.

// askingTool needs one answer before it deletes anything, which is the shape
// every mid-call question has: run, check what arrived, ask or proceed.
func askingTool(seen *map[string]any) interaction.ToolFunc {
	return interaction.ToolFunc{
		ToolName: "delete_rows",
		Fn: func(ctx context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
			answers, state := interaction.InputAnswersFromContext(ctx)
			*seen = state
			if answers["confirm"] != true {
				return interaction.ToolResult{}, &interaction.InputRequired{
					Requests: map[string]interaction.InputRequest{
						"confirm": {Kind: "elicitation", Params: map[string]any{"message": "Delete 42 rows?"}},
					},
					State: map[string]any{"rows": 42},
				}
			}
			return interaction.ToolResult{Output: "deleted"}, nil
		},
	}
}

// handlerWithAskingTool returns a handler whose only tool asks a question, and
// the state the tool last saw.
func handlerWithAskingTool(t *testing.T) (*StreamableHandler, *map[string]any) {
	t.Helper()
	seen := new(map[string]any)
	rt := interaction.NewRuntime()
	if err := rt.RegisterTool(askingTool(seen)); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}
	h := NewStreamableHandler(rt)
	t.Cleanup(func() { _ = h.Close() })
	return h, seen
}

// elicitingClient declares the capability a question of that kind needs.
var elicitingClient = map[string]any{
	metaClientCapabilities: map[string]any{"elicitation": map[string]any{}},
}

// TestStatelessToolAsksForInputAndResumes proves a tool can ask the caller
// something without a stream to ask over: the question is a result, and the
// answer arrives as the next call's input.
func TestStatelessToolAsksForInputAndResumes(t *testing.T) {
	h, seenState := handlerWithAskingTool(t)

	_, resp := statelessRequest(t, h, "tools/call", "delete_rows", statelessCall("tools/call", map[string]any{
		"name":  "delete_rows",
		"_meta": elicitingClient,
	}))
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("first call answered %v, want an interim result", resp)
	}
	if result["resultType"] != "input_required" {
		t.Fatalf("resultType = %v, want input_required", result["resultType"])
	}
	requests, _ := result["inputRequests"].(map[string]any)
	confirm, _ := requests["confirm"].(map[string]any)
	if confirm["kind"] != "elicitation" {
		t.Errorf("inputRequests.confirm = %v, want an elicitation", requests["confirm"])
	}
	state, _ := result["requestState"].(string)
	if state == "" {
		t.Fatal("the interim result carries no requestState, so the tool cannot resume")
	}

	_, resp = statelessRequest(t, h, "tools/call", "delete_rows", statelessCall("tools/call", map[string]any{
		"name":           "delete_rows",
		"_meta":          elicitingClient,
		"inputResponses": map[string]any{"confirm": true},
		"requestState":   state,
	}))
	result, ok = resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("the resumed call answered %v, want a result", resp)
	}
	if result["resultType"] != "complete" {
		t.Errorf("resumed resultType = %v, want complete", result["resultType"])
	}
	if result["isError"] != false {
		t.Errorf("resumed result = %v, want a successful call", result)
	}
	if rows, _ := (*seenState)["rows"].(float64); rows != 42 {
		t.Errorf("the tool resumed with state %v, want the 42 rows it recorded", *seenState)
	}
}

// TestStatelessInputRequiresADeclaredCapability proves the server does not ask a
// question the caller never said it could answer, which would hang a client that
// has no code for it.
func TestStatelessInputRequiresADeclaredCapability(t *testing.T) {
	h, _ := handlerWithAskingTool(t)

	_, resp := statelessRequest(t, h, "tools/call", "delete_rows", statelessCall("tools/call", map[string]any{
		"name": "delete_rows",
	}))
	rpcErr, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("a question the client cannot answer produced %v, want an error", resp)
	}
	if code, _ := rpcErr["code"].(float64); code != -32021 {
		t.Errorf("error code = %v, want -32021", rpcErr["code"])
	}
	data, _ := rpcErr["data"].(map[string]any)
	required, _ := data["requiredCapabilities"].([]any)
	if len(required) != 1 || required[0] != "elicitation" {
		t.Errorf("data.requiredCapabilities = %v, want [elicitation]", data["requiredCapabilities"])
	}
}

// TestLegacyPathRefusesAnInputRequiredResult proves the interim result is not
// emitted where it does not exist: a 2025-06-18 client would not know to answer
// it, and its own way to ask is the session stream.
func TestLegacyPathRefusesAnInputRequiredResult(t *testing.T) {
	h, _ := handlerWithAskingTool(t)

	sid := initSessionHelper(t, h)
	_, resp := streamPostJSON(t, h, sid, "tools/call", map[string]any{"name": "delete_rows"})
	rpcErr, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("the handshake revision answered %v, want an error", resp)
	}
	if code, _ := rpcErr["code"].(float64); code != -32603 {
		t.Errorf("error code = %v, want -32603", rpcErr["code"])
	}
	if data, _ := rpcErr["data"].(string); !contains(data, protocolVersion) {
		t.Errorf("error data = %q, want it to name %s", data, protocolVersion)
	}
}

// TestRequestStateIsAuthenticated covers the codec that seals what a tool asks to
// have echoed. With a key configured the state has to come back the way it left:
// a tool reading its own earlier decision out of it would otherwise be reading
// whatever the caller chose to send.
func TestRequestStateIsAuthenticated(t *testing.T) {
	t.Parallel()
	codec := requestStateCodec{key: []byte("a-shared-signing-key"), ttl: time.Minute}
	now := time.Unix(1_800_000_000, 0)
	state := map[string]any{"rows": float64(42), "approved": true}

	sealed, err := codec.seal(state, now)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	opened, err := codec.open(sealed, now.Add(time.Second))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if opened["rows"] != state["rows"] || opened["approved"] != state["approved"] {
		t.Fatalf("opened = %v, want %v", opened, state)
	}

	edited, err := codec.seal(map[string]any{"rows": float64(42), "approved": false}, now)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	forged := strings.SplitN(edited, ".", 3)[1] // the payload the caller would swap in
	sealedParts := strings.SplitN(sealed, ".", 3)

	for _, tc := range []struct {
		name  string
		state string
		at    time.Time
		want  string
	}{
		{
			name:  "an edited payload keeps the old signature",
			state: sealedParts[0] + "." + forged + "." + sealedParts[2],
			at:    now,
			want:  "signature does not verify",
		},
		{
			name:  "dropping the signature is not a way out",
			state: sealedParts[0] + "." + sealedParts[1],
			at:    now,
			want:  "carries no signature",
		},
		{
			name:  "a state older than the TTL cannot be replayed",
			state: sealed,
			at:    now.Add(2 * time.Minute),
			want:  "past the 1m0s it may be replayed for",
		},
		{
			name:  "a state from the future is refused as well",
			state: sealed,
			at:    now.Add(-10 * time.Minute),
			want:  "past the 1m0s it may be replayed for",
		},
		{
			name:  "an unknown envelope version is refused",
			state: "v2." + sealedParts[1] + "." + sealedParts[2],
			at:    now,
			want:  "not a v1 envelope",
		},
		{
			name:  "a payload that is not base64 is refused",
			state: sealedParts[0] + ".!!!." + sealedParts[2],
			at:    now,
			want:  "signature does not verify",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := codec.open(tc.state, tc.at); err == nil {
				t.Fatalf("open accepted %q", tc.state)
			} else if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("open error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

// TestRequestStateWithoutAKeyIsAcceptedAsReturned pins the documented default: a
// transport that configured no key gets no authentication, and a tool must treat
// the state as the client input it is. The gate here is that the behaviour is
// deliberate rather than an accident of the envelope format.
func TestRequestStateWithoutAKeyIsAcceptedAsReturned(t *testing.T) {
	t.Parallel()
	unsigned := requestStateCodec{}
	now := time.Unix(1_800_000_000, 0)

	sealed, err := unsigned.seal(map[string]any{"rows": float64(1)}, now)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if strings.Count(sealed, ".") != 1 {
		t.Fatalf("sealed = %q, want no signature segment", sealed)
	}

	// Anything the caller writes into the payload arrives as written, including
	// long after it was issued.
	forged, err := json.Marshal(requestStateEnvelope{IssuedAt: 1, State: map[string]any{"approved": true}})
	if err != nil {
		t.Fatal(err)
	}
	opened, err := unsigned.open(requestStateVersion+"."+base64.RawURLEncoding.EncodeToString(forged), now)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if opened["approved"] != true {
		t.Fatalf("opened = %v, want the caller's own state", opened)
	}

	// The same state is refused once a key is configured, which is the whole
	// difference between the two modes.
	if _, err := (requestStateCodec{key: []byte("k")}).open(sealed, now); err == nil {
		t.Fatal("a configured key accepted an unsigned state")
	}
}

// TestSignedRequestStateSurvivesTheRoundTrip proves the handler seals and opens
// with the key it was configured with, and refuses an edited one over the wire
// rather than only in the codec's own test.
func TestSignedRequestStateSurvivesTheRoundTrip(t *testing.T) {
	h, seenState := handlerWithAskingTool(t)
	h.RequestStateKey = []byte("a-shared-signing-key")

	_, resp := statelessRequest(t, h, "tools/call", "delete_rows", statelessCall("tools/call", map[string]any{
		"name":  "delete_rows",
		"_meta": elicitingClient,
	}))
	result, _ := resp["result"].(map[string]any)
	state, _ := result["requestState"].(string)
	if strings.Count(state, ".") != 2 {
		t.Fatalf("requestState = %q, want a signed envelope", state)
	}

	_, resp = statelessRequest(t, h, "tools/call", "delete_rows", statelessCall("tools/call", map[string]any{
		"name":           "delete_rows",
		"_meta":          elicitingClient,
		"inputResponses": map[string]any{"confirm": true},
		"requestState":   state,
	}))
	if _, ok := resp["result"].(map[string]any); !ok {
		t.Fatalf("the resumed call answered %v, want a result", resp)
	}
	if rows, _ := (*seenState)["rows"].(float64); rows != 42 {
		t.Errorf("the tool resumed with state %v, want the 42 rows it recorded", *seenState)
	}

	tampered := strings.SplitN(state, ".", 3)
	tampered[1] = base64.RawURLEncoding.EncodeToString([]byte(`{"iat":1,"state":{"rows":9999}}`))
	rec, resp := statelessRequest(t, h, "tools/call", "delete_rows", statelessCall("tools/call", map[string]any{
		"name":           "delete_rows",
		"_meta":          elicitingClient,
		"inputResponses": map[string]any{"confirm": true},
		"requestState":   strings.Join(tampered, "."),
	}))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want a JSON-RPC error inside a 200", rec.Code)
	}
	rpcErr, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("an edited requestState answered %v, want an error", resp)
	}
	if data, _ := rpcErr["data"].(string); !strings.Contains(data, "signature does not verify") {
		t.Fatalf("error data = %q, want it to name the signature", data)
	}
}
