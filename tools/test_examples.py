#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
test_examples.py  --  examples/ 目录测试工具
=============================================
覆盖所有 examples/ 子目录：
  basic/          - 中间件链顺序 (go test)
  best_practice/  - 编译 + HTTP 冒烟测试
  common/         - 编译检查
  multisvc/       - 编译检查
  profilesvc/     - 编译 + 内存服务单元测试
  quickstart/     - 编译 + HTTP 冒烟测试 (/hello /health)
  transport/      - 所有 go test (HTTP server/client + gRPC)
  usersvc/        - IDL 解析 + microgen 生成编译

用法:
  python tools/test_examples.py
  python tools/test_examples.py -v
  python tools/test_examples.py -k quickstart
  python tools/test_examples.py --no-runtime   # 跳过需要启动进程的测试
"""

import argparse
import json
import os
import socket
import subprocess
import sys
import time
import urllib.error
import urllib.request
from dataclasses import dataclass, field
from pathlib import Path
from typing import Callable, List, Optional, Tuple

# ── 强制 stdout/stderr 使用 UTF-8，解决 Windows GBK 乱码 ─────────────────────
if sys.stdout.encoding and sys.stdout.encoding.lower() != "utf-8":
    import io
    sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding="utf-8", errors="replace")
    sys.stderr = io.TextIOWrapper(sys.stderr.buffer, encoding="utf-8", errors="replace")

RESET  = "\033[0m"
GREEN  = "\033[32m"
RED    = "\033[31m"
YELLOW = "\033[33m"
CYAN   = "\033[36m"
BOLD   = "\033[1m"

def _p(color: str, tag: str, msg: str) -> None:
    print(f"  {color}[{tag}]{RESET}  {msg}", flush=True)

def ok(m):   _p(GREEN,  "OK",   m)
def fail(m): _p(RED,    "FAIL", m)
def info(m): _p(CYAN,   "..",   m)
def warn(m): _p(YELLOW, "!!",   m)

REPO = Path(__file__).resolve().parent.parent

# ── subprocess helpers ────────────────────────────────────────────────────────

def _env() -> dict:
    e = os.environ.copy()
    e["GOTOOLCHAIN"] = "auto"
    # 让 go 工具链输出 UTF-8
    e["GOFLAGS"] = e.get("GOFLAGS", "")
    return e

def run(cmd: List[str], cwd: Optional[Path] = None,
        timeout: int = 120) -> subprocess.CompletedProcess:
    return subprocess.run(
        cmd, cwd=cwd or REPO,
        capture_output=True, text=True,
        encoding="utf-8", errors="replace",
        timeout=timeout, env=_env(),
    )

def go_test(pkg: str, extra: Optional[List[str]] = None,
            cwd: Optional[Path] = None, timeout: int = 60) -> Tuple[bool, str]:
    cmd = ["go", "test", "-v", "-count=1", f"-timeout={timeout}s", pkg]
    if extra:
        cmd += extra
    r = run(cmd, cwd=cwd, timeout=timeout + 5)
    return r.returncode == 0, r.stdout + r.stderr

def go_build(pkg: str, cwd: Optional[Path] = None) -> Tuple[bool, str]:
    r = run(["go", "build", pkg], cwd=cwd)
    return r.returncode == 0, r.stderr

def go_build_bin(pkg: str, out: Path,
                 cwd: Optional[Path] = None) -> Tuple[bool, str]:
    r = run(["go", "build", "-o", str(out), pkg], cwd=cwd)
    return r.returncode == 0, r.stderr

def free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]

def wait_port(port: int, timeout: float = 8.0) -> bool:
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=0.3):
                return True
        except OSError:
            time.sleep(0.1)
    return False

def http_get(url: str, timeout: float = 3.0) -> Tuple[int, str]:
    try:
        with urllib.request.urlopen(url, timeout=timeout) as r:
            return r.status, r.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as e:
        return e.code, ""
    except Exception as e:
        return -1, str(e)

def http_post_json(url: str, body: dict, timeout: float = 3.0) -> Tuple[int, dict]:
    data = json.dumps(body).encode("utf-8")
    req = urllib.request.Request(
        url, data=data,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return r.status, json.loads(r.read().decode("utf-8", errors="replace"))
    except urllib.error.HTTPError as e:
        return e.code, {}
    except Exception as e:
        return -1, {"error": str(e)}

# ── Result / Suite ────────────────────────────────────────────────────────────

@dataclass
class Result:
    name:     str
    passed:   bool
    duration: float
    errors:   List[str]

class Suite:
    def __init__(self, verbose: bool):
        self.verbose = verbose
        self._errors: List[str] = []

    def err(self, msg: str) -> None:
        fail(msg)
        self._errors.append(msg)

    def assert_true(self, cond: bool, msg: str) -> None:
        if not cond:
            self.err(msg)

    def assert_in(self, needle: str, haystack: str, label: str = "") -> None:
        if needle not in haystack:
            self.err(f"{label or 'output'} should contain {needle!r}")
        elif self.verbose:
            ok(f"contains {needle!r}")

    def assert_build(self, pkg: str, cwd: Optional[Path] = None) -> bool:
        ok_, stderr = go_build(pkg, cwd)
        if ok_:
            ok(f"go build {pkg}")
        else:
            self.err(f"go build {pkg} failed:\n{stderr[:400]}")
        return ok_

    def assert_go_test(self, pkg: str, extra: Optional[List[str]] = None,
                       cwd: Optional[Path] = None) -> bool:
        ok_, out = go_test(pkg, extra, cwd)
        if ok_:
            ok(f"go test {pkg}")
            if self.verbose:
                info(out[-300:].strip())
        else:
            self.err(f"go test {pkg} failed:\n{out[-500:]}")
        return ok_

    @property
    def passed(self) -> bool:
        return not self._errors

    @property
    def errors(self) -> List[str]:
        return list(self._errors)


# ── individual example tests ──────────────────────────────────────────────────

def test_basic(s: Suite, run_runtime: bool) -> None:
    """examples/basic: middleware chain order via go test."""
    s.assert_go_test("./examples/basic/...")


def test_best_practice(s: Suite, run_runtime: bool) -> None:
    """examples/best_practice: compile + HTTP smoke test."""
    ok_, stderr = go_build("./examples/best_practice/...", REPO)
    if not ok_:
        s.err(f"go build failed:\n{stderr[:400]}")
        return
    ok(f"go build examples/best_practice")

    if not run_runtime:
        warn("skipping runtime test (--no-runtime)")
        return

    port = free_port()
    bin_ = REPO / "_best_practice_bin"
    ok_, stderr = go_build_bin("./examples/best_practice", bin_, REPO)
    if not ok_:
        s.err(f"build binary failed:\n{stderr[:300]}")
        return

    proc = subprocess.Popen(
        [str(bin_)],
        env={**_env(), "PORT": str(port)},
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        cwd=REPO,
    )
    try:
        # best_practice listens on :8080 hardcoded — use a different approach:
        # just verify the binary starts without crashing for 1s
        time.sleep(1.0)
        if proc.poll() is not None:
            s.err("best_practice binary exited unexpectedly")
        else:
            ok("best_practice binary started OK")
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=3)
        except subprocess.TimeoutExpired:
            proc.kill()
        if bin_.exists():
            bin_.unlink(missing_ok=True)


def test_common(s: Suite, run_runtime: bool) -> None:
    """examples/common: compile check."""
    s.assert_build("./examples/common/...", REPO)


def test_multisvc(s: Suite, run_runtime: bool) -> None:
    """examples/multisvc: IDL compiles cleanly."""
    s.assert_build("./examples/multisvc/...", REPO)


def test_profilesvc(s: Suite, run_runtime: bool) -> None:
    """examples/profilesvc: compile + in-process service unit tests."""
    s.assert_build("./examples/profilesvc/...", REPO)
    # Run the profilesvc package tests (service logic, not HTTP)
    s.assert_go_test("./examples/profilesvc/...", cwd=REPO)


def test_quickstart(s: Suite, run_runtime: bool) -> None:
    """examples/quickstart: compile + /hello + /health HTTP smoke test."""
    ok_, stderr = go_build("./examples/quickstart/...", REPO)
    if not ok_:
        s.err(f"go build failed:\n{stderr[:400]}")
        return
    ok("go build examples/quickstart")

    if not run_runtime:
        warn("skipping runtime test (--no-runtime)")
        return

    port = free_port()
    bin_ = REPO / "_quickstart_bin"
    ok_, stderr = go_build_bin("./examples/quickstart", bin_, REPO)
    if not ok_:
        s.err(f"build binary failed:\n{stderr[:300]}")
        return

    proc = subprocess.Popen(
        [str(bin_), f"-http.addr=:{port}"],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        cwd=REPO,
    )
    try:
        if not wait_port(port, timeout=8):
            s.err(f"quickstart did not start on :{port} within 8s")
            return
        ok(f"quickstart started on :{port}")

        # /health
        code, body = http_get(f"http://127.0.0.1:{port}/health")
        s.assert_true(code == 200, f"/health: want 200, got {code}")
        s.assert_in("status", body, "/health body")
        ok(f"/health -> {body[:60]}")

        # /hello with name
        code, resp = http_post_json(
            f"http://127.0.0.1:{port}/hello",
            {"name": "go-kit"},
        )
        s.assert_true(code == 200, f"/hello: want 200, got {code}")
        s.assert_in("Hello, go-kit!", resp.get("message", ""), "/hello message")
        ok(f"/hello -> {resp}")

        # /hello missing name -> error
        code2, resp2 = http_post_json(
            f"http://127.0.0.1:{port}/hello", {}
        )
        s.assert_true(code2 != 200, f"/hello empty name: want non-200, got {code2}")
        ok(f"/hello empty name -> {code2}")

    finally:
        proc.terminate()
        try:
            proc.wait(timeout=3)
        except subprocess.TimeoutExpired:
            proc.kill()
        if bin_.exists():
            bin_.unlink(missing_ok=True)


def test_transport(s: Suite, run_runtime: bool) -> None:
    """examples/transport: all go tests (HTTP server, HTTP client, gRPC)."""
    # HTTP server tests (no external process needed)
    s.assert_go_test("./examples/transport/server/http/...", cwd=REPO)
    # HTTP client tests (uses httptest internally)
    s.assert_go_test("./examples/transport/client/http/...", cwd=REPO)
    # gRPC tests (uses bufconn, no real network)
    s.assert_go_test("./examples/transport/client/grpc/...", cwd=REPO)
    s.assert_go_test("./examples/transport/server/grpc/...", cwd=REPO)


def test_middleware(s: Suite, run_runtime: bool) -> None:
    """examples/middleware: compile + run, verify key output lines."""
    if not s.assert_build("./examples/middleware/...", REPO):
        return
    if not run_runtime:
        warn("skipping runtime test (--no-runtime)")
        return
    r = run(["go", "run", "./examples/middleware"], cwd=REPO, timeout=30)
    if r.returncode != 0:
        s.err(f"go run failed:\n{r.stderr[:300]}")
        return
    out = r.stdout
    s.assert_in("execution order: [A:pre B:pre C:pre C:post B:post A:post]", out, "Chain order")
    s.assert_in("result: 5.00", out, "Builder result")
    s.assert_in("Failer detected: division by zero", out, "Failer")
    s.assert_in("circuit breaker is open", out, "Gobreaker")
    s.assert_in("rate limit exceeded", out, "ErroringLimiter")
    s.assert_in("would exceed context deadline", out, "DelayingLimiter")
    ok("middleware example output verified")


def test_httpclient(s: Suite, run_runtime: bool) -> None:
    """examples/httpclient: compile + run, verify round-trip output."""
    if not s.assert_build("./examples/httpclient/...", REPO):
        return
    if not run_runtime:
        warn("skipping runtime test (--no-runtime)")
        return
    r = run(["go", "run", "./examples/httpclient"], cwd=REPO, timeout=30)
    if r.returncode != 0:
        s.err(f"go run failed:\n{r.stderr[:300]}")
        return
    out = r.stdout
    s.assert_in("echo: hello", out, "NewJSONClient round-trip")
    s.assert_in("Bearer demo-token", out, "ClientBefore")
    s.assert_in("application/json", out, "ClientAfter")
    s.assert_in("finalizer called", out, "ClientFinalizer")
    ok("httpclient example output verified")


def test_sd(s: Suite, run_runtime: bool) -> None:
    """examples/sd: compile + run, verify SD/retry/balancer output."""
    if not s.assert_build("./examples/sd/...", REPO):
        return
    if not run_runtime:
        warn("skipping runtime test (--no-runtime)")
        return
    r = run(["go", "run", "./examples/sd"], cwd=REPO, timeout=30)
    if r.returncode != 0:
        s.err(f"go run failed:\n{r.stderr[:300]}")
        return
    out = r.stdout
    s.assert_in("no endpoints available", out, "ErrNoEndpoints")
    s.assert_in("host-A:8080", out, "RoundRobin")
    s.assert_in("success on attempt 3", out, "Retry")
    s.assert_in("non-retryable error, stopping", out, "RetryWithCallback")
    s.assert_in("svc1:80", out, "sd.NewEndpoint")
    s.assert_in("cache invalidated as expected", out, "InvalidateOnError")
    ok("sd example output verified")


def test_usersvc(s: Suite, run_runtime: bool) -> None:
    """examples/usersvc: IDL compiles; microgen generates + compiles project."""
    # 1. IDL package compiles
    s.assert_build("./examples/usersvc/...", REPO)
    ok("examples/usersvc IDL compiles")

    # 2. microgen generates from this IDL and the result compiles
    import tempfile, re, shutil
    bin_ = REPO / "microgen.exe"
    if not bin_.exists():
        warn("microgen.exe not found, skipping generation test")
        return

    with tempfile.TemporaryDirectory(prefix="ex_usersvc_") as tmp:
        out = Path(tmp) / "out"
        out.mkdir()
        r = subprocess.run(
            [str(bin_),
             "-idl", str(REPO / "examples" / "usersvc" / "idl.go"),
             "-import", "example.com/usersvc_test",
             "-protocols", "http",
             "-model=true", "-driver", "sqlite",
             "-config=false", "-docs=false",
             "-out", str(out)],
            capture_output=True, text=True,
            encoding="utf-8", errors="replace",
            timeout=60, env=_env(), cwd=REPO,
        )
        if r.returncode != 0:
            s.err(f"microgen failed: {(r.stderr or r.stdout)[:300]}")
            return
        ok("microgen generated usersvc project")

        # fix replace directive
        gomod = out / "go.mod"
        if gomod.exists():
            content = gomod.read_text(encoding="utf-8")
            abs_root = str(REPO).replace("\\", "/")
            content = re.sub(
                r"(replace\s+github\.com/dreamsxin/go-kit\s+=>\s+)\.\.",
                lambda m: m.group(1) + abs_root,
                content,
            )
            # sync go version
            repo_mod = (REPO / "go.mod").read_text(encoding="utf-8")
            go_m = re.search(r"^go (\S+)", repo_mod, re.MULTILINE)
            if go_m:
                content = re.sub(r"^go \S+", f"go {go_m.group(1)}", content, flags=re.MULTILINE)
            gomod.write_text(content, encoding="utf-8")

        # go mod tidy
        tidy = subprocess.run(
            ["go", "mod", "tidy"], cwd=out,
            capture_output=True, text=True,
            encoding="utf-8", errors="replace",
            timeout=180, env=_env(),
        )
        if tidy.returncode != 0:
            # copy go.sum as fallback
            repo_sum = REPO / "go.sum"
            if repo_sum.exists():
                shutil.copy(repo_sum, out / "go.sum")

        build = subprocess.run(
            ["go", "build", "-mod=mod", "./..."], cwd=out,
            capture_output=True, text=True,
            encoding="utf-8", errors="replace",
            timeout=180, env=_env(),
        )
        if build.returncode == 0:
            ok("generated usersvc project compiles")
        else:
            s.err(f"generated project build failed:\n{build.stderr[:500]}")

# ── runner + main ─────────────────────────────────────────────────────────────

ALL_TESTS: List[Tuple[str, Callable]] = [
    ("basic",         test_basic),
    ("best_practice", test_best_practice),
    ("common",        test_common),
    ("httpclient",    test_httpclient),
    ("middleware",    test_middleware),
    ("multisvc",      test_multisvc),
    ("profilesvc",    test_profilesvc),
    ("quickstart",    test_quickstart),
    ("sd",            test_sd),
    ("transport",     test_transport),
    ("usersvc",       test_usersvc),
]


def run_case(name: str, fn: Callable, verbose: bool,
             run_runtime: bool) -> Result:
    start = time.time()
    s = Suite(verbose)
    try:
        fn(s, run_runtime)
    except Exception as e:
        s.err(f"unhandled exception: {e}")
    return Result(name, s.passed, time.time() - start, s.errors)


def main() -> None:
    ap = argparse.ArgumentParser(
        description="examples/ 目录测试工具",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    ap.add_argument("-v", "--verbose", action="store_true",
                    help="显示详细输出")
    ap.add_argument("-k", "--filter", default="",
                    help="只运行名称含此字符串的用例")
    ap.add_argument("--no-runtime", action="store_true",
                    help="跳过需要启动进程的冒烟测试")
    args = ap.parse_args()

    tests = [(n, f) for n, f in ALL_TESTS
             if not args.filter or args.filter.lower() in n.lower()]
    if not tests:
        print(f"No tests match filter {args.filter!r}")
        sys.exit(0)

    print(f"\n{BOLD}examples/ 测试{RESET}  ({len(tests)} 个用例)\n",
          flush=True)

    results: List[Result] = []
    for name, fn in tests:
        print(f"{BOLD}[{name}]{RESET}", flush=True)
        r = run_case(name, fn, args.verbose, not args.no_runtime)
        results.append(r)
        status = f"{GREEN}PASS{RESET}" if r.passed else f"{RED}FAIL{RESET}"
        print(f"  -> {status}  ({r.duration:.1f}s)\n", flush=True)

    # ── summary table ─────────────────────────────────────────────────────────
    col = 22
    print("─" * 50, flush=True)
    print(f"  {'Example':<{col}} {'Time':>6}  Status", flush=True)
    print(f"  {'-'*col} {'------'}  ------", flush=True)
    for r in results:
        st = f"{GREEN}PASS{RESET}" if r.passed else f"{RED}FAIL{RESET}"
        print(f"  {r.name:<{col}} {r.duration:>5.1f}s  {st}", flush=True)
    print("─" * 50, flush=True)

    passed = sum(1 for r in results if r.passed)
    failed = len(results) - passed
    total  = sum(r.duration for r in results)

    print(f"{BOLD}Result: {GREEN}{passed} passed{RESET}", end="", flush=True)
    if failed:
        print(f"  {RED}{failed} failed{RESET}", end="", flush=True)
    print(f"  ({total:.1f}s total)\n", flush=True)

    if failed:
        print(f"{RED}Failures:{RESET}", flush=True)
        for r in results:
            if not r.passed:
                print(f"  {BOLD}{r.name}{RESET}:", flush=True)
                for e in r.errors:
                    print(f"    * {e}", flush=True)
        sys.exit(1)
    else:
        print(f"{GREEN}All examples OK{RESET}", flush=True)


if __name__ == "__main__":
    main()
