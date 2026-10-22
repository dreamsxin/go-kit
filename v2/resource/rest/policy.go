package rest

import (
	"context"
	"fmt"
	"net/http"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/resource"
)

// Decision is one policy's answer for one action: granted under constraints, or
// refused for a reason.
//
// The zero value refuses. That is the whole reason this is a struct with no
// exported field rather than a bool: a policy somebody did not finish, a switch
// that falls through, a value that was never filled in — every one of those is
// silence, and silence has to mean no. A grant is a statement a policy makes on
// purpose, and it has to carry the constraints that make it true, because
// "allowed, with no restriction" is a different claim from "allowed to see your
// own rows" and only one of them is usually meant.
//
// Stable: rest.decision-zero-refuses — an action proceeds only when a policy returned rest.Allow for it; the zero Decision and a policy that was never installed both refuse.
// Covered by: TestAnUnfinishedPolicyRefuses, TestAPolicyThatGrantsOneActionStillRefusesTheOthers
type Decision struct {
	allowed bool
	params  resource.Params
	reason  string
}

// Allow grants the action, narrowed by params.
//
// The params are merged like every other layer, so a grant cannot widen what the
// resource's defaults or the client's own request already narrowed: filters
// conjoin, projections intersect, a page size only shrinks. An empty Params is a
// grant with no restriction, which is what a policy means when it grants on the
// strength of the context alone.
func Allow(params resource.Params) Decision {
	return Decision{allowed: true, params: params}
}

// Deny refuses the action, for the reason given.
//
// The reason reaches the client in the 403 body, so it is written for the caller:
// say which capability is missing, not how the policy decides. A reason left
// empty becomes a generic sentence rather than an empty message, because a
// refusal that explains nothing is indistinguishable from a bug.
func Deny(reason string) Decision {
	return Decision{reason: reason}
}

// Allowed reports whether the decision grants the action.
func (d Decision) Allowed() bool { return d.allowed }

// Params returns the constraints a grant carries. A refusal carries none.
func (d Decision) Params() resource.Params {
	if !d.allowed {
		return resource.Params{}
	}
	return d.params
}

// Reason returns why the action was refused, or "" for a grant.
func (d Decision) Reason() string {
	if d.allowed {
		return ""
	}
	return d.reason
}

// Policy decides one action on one request.
//
// It is a function rather than an interface because a policy is one decision:
// there is no lifecycle to own and no state to carry beyond what the request's
// context already holds. A policy reads the subject from the context — the
// application's authentication middleware put it there — and answers.
//
// An error is not a refusal. A refusal is an answer the policy reached; an error
// says the policy could not answer, so it stops the request as the error it is
// instead of becoming a 403 that hides a broken dependency.
type Policy func(ctx context.Context, action Action) (Decision, error)

// WithPolicy installs the authorities that decide whether an action may run.
//
// Installing a policy closes the surface: an action that no policy grants is
// refused, so the set of what a client may do is the set the policies allow
// rather than everything minus what someone remembered to forbid. Several
// policies are several authorities — a tenant scope and a role, say — and every
// one of them must allow, because a union would let one authority overrule
// another authority's refusal.
//
// The constraints of every grant are merged after the resource's defaults and the
// client's request, by resource.MergeParams, which only narrows.
func WithPolicy(policies ...Policy) Option {
	return func(s *settings) {
		for _, policy := range policies {
			if policy != nil {
				s.policies = append(s.policies, policy)
			}
		}
	}
}

// WithOpenAccess declares that a collection answers every request, which is the
// only way to mount one without a policy.
//
// It exists so that "anybody may do anything here" is a sentence in the
// application rather than the absence of a sentence: a surface that answers
// everyone is a decision, and a decision is greppable, reviewable, and visible in
// a diff. A collection mounted with neither option is refused by Routes,
// because a default that grants everything is a default nobody chose.
func WithOpenAccess() Option {
	return func(s *settings) { s.openAccess = true }
}

// decide runs every policy for one action and returns the merged parameters.
//
// The order is what makes the merge safe: the resource's defaults, then what the
// client asked for, then the policies. Each step narrows, so a client can
// override a default sort and cannot override a policy's condition — and a
// policy's ordering decision is the one that stands.
func (b *binding) decide(ctx context.Context, r *http.Request, action Action) (resource.Params, error) {
	client, err := clientParams(r)
	if err != nil {
		return resource.Params{}, err
	}
	layers := make([]resource.Params, 0, len(b.settings.policies)+2)
	layers = append(layers, b.defaults, client)
	for _, policy := range b.settings.policies {
		decision, err := policy(ctx, action)
		if err != nil {
			return resource.Params{}, err
		}
		if !decision.Allowed() {
			return resource.Params{}, refusal(action, decision.Reason())
		}
		layers = append(layers, decision.Params())
	}
	merged, err := resource.MergeParams(layers...)
	if err != nil {
		return resource.Params{}, err
	}
	if err := merged.Validate(b.collection.Schema); err != nil {
		return resource.Params{}, err
	}
	return merged, nil
}

// refusal is the error a denied action answers with.
//
// It is a 403 and not a 404: the caller named an action on a collection it can
// see, and refusing to say so would turn every typo in a policy into a mystery.
// A record outside a constraint is a different case, and that one is NotFound —
// there the existence of a row is the secret.
func refusal(action Action, reason string) error {
	if reason == "" {
		reason = "no policy granted this action"
	}
	return apperror.PermissionDenied("rest.action_not_allowed",
		fmt.Sprintf("%s was refused: %s", action, reason))
}
