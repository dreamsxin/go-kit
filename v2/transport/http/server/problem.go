package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/dreamsxin/go-kit/v2/endpoint"
	transporthttp "github.com/dreamsxin/go-kit/v2/transport/http"
)

// ProblemMediaType is the media type RFC 9457 registers for a problem
// document. It carries no charset parameter: the RFC does not define one and
// JSON is UTF-8 by definition.
//
// Stable: http.problem-media-type — a problem document is written as application/problem+json, with no charset parameter.
// Covered by: TestProblemJSONErrorEncoderWritesTheRegisteredMediaType
const ProblemMediaType = "application/problem+json"

// ProblemTypeAboutBlank is the "type" RFC 9457 prescribes when a problem has no
// URI of its own, and what this package writes when no resolver supplies one.
const ProblemTypeAboutBlank = "about:blank"

// ProblemTypeResolver maps the machine-readable error code to the "type" URI of
// the problem document. Returning an empty string means "no type of its own",
// which is written as ProblemTypeAboutBlank.
//
// The URI space belongs to the deployment — it is the identifier of a document
// somebody has to publish and keep resolvable — so the framework does not
// invent one:
//
//	server.ProblemJSONErrorEncoder(func(code string) string {
//	    return "https://errors.example.com/" + code
//	})
type ProblemTypeResolver func(code string) string

// ProblemFieldError is one entry of the "errors" extension member, carrying the
// field-level detail that ErrorResponse has nowhere to put.
type ProblemFieldError struct {
	Field  string `json:"field"`
	Detail string `json:"detail"`
}

// ProblemDetails is an RFC 9457 problem document.
//
// Type, Title and Status are always populated so the document is valid on its
// own. Code and RequestID are extension members that keep what the ErrorResponse
// envelope carried, so moving to problem+json does not cost a client the
// machine-readable code it already switches on. Errors is the extension this
// exists for: endpoint.ValidationError carries a field list that the envelope
// flattens into one sentence.
//
// Stable: http.problem-document-shape — a problem document always has type, title and status, keeps the envelope's code and request_id, and adds errors for a validation failure.
// Covered by: TestProblemFromErrorAlwaysHasTypeTitleAndStatus, TestProblemFromErrorKeepsTheEnvelopeCode
type ProblemDetails struct {
	Type      string              `json:"type"`
	Title     string              `json:"title"`
	Status    int                 `json:"status"`
	Detail    string              `json:"detail,omitempty"`
	Code      string              `json:"code"`
	RequestID string              `json:"request_id,omitempty"`
	Errors    []ProblemFieldError `json:"errors,omitempty"`
}

// ProblemFromError builds the problem document the built-in encoder would write
// for err. It reads the same contracts as JSONErrorEncoder — StatusCoder,
// ErrorCoder, PublicMessager, apperror kinds — and obeys the same redaction
// rule, so a 500 says no more here than it does there. Response headers are the
// encoder's job, not this function's.
//
// Export exists for the encoder a deployment writes itself: build the document,
// adjust it, and hand it to WriteProblemJSON. That is also how a custom kind
// mapper composes, since Status is an ordinary field.
//
// Stable: http.problem-redaction — a problem document's detail obeys the same redaction as the envelope's message, so a 500 carries no error text.
// Covered by: TestProblemFromErrorRedactsAt500
func ProblemFromError(ctx context.Context, err error, resolveType ProblemTypeResolver) ProblemDetails {
	status := httpStatus(err)
	code := errorCodeFor(err, status)

	problem := ProblemDetails{
		Type:      ProblemTypeAboutBlank,
		Title:     problemTitle(status),
		Status:    status,
		Code:      code,
		RequestID: endpoint.RequestIDFromContext(ctx),
	}
	if resolveType != nil {
		if uri := resolveType(code); uri != "" {
			problem.Type = uri
		}
	}
	// Detail repeats Title for every error that has nothing of its own to say,
	// a redacted 500 above all. Leaving it out then is what makes its presence
	// mean something.
	if detail := publicErrorMessage(err, status); detail != problem.Title {
		problem.Detail = detail
	}
	var verr *endpoint.ValidationError
	if errors.As(err, &verr) && len(verr.Fields) > 0 {
		problem.Errors = make([]ProblemFieldError, 0, len(verr.Fields))
		for _, field := range verr.Fields {
			problem.Errors = append(problem.Errors, ProblemFieldError{
				Field:  field.Field,
				Detail: field.Reason,
			})
		}
	}
	return problem
}

// WriteProblemJSON writes a problem document with the registered media type and
// the document's own status. Headers already set on w are left alone.
func WriteProblemJSON(w http.ResponseWriter, problem ProblemDetails) {
	w.Header().Set("Content-Type", ProblemMediaType)
	status := problem.Status
	if status < 100 || status > 999 {
		status = http.StatusInternalServerError
		problem.Status = status
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem)
}

// ProblemJSONErrorEncoder returns an ErrorEncoder that writes RFC 9457
// problem documents instead of the ErrorResponse envelope.
//
// It is offered rather than adopted. ErrorResponse is declared stable and is
// what every service built on this framework already emits; which error format
// a service speaks is a decision its clients live with, so it is a seam, not
// framework policy. Install it where the interoperability is worth the change:
//
//	server.NewJSONServer[Req](handler,
//	    server.ServerErrorEncoder(server.ProblemJSONErrorEncoder(nil)),
//	)
//
// Or component-wide:
//
//	kit.NewHTTP(kit.WithJSONServerOptions(
//	    httpserver.ServerErrorEncoder(httpserver.ProblemJSONErrorEncoder(nil)),
//	))
//
// The status mapping, the redaction rule, Headerer and Retry-After behave
// exactly as they do in JSONErrorEncoder. The gain over the envelope is the
// field list on a validation failure and a media type a generic client can
// recognize; the cost is a body shape existing clients do not parse.
//
// Stable: http.problem-encoder-parity — a problem document uses the same status, headers, Retry-After and code as the JSON envelope for the same error.
// Covered by: TestProblemJSONErrorEncoderMatchesTheEnvelopeStatusAndCode, TestProblemJSONErrorEncoderEmitsRetryAfter
func ProblemJSONErrorEncoder(resolveType ProblemTypeResolver) ErrorEncoder {
	return func(ctx context.Context, err error, w http.ResponseWriter) {
		var h transporthttp.Headerer
		if errors.As(err, &h) {
			for key, values := range h.Headers() {
				for _, value := range values {
					w.Header().Add(key, value)
				}
			}
		}
		applyRetryAfter(w, err)
		WriteProblemJSON(w, ProblemFromError(ctx, err, resolveType))
	}
}

// problemTitle is the human-readable summary of the status. It is the status
// phrase rather than the error text: RFC 9457 says the title describes the
// problem type, not this occurrence, and the occurrence is what Detail is for.
func problemTitle(status int) string {
	if text := statusText(status); text != "" {
		return text
	}
	return "HTTP error"
}
