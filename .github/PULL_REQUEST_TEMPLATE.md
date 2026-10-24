**What property does this change make true?**

One sentence. The subject line states the property; this is where the detail
goes.

**How it is verified**

The tests that fail without the change (or the gate, for tooling). A gate
change says which promise moved and why, in its own commit.

**Records**

- CHANGELOG entry (English and Chinese) under the open candidate heading, if
  behavior or the public surface changed.
- ROADMAP note for findings and deliberate non-decisions.
- Snapshot diff attached or summarized, if `make update-snapshots` moved
  anything.
