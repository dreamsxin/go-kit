package rest_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/resource"
	"github.com/dreamsxin/go-kit/v2/resource/rest"
)

// --- mounting -----------------------------------------------------------------

// TestACollectionMountedWithoutADecisionIsRefused is the gate that makes the rest
// of this file possible. A surface that answers everybody is a decision, so it is
// spelled out with WithOpenAccess; a surface with neither option is a collection
// nobody decided about, and the failure mode of a permission model is not a
// policy that says no — it is a surface that was never asked.
func TestACollectionMountedWithoutADecisionIsRefused(t *testing.T) {
	_, err := rest.Routes(newCollection(t))
	if err == nil {
		t.Fatal("Routes mounted a collection with no policy and no open-access declaration")
	}
	if code := codeOfError(t, err); code != "rest.access_required" {
		t.Fatalf("code = %q, want rest.access_required", code)
	}
}

// TestAPolicyAndOpenAccessAreAContradiction keeps the two statements exclusive.
// Whichever won, the other would be a line of source that does not describe what
// the process does.
func TestAPolicyAndOpenAccessAreAContradiction(t *testing.T) {
	_, err := rest.Routes(newCollection(t), rest.WithOpenAccess(), rest.WithPolicy(allow(resource.Params{})))
	if err == nil {
		t.Fatal("Routes accepted open access together with a policy")
	}
	if code := codeOfError(t, err); code != "rest.access_conflict" {
		t.Fatalf("code = %q, want rest.access_conflict", code)
	}
}

// --- the decision -------------------------------------------------------------

// TestAnUnfinishedPolicyRefuses pins the zero value. A policy that returns what it
// was never given, a switch that falls through, a branch somebody did not finish —
// all of them are the zero Decision, and all of them have to mean no.
func TestAnUnfinishedPolicyRefuses(t *testing.T) {
	unfinished := func(context.Context, rest.Action) (rest.Decision, error) {
		var decision rest.Decision
		return decision, nil
	}
	handler := mountWith(t, newCollection(t), rest.WithPolicy(unfinished))
	result := do(t, handler, http.MethodGet, "/products", "")
	if result.status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (body %s)", result.status, result.body)
	}
	if code := errorCode(t, result); code != "rest.action_not_allowed" {
		t.Fatalf("code = %q, want rest.action_not_allowed", code)
	}
}

// TestAPolicyThatGrantsOneActionStillRefusesTheOthers is what "whitelist" means:
// six actions, and a policy that mentions one of them has said nothing about the
// other five. Defaulting those to "allowed" would make the policy's silence the
// widest part of the surface.
func TestAPolicyThatGrantsOneActionStillRefusesTheOthers(t *testing.T) {
	handler := mountWith(t, newCollection(t), rest.WithPolicy(
		func(_ context.Context, action rest.Action) (rest.Decision, error) {
			if action == rest.ActionList {
				return rest.Allow(resource.Params{}), nil
			}
			return rest.Decision{}, nil
		}))

	if listed := do(t, handler, http.MethodGet, "/products", ""); listed.status != http.StatusOK {
		t.Fatalf("list status = %d, want 200", listed.status)
	}
	for _, attempt := range []struct {
		method string
		target string
		body   string
	}{
		{http.MethodGet, "/products/1", ""},
		{http.MethodPost, "/products", `{"name":"delta"}`},
		{http.MethodPatch, "/products/1", `{"name":"delta"}`},
		{http.MethodDelete, "/products/1", ""},
		{http.MethodGet, "/products:aggregate?function=count", ""},
	} {
		result := do(t, handler, attempt.method, attempt.target, attempt.body)
		if result.status != http.StatusForbidden {
			t.Fatalf("%s %s status = %d, want 403 (body %s)", attempt.method, attempt.target, result.status, result.body)
		}
	}
}

// TestASecondAuthorityCannotOverruleTheFirst is the reason several policies are a
// conjunction: a tenant scope and a role answer different questions, and letting
// one grant what the other refused would make the narrower authority optional.
func TestASecondAuthorityCannotOverruleTheFirst(t *testing.T) {
	refuse := func(context.Context, rest.Action) (rest.Decision, error) {
		return rest.Deny("this tenant is archived"), nil
	}
	handler := mountWith(t, newCollection(t),
		rest.WithPolicy(refuse, allow(resource.Params{})))

	result := do(t, handler, http.MethodGet, "/products", "")
	if result.status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: one authority's grant cannot undo another's refusal (body %s)", result.status, result.body)
	}
	if reason, _ := result.decoded["message"].(string); !strings.Contains(reason, "archived") {
		t.Fatalf("message = %q, want the refusing policy's reason", reason)
	}
}

