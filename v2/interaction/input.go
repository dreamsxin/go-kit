package interaction

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// InputRequest is one question a tool needs answered before it can finish: a
// confirmation, a missing field, a completion from the caller's model. Kind
// names what is being asked — "elicitation" for a form the caller fills in, or
// an application-defined kind — and Params is what the caller needs to render
// or satisfy it.
type InputRequest struct {
	Kind   string
	Params map[string]any
}

// InputRequired reports that a tool cannot finish without answers. A tool
// returns it in place of an error:
//
//	answers, state := interaction.InputAnswersFromContext(ctx)
//	if _, ok := answers["confirm"]; !ok {
//		return interaction.ToolResult{}, &interaction.InputRequired{
//			Requests: map[string]interaction.InputRequest{
//				"confirm": {Kind: "elicitation", Params: map[string]any{"message": "Delete 42 rows?"}},
//			},
//			State: map[string]any{"rows": 42},
//		}
//	}
//
// It is not a failure: the call is unfinished, and the transport turns it into
// whatever its protocol uses to ask. A tool that asks this way runs again from
// the top when the answers arrive, so it reads as a guard rather than as a
// suspended coroutine — nothing is held open on the server between rounds.
//
// State travels to the caller and back. It is the tool's own to interpret, and
// it arrives having been outside the process: validate it the way any other
// client input is validated. Nothing about a value being in State says the
// server put it there.
type InputRequired struct {
	// Requests are the questions to answer, keyed by an id the tool chooses and
	// reads back out of the answers.
	Requests map[string]InputRequest
	// State is what the tool needs to resume, opaque to the caller.
	State map[string]any
}

// Error names the questions, so a transport that has no way to ask them reports
// something a developer can act on.
func (i *InputRequired) Error() string {
	if i == nil || len(i.Requests) == 0 {
		return "interaction: input required"
	}
	ids := make([]string, 0, len(i.Requests))
	for id := range i.Requests {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return fmt.Sprintf("interaction: input required: %s", strings.Join(ids, ", "))
}

// Kinds reports the request kinds, sorted and deduplicated. A transport uses it
// to check what the caller declared it can answer before asking.
func (i *InputRequired) Kinds() []string {
	if i == nil {
		return nil
	}
	seen := map[string]struct{}{}
	kinds := make([]string, 0, len(i.Requests))
	for _, request := range i.Requests {
		if request.Kind == "" {
			continue
		}
		if _, ok := seen[request.Kind]; ok {
			continue
		}
		seen[request.Kind] = struct{}{}
		kinds = append(kinds, request.Kind)
	}
	sort.Strings(kinds)
	return kinds
}

type inputAnswersContextKey struct{}

type inputAnswers struct {
	answers map[string]any
	state   map[string]any
}

// WithInputAnswers carries the answers to a tool's earlier InputRequired, and
// the state it asked to have echoed back. Transports call it; tools read the
// result with InputAnswersFromContext.
func WithInputAnswers(ctx context.Context, answers, state map[string]any) context.Context {
	if len(answers) == 0 && len(state) == 0 {
		return ctx
	}
	return context.WithValue(ctx, inputAnswersContextKey{}, inputAnswers{answers: answers, state: state})
}

// InputAnswersFromContext returns the answers and state a resumed call carries,
// keyed by the request ids the tool chose. Both are empty on a first call.
func InputAnswersFromContext(ctx context.Context) (answers, state map[string]any) {
	carried, ok := ctx.Value(inputAnswersContextKey{}).(inputAnswers)
	if !ok {
		return nil, nil
	}
	return carried.answers, carried.state
}
