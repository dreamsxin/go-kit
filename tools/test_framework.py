#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
test_framework.py — 框架优化验证测试
=====================================
验证本次优化新增的功能：
  1. NewJSONServer / DecodeJSONRequest  (transport/http/server)
  2. NewJSONClient                      (transport/http/client)
  3. endpoint.Builder / TimeoutMiddleware
  4. sd.NewEndpoint                     (sd/sd.go)
  5. quickstart 示例能编译并正常响应

用法：
  python tools/test_framework.py
  python tools/test_framework.py -v
  python tools/test_framework.py -k builder
"""

import argparse
import subprocess
import sys
import time
import urllib.request
import urllib.error
import json
import os
import signal
from pathlib import Path
from typing import Callable

# ── 强制 stdout/stderr 使用 UTF-8，解决 Windows GBK 乱码 ─────────────────────
if sys.stdout.encoding and sys.stdout.encoding.lower() != "utf-8":
    import io
    sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")
    sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding="utf-8", errors="replace")


REPO = Path(__file__).resolve().parent.parent
RESET = "\033[0m"; GREEN = "\033[32m"; RED = "\033[31m"; CYAN = "\033[36m"; BOLD = "\033[1m"

def ok(m):   print(f"  {GREEN}[OK]{RESET}   {m}")
def fail(m): print(f"  {RED}[FAIL]{RESET} {m}", file=sys.stderr)
def info(m): print(f"  {CYAN}[..]{RESET}   {m}")

# ── helpers ───────────────────────────────────────────────────────────────────

def run(cmd, cwd=None, timeout=60):
    return subprocess.run(cmd, cwd=cwd or REPO, capture_output=True,
                          text=True, timeout=timeout, encoding="utf-8", errors="replace")

def go_test(pkg, extra=None, timeout=60):
    cmd = ["go", "test", "-count=1", "-timeout", f"{timeout}s", pkg]
    if extra:
        cmd += extra
    return run(cmd, timeout=timeout + 5)

# ── test cases ────────────────────────────────────────────────────────────────

class Suite:
    def __init__(self, verbose):
        self.verbose = verbose
        self.passed = self.failed = 0

    def case(self, name: str, fn: Callable):
        print(f"{BOLD}[{name}]{RESET}")
        try:
            fn(self)
            self.passed += 1
            print(f"  → {GREEN}PASS{RESET}\n")
        except AssertionError as e:
            self.failed += 1
            fail(str(e))
            print(f"  → {RED}FAIL{RESET}\n")
        except Exception as e:
            self.failed += 1
            fail(f"exception: {e}")
            print(f"  → {RED}FAIL{RESET}\n")

    def assert_ok(self, cond, msg="assertion failed"):
        if not cond:
            raise AssertionError(msg)

    def assert_in(self, needle, haystack, msg=None):
        if needle not in haystack:
            raise AssertionError(msg or f"{needle!r} not found in output")

    def assert_build(self, pkg):
        r = run(["go", "build", "./..."], cwd=REPO / pkg if (REPO / pkg).is_dir() else REPO)
        if r.returncode != 0:
            raise AssertionError(f"go build failed:\n{r.stderr[:400]}")
        ok(f"go build {pkg}")

    def assert_go_test(self, pkg, run_filter=None):
        extra = [f"-run={run_filter}"] if run_filter else None
        r = go_test(pkg, extra)
        if r.returncode != 0:
            raise AssertionError(f"go test {pkg} failed:\n{r.stdout[-600:]}\n{r.stderr[-400:]}")
        ok(f"go test {pkg}")
        if self.verbose:
            info(r.stdout.strip()[-300:])

# ── individual tests ──────────────────────────────────────────────────────────

def test_build_all(s: Suite):
    """All packages compile cleanly."""
    r = run(["go", "build", "./..."])
    s.assert_ok(r.returncode == 0, f"go build ./... failed:\n{r.stderr[:400]}")
    ok("go build ./...")


def test_endpoint_builder(s: Suite):
    """endpoint.Builder and TimeoutMiddleware compile and tests pass."""
    # Check file exists and has expected symbols
    f = (REPO / "endpoint/builder.go").read_text(encoding="utf-8")
    s.assert_in("type Builder struct", f)
    s.assert_in("func NewBuilder", f)
    s.assert_in("func (b *Builder) Use", f)
    s.assert_in("func (b *Builder) Build", f)
    s.assert_in("func TimeoutMiddleware", f)
    ok("builder.go symbols present")
    s.assert_go_test("./endpoint/...")


def test_json_server(s: Suite):
    """NewJSONServer and DecodeJSONRequest are present and tests pass."""
    f = (REPO / "transport/http/server/json.go").read_text(encoding="utf-8")
    s.assert_in("func NewJSONServer", f)
    s.assert_in("func DecodeJSONRequest", f)
    ok("json.go symbols present")
    s.assert_go_test("./transport/http/server/...")


def test_json_client(s: Suite):
    """NewJSONClient is present and compiles."""
    f = (REPO / "transport/http/client/json.go").read_text(encoding="utf-8")
    s.assert_in("func NewJSONClient", f)
    ok("client/json.go symbols present")
    r = run(["go", "build", "./transport/http/client/..."])
    s.assert_ok(r.returncode == 0, f"build failed: {r.stderr[:300]}")
    ok("transport/http/client builds")


def test_sd_package(s: Suite):
    """sd.NewEndpoint is present and sd package compiles + tests pass."""
    f = (REPO / "sd/sd.go").read_text(encoding="utf-8")
    s.assert_in("func NewEndpoint", f)
    s.assert_in("type Options struct", f)
    s.assert_in("func WithMaxRetries", f)
    s.assert_in("func WithTimeout", f)
    ok("sd/sd.go symbols present")
    s.assert_go_test("./sd/...")


def test_quickstart_build(s: Suite):
    """examples/quickstart compiles."""
    r = run(["go", "build", "./examples/quickstart/..."])
    s.assert_ok(r.returncode == 0, f"quickstart build failed:\n{r.stderr[:400]}")
    ok("examples/quickstart builds")


def test_quickstart_http(s: Suite):
    """quickstart server starts, responds to /hello and /health, then shuts down."""
    # Build binary first
    bin_path = REPO / "_quickstart_test"
    r = run(["go", "build", "-o", str(bin_path), "./examples/quickstart/..."])
    s.assert_ok(r.returncode == 0, f"build failed: {r.stderr[:300]}")

    proc = subprocess.Popen(
        [str(bin_path)],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        cwd=REPO,
    )
    try:
        # Wait for server to be ready
        deadline = time.time() + 5
        while time.time() < deadline:
            try:
                urllib.request.urlopen("http://localhost:8080/health", timeout=1)
                break
            except Exception:
                time.sleep(0.1)
        else:
            raise AssertionError("server did not start within 5s")

        ok("server started")

        # Test /hello
        req_data = json.dumps({"name": "go-kit"}).encode()
        req = urllib.request.Request(
            "http://localhost:8080/hello",
            data=req_data,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urllib.request.urlopen(req, timeout=3) as resp:
            body = json.loads(resp.read())
        s.assert_in("Hello, go-kit!", body.get("message", ""))
        ok(f"/hello → {body}")

        # Test /health
        with urllib.request.urlopen("http://localhost:8080/health", timeout=3) as resp:
            health = json.loads(resp.read())
        s.assert_ok("status" in health)
        ok(f"/health → {health}")

    finally:
        proc.terminate()
        try:
            proc.wait(timeout=3)
        except subprocess.TimeoutExpired:
            proc.kill()
        if bin_path.exists():
            bin_path.unlink()


def test_docs_comments(s: Suite):
    """Key files have package-level doc comments."""
    checks = {
        "endpoint/endpoint.go":       "Package endpoint",
        "endpoint/middleware.go":     "Middleware is a function",
        "transport/http/server/server.go": "Package server",
        "sd/sd.go":                   "Package sd",
    }
    for path, needle in checks.items():
        content = (REPO / path).read_text(encoding="utf-8")
        s.assert_in(needle, content, f"{path}: missing doc comment {needle!r}")
        ok(f"{path}: doc comment present")


def test_existing_tests_still_pass(s: Suite):
    """All pre-existing tests continue to pass."""
    pkgs = [
        "./endpoint/...",
        "./sd/...",
        "./transport/http/server/...",
        "./utils/...",
    ]
    for pkg in pkgs:
        s.assert_go_test(pkg)


def test_typed_endpoint(s: Suite):
    """TypedEndpoint, Unwrap, NewTypedBuilder are present and tests pass."""
    f = (REPO / "endpoint/typed.go").read_text(encoding="utf-8")
    s.assert_in("type TypedEndpoint", f)
    s.assert_in("func Unwrap", f)
    s.assert_in("func NewTypedBuilder", f)
    s.assert_in("type TypeAssertError", f)
    ok("typed.go symbols present")
    s.assert_go_test("./endpoint/...", ["-run", "TestTyped"])


def test_backpressure_tracing(s: Suite):
    """BackpressureMiddleware and TracingMiddleware are present and tests pass."""
    bp = (REPO / "endpoint/backpressure.go").read_text(encoding="utf-8")
    s.assert_in("func BackpressureMiddleware", bp)
    s.assert_in("func InFlightMiddleware", bp)
    s.assert_in("ErrBackpressure", bp)
    ok("backpressure.go symbols present")

    tr = (REPO / "endpoint/tracing.go").read_text(encoding="utf-8")
    s.assert_in("type TraceID", tr)
    s.assert_in("func TracingMiddleware", tr)
    s.assert_in("func WithTraceID", tr)
    s.assert_in("func TraceIDFromContext", tr)
    ok("tracing.go symbols present")

    s.assert_go_test("./endpoint/...", ["-run", "TestBackpressure|TestTracing|TestWithTrace|TestWithRequest|TestBuilder_With"])


def test_json_error_encoder(s: Suite):
    """JSONErrorEncoder is present and server tests pass (including new ones)."""
    f = (REPO / "transport/http/server/error.go").read_text(encoding="utf-8")
    s.assert_in("JSONErrorEncoder", f)
    ok("error.go: JSONErrorEncoder present")
    s.assert_go_test("./transport/http/server/...", ["-run", "TestJSONErrorEncoder|TestNewJSONServer|TestDecodeJSON"])


def test_sd_newep(s: Suite):
    """sd.NewEndpoint and NewEndpointWithDefaults tests pass."""
    s.assert_go_test("./sd/...", ["-run", "TestNewEndpoint|TestNewEndpointWithDefaults"])


def test_kit_package(s: Suite):
    """kit package compiles and Service API is present."""
    f = (REPO / "kit/kit.go").read_text(encoding="utf-8")
    s.assert_in("func New(", f)
    s.assert_in("func (s *Service) Run()", f)
    s.assert_in("func (s *Service) Start()", f)
    s.assert_in("func (s *Service) Shutdown(", f)
    s.assert_in("func JSON[", f)
    s.assert_in("func WithRateLimit", f)
    s.assert_in("func WithCircuitBreaker", f)
    s.assert_in("func WithTimeout", f)
    s.assert_in("func WithMetrics", f)
    ok("kit/kit.go symbols present")
    r = run(["go", "build", "./kit/..."])
    s.assert_ok(r.returncode == 0, f"kit build failed: {r.stderr[:300]}")
    ok("kit package builds")


# ── main ──────────────────────────────────────────────────────────────────────

ALL_TESTS = [
    ("build_all",              test_build_all),
    ("endpoint_builder",       test_endpoint_builder),
    ("typed_endpoint",         test_typed_endpoint),
    ("backpressure_tracing",   test_backpressure_tracing),
    ("json_server",            test_json_server),
    ("json_error_encoder",     test_json_error_encoder),
    ("json_client",            test_json_client),
    ("sd_package",             test_sd_package),
    ("sd_newep",               test_sd_newep),
    ("kit_package",            test_kit_package),
    ("quickstart_build",       test_quickstart_build),
    ("quickstart_http",        test_quickstart_http),
    ("docs_comments",          test_docs_comments),
    ("existing_tests",         test_existing_tests_still_pass),
]

def main():
    p = argparse.ArgumentParser(description="框架优化验证测试")
    p.add_argument("-v", "--verbose", action="store_true")
    p.add_argument("-k", "--filter", default="", help="只运行名称含此字符串的用例")
    args = p.parse_args()

    tests = [(n, f) for n, f in ALL_TESTS if args.filter.lower() in n.lower()]
    if not tests:
        print(f"No tests match filter {args.filter!r}")
        sys.exit(0)

    print(f"\n{BOLD}框架优化验证{RESET}  ({len(tests)} 个用例)\n")
    suite = Suite(args.verbose)
    for name, fn in tests:
        suite.case(name, fn)

    print("─" * 50)
    print(f"{BOLD}结果: {GREEN}{suite.passed} passed{RESET}", end="")
    if suite.failed:
        print(f"  {RED}{suite.failed} failed{RESET}")
        sys.exit(1)
    else:
        print(f"\n{GREEN}所有测试通过 ✓{RESET}")

if __name__ == "__main__":
    main()
