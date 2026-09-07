package interaction_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dreamsxin/go-kit/v2/interaction"
)

// TestCallToolInputRequiredIsNotAFailure proves an unfinished call is reported as
// its own outcome. A tool that asks the caller a question has not failed, and an
// error event would put a working conversation in front of whoever watches them.
func TestCallToolInputRequiredIsNotAFailure(t *testing.T) {
	events := interaction.NewMemoryEventSink()
	rt := interaction.NewRuntime().WithEvents(events)
	if err := rt.RegisterTool(interaction.ToolFunc{
		ToolName: "ask",
		Fn: func(ctx context.Context, call interaction.ToolCall) (interaction.ToolResult, error) {
			answers, state := interaction.InputAnswersFromContext(ctx)
			if answers["confirm"] != true {
				return interaction.ToolResult{}, &interaction.InputRequired{
					Requests: map[string]interaction.InputRequest{
						"confirm": {Kind: "elicitation", Params: map[string]any{"message": "sure?"}},
					},
					State: map[string]any{"attempt": 1},
				}
			}
			if state["attempt"] != 1 {
				return interaction.ToolResult{}, errors.New("resumed without the state the first round recorded")
			}
			return interaction.ToolResult{Output: "done"}, nil
		},
	}); err != nil {
		t.Fatalf("RegisterTool: %v", err)
	}

	ctx := context.Background()
	session, err := rt.StartSession(ctx, "subject", nil)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	call := interaction.ToolCall{SessionID: session.ID, Name: "ask"}

	_, err = rt.CallTool(ctx, call)
	var needsInput *interaction.InputRequired
	if !errors.As(err, &needsInput) {
		t.Fatalf("CallTool error = %v, want an InputRequired", err)
	}
	if kinds := needsInput.Kinds(); len(kinds) != 1 || kinds[0] != "elicitation" {
		t.Errorf("Kinds() = %v, want [elicitation]", kinds)
	}

	recorded, err := events.List(ctx, session.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var sawInputRequired bool
	for _, event := range recorded {
		switch event.Type {
		case interaction.EventToolInputRequired:
			sawInputRequired = true
		case interaction.EventError:
			t.Errorf("an unfinished call emitted an error event: %+v", event)
		case interaction.EventToolResult:
			t.Errorf("an unfinished call emitted a result event: %+v", event)
		}
	}
	if !sawInputRequired {
		t.Errorf("no %s event was emitted: %+v", interaction.EventToolInputRequired, recorded)
	}

	// The same call, resumed with the answers, finishes.
	resumed := interaction.WithInputAnswers(ctx,
		map[string]any{"confirm": true}, map[string]any{"attempt": 1})
	result, err := rt.CallTool(resumed, call)
	if err != nil {
		t.Fatalf("resumed CallTool: %v", err)
	}
	if result.Output != "done" {
		t.Errorf("resumed output = %v, want done", result.Output)
	}
}
