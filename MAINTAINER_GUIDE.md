# Maintainer Guide

Active development and releases target the independent v2 module under
[`v2/`](v2/). Use these authoritative documents:

1. [v2/MAINTAINING.md](v2/MAINTAINING.md) for development and validation.
2. [v2/ROADMAP.md](v2/ROADMAP.md) for implementation scope and sequencing.
3. [v2/RELEASE.md](v2/RELEASE.md) for release gates and tag conventions.
4. [v2/DOCS_INDEX.md](v2/DOCS_INDEX.md) for the complete documentation map.

The root module is the v1 maintenance line. Make changes there only when a v1
fix is explicitly required, and validate them against the root package tests
and legacy compatibility documents.

Do not create session snapshots or parallel workflow/roadmap documents in the
repository root. Update the owning v2 document or use an issue/pull request for
temporary planning.