// TestAPolicyErrorIsNotARefusal keeps a broken dependency from becoming a 403. A
// refusal is an answer the policy reached; an error says it could not answer, and
// hiding that behind "forbidden" is how an outage spends a day looking like a
// permissions bug.
func TestAPolicyErrorIsNotARefusal(t *testing.T) {
	failing := func(context.Context, rest.Action) (rest.Decision, error) {
		return rest.Decision{}, errors.New("the policy store is unreachable")
	}
	handler := mountWith(t, newCollection(t), rest.WithPolicy(failing))
	result := do(t, handler, http.MethodGet, "/products", "")
	if result.status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (body %s)", result.status, result.body)
	}
	if strings.Contains(string(result.body), "unreachable") {
		t.Fatalf("the 500 body repeats the error: %s", result.body)
	}
}

// --- the constraint reaches the record ---------------------------------------

// TestAConditionSelectsTheRecordNotJustTheList is the whole milestone in one
// test. A condition that only narrowed list would be decorative: the record it
// excludes still has an address, and a client that knows the identifier reaches
// it with one request. Get, update and delete therefore carry the condition to the
// store, in the same statement as the key.
func TestAConditionSelectsTheRecordNotJustTheList(t *testing.T) {
	// The fixture is alpha and beta active, gamma archived; ids follow the seed
	// order.
	onlyActive := allow(resource.Params{Filter: resource.Condition("status", resource.OpEq, "active")})
	collection := newCollection(t)
	handler := mountWith(t, collection, rest.WithPolicy(onlyActive))

	listed := do(t, handler, http.MethodGet, "/products", "")
	if total := listed.decoded["total"]; total != float64(2) {
		t.Fatalf("list total = %v, want the two active rows", total)
	}
	if reached := do(t, handler, http.MethodGet, "/products/1", ""); reached.status != http.StatusOK {
		t.Fatalf("get inside the condition status = %d, want 200 (body %s)", reached.status, reached.body)
	}

	// Archived row 3, the one the list excludes. Every route that can reach a
	// record has to answer as if it did not exist: a 403 would confirm it does.
	outside := []struct {
		method string
		target string
		body   string
	}{
		{http.MethodGet, "/products/3", ""},
		{http.MethodPatch, "/products/3", `{"price":99}`},
		{http.MethodDelete, "/products/3", ""},
	}
	for _, attempt := range outside {
		result := do(t, handler, attempt.method, attempt.target, attempt.body)
		if result.status != http.StatusNotFound {
			t.Fatalf("%s %s status = %d, want 404 (body %s)", attempt.method, attempt.target, result.status, result.body)
		}
		if code := errorCode(t, result); code != "resource.record_not_found" {
			t.Fatalf("%s %s code = %q, want resource.record_not_found", attempt.method, attempt.target, code)
		}
	}

	// And the refusals were refusals rather than writes that happened anyway:
	// the same collection, mounted openly, still holds the archived record as it
	// was seeded.
	mounted := mount(t, collection)
	still := do(t, mounted, http.MethodGet, "/products/3", "")
	if still.status != http.StatusOK {
		t.Fatalf("get outside the condition, mounted openly, status = %d", still.status)
	}
	if still.decoded["price"] != float64(20) {
		t.Fatalf("price = %v, want 20: a patch the condition excluded must not have been applied", still.decoded["price"])
	}
}

// TestACreateOutsideTheConditionIsRefused evaluates the same condition against the
// record being stored. Without it a caller could write a row it would never be
// allowed to read, and the surface would be a write-only hole in the table it
// claims to protect.
func TestACreateOutsideTheConditionIsRefused(t *testing.T) {
	onlyActive := allow(resource.Params{Filter: resource.Condition("status", resource.OpEq, "active")})
	handler := mountWith(t, newCollection(t), rest.WithPolicy(onlyActive))

	inside := do(t, handler, http.MethodPost, "/products", `{"name":"delta","price":40,"status":"active"}`)
	if inside.status != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", inside.status, inside.body)
	}

	outside := do(t, handler, http.MethodPost, "/products", `{"name":"epsilon","price":40,"status":"archived"}`)
	if outside.status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (body %s)", outside.status, outside.body)
	}
	if code := errorCode(t, outside); code != "rest.constraint_unsatisfied" {
		t.Fatalf("code = %q, want rest.constraint_unsatisfied", code)
	}

	// A record that does not carry the field the condition names is unknown
	// rather than acceptable: a column a database default fills is a column the
	// policy cannot check before the write, so the create is refused and the
	// message says the condition could not be evaluated.
	silent := do(t, handler, http.MethodPost, "/products", `{"name":"zeta","price":40}`)
	if silent.status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a record the condition cannot evaluate (body %s)", silent.status, silent.body)
	}
}

// codeOfError reads the classified code from an error returned by Routes.
func codeOfError(t *testing.T, err error) string {
	t.Helper()
	var classified *apperror.Error
	if !errors.As(err, &classified) {
		t.Fatalf("error %v is not classified", err)
	}
	return classified.ErrorCode()
}
