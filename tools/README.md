# tools/

Development and testing utilities for the go-kit framework.

## Files

| File | Purpose |
|------|---------|
| `filewriter.py` | AI-assisted file writer — safely write/patch files on Windows |
| `test_microgen.py` | End-to-end tests for the `microgen` code generator |
| `test_framework.py` | Validates framework API additions (Builder, NewJSONServer, etc.) |
| `test_examples.py` | Compiles and smoke-tests all `examples/` directories |
| `SKILL.md` | AI skill file — teaches an AI assistant how to use this framework |

---

## filewriter.py

Solves the problem of writing large files on Windows where shell heredocs are unavailable.

```bash
# Write a file from inline text
python tools/filewriter.py write path/to/file.go --text "package main"

# Write from a Python content file (for large files)
python tools/filewriter.py write path/to/file.go --content-file my_content.py

# Append to an existing file
python tools/filewriter.py append path/to/file.go --text "// added"

# Search-and-replace patch
python tools/filewriter.py patch path/to/file.go --old "foo := 1" --new "foo := 2"

# Multiple patches from JSON
python tools/filewriter.py patch path/to/file.go --patch-file patches.json

# Read with line numbers
python tools/filewriter.py read path/to/file.go --start 10 --end 20

# Verify content
python tools/filewriter.py check path/to/file.go --contains "package main" --not-contains "TODO"
```

**Content file format** (`my_content.py`):
```python
content = r'''
package main
// ... file content here
'''
```

---

## test_microgen.py

End-to-end integration tests for the `microgen` code generator.
Covers 25 test cases including IDL mode, DB mode, CLI validation, and runtime smoke tests.

```bash
python tools/test_microgen.py                    # run all 25 cases
python tools/test_microgen.py --no-runtime       # skip HTTP smoke tests (CI)
python tools/test_microgen.py -k db              # filter by name
python tools/test_microgen.py -k runtime -v      # verbose runtime tests
python tools/test_microgen.py --bin ./microgen.exe
```

**Test categories:**

| Category | Cases | What it tests |
|----------|-------|---------------|
| IDL mode | 14 | Generate from `.go` interface file + `go build` |
| DB mode | 3 | Generate from SQLite + `go build` |
| `-add-tables` | 1 | Incremental table addition |
| CLI validation | 3 | Error paths (missing args, mutual exclusion) |
| Runtime IDL | 2 | Start generated service, hit `/health` + CRUD routes |
| Runtime DB | 1 | Start DB-generated service, hit `/health` + REST routes |
| Runtime swag | 1 | Swagger UI accessible at `/swagger/index.html` |
| Runtime config | 1 | Config-driven service starts correctly |

---

## test_framework.py

Validates the framework's own API additions.

```bash
python tools/test_framework.py           # run all 9 cases
python tools/test_framework.py -v        # verbose
python tools/test_framework.py -k builder
```

**What it checks:**
- `endpoint.Builder` symbols and tests
- `NewJSONServer` / `DecodeJSONRequest`
- `NewJSONClient`
- `sd.NewEndpoint` / `sd.NewEndpointWithDefaults`
- `quickstart` example compiles and responds to HTTP

---

## test_examples.py

Compiles and smoke-tests all 11 `examples/` directories.

```bash
python tools/test_examples.py                    # all 11 examples
python tools/test_examples.py --no-runtime       # compile + go test only
python tools/test_examples.py -k quickstart      # single example
python tools/test_examples.py -v                 # verbose output
```

**Coverage:**

| Example | Test type |
|---------|-----------|
| `basic` | `go test` — middleware chain order |
| `best_practice` | compile + process start |
| `common`, `multisvc` | compile |
| `profilesvc` | compile + `go test` |
| `quickstart` | compile + `/hello` + `/health` HTTP |
| `middleware` | compile + output verification |
| `httpclient` | compile + round-trip output |
| `sd` | compile + SD/retry/balancer output |
| `transport` | `go test` HTTP server/client + gRPC |
| `usersvc` | compile + microgen generation + compile |

---

## SKILL.md

An AI skill file that teaches an AI assistant (like Kiro) how to use this framework.

It covers:
- Repository layout
- 30-second service example
- Production service pattern
- All key APIs with examples
- Code generation (IDL + DB modes)
- Generated project structure
- Debug endpoints
- Testing patterns
- Common mistakes

**Usage:** Reference this file when asking an AI to build a service with this framework.
In Kiro, add it as a steering file or reference it with `#File tools/SKILL.md`.
