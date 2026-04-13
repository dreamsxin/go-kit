# PR Checklist

Use this checklist when reviewing or preparing pull requests for `go-kit`.

The goal is to make framework scope, compatibility, and validation checks routine rather than ad hoc.

## 1. Scope Check

- Does this change belong in a microservice framework rather than business/application logic?
- Does it strengthen layering, runtime governance, generation, or documentation?
- Does it avoid pulling the framework toward platform concerns that should live elsewhere?

If the answer is unclear, review:

- [FRAMEWORK_BOUNDARIES.md](FRAMEWORK_BOUNDARIES.md)
- [ANTI_PATTERNS.md](ANTI_PATTERNS.md)

## 2. Layering Check

- Does business logic remain out of `transport/`?
- Does runtime policy remain primarily in `endpoint/`?
- Does the change preserve the service -> endpoint -> transport model?
- Does it avoid transport-aware service interfaces?

If not, the design should usually be reconsidered.

## 3. Stability Check

- Which surface is being changed: stable, semi-stable, or internal?
- If stable, were docs/examples/tests updated together?
- If semi-stable, was the change kept additive where possible?
- If internal, did the change avoid accidentally creating a new public contract?

Reference:

- [STABILITY.md](STABILITY.md)
- [PACKAGE_SURFACES.md](PACKAGE_SURFACES.md)

## 4. Extension Check

- Is this solving a reusable framework problem rather than a one-off app need?
- Could this be implemented through an existing extension point instead of a new core abstraction?
- If a new extension point is added, is it documented clearly?

Reference:

- [FRAMEWORK_BOUNDARIES.md](FRAMEWORK_BOUNDARIES.md)
- [PACKAGE_SURFACES.md](PACKAGE_SURFACES.md)

## 5. microgen Check

Run this section whenever the PR touches `cmd/microgen`, templates, generated layout, or generator docs.

- Does the change alter a documented CLI flag?
- Does it alter the meaning of generated top-level directories or major files?
- Does it change generated layering or skill output behavior?
- If user-visible output changed, was it treated as a product change rather than an internal refactor?
- Were migration notes added if existing users may be affected?

Reference:

- [MICROGEN_COMPATIBILITY.md](MICROGEN_COMPATIBILITY.md)

## 6. Documentation Check

- Are README, examples, and workflow docs still aligned?
- If public behavior changed, were the relevant docs updated in the same PR?
- If examples changed, do they still demonstrate recommended patterns instead of anti-patterns?

Key docs:

- [README.md](README.md)
- [PROJECT_WORKFLOW.md](PROJECT_WORKFLOW.md)
- [FRAMEWORK_BOUNDARIES.md](FRAMEWORK_BOUNDARIES.md)

## 7. Validation Check

Choose the smallest sufficient validation first, then broaden if needed.

Common targets:

- `make test-runtime`
- `make test-microgen`
- `make test-docs`
- `make test-examples`
- `go test -race ./...`

Ask:

- Did the tests match the type of change?
- If generator output changed, were integration tests included?
- If docs or skill guidance changed, were docs-backed tests included?

## 8. Reviewer Notes

When leaving review comments, prefer comments like:

- "This looks like business logic moving into transport."
- "This seems to promote an internal generator detail into public contract."
- "This may change a stable surface; can we update docs and examples in the same PR?"
- "Can this be expressed as an extension point instead of a new core branch?"

These comments make review more consistent with the framework's documented intent.

## Fast Merge Gate

Before merge, reviewers should be able to answer "yes" to all of the following:

- The change fits framework scope.
- The layering model is preserved.
- Public/internal boundaries remain clear.
- Compatibility impact is understood.
- Validation is appropriate for the risk level.
- Documentation is updated if user-visible behavior changed.
