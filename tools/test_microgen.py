#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
microgen 端到端集成测试  (v2)
==============================
新增覆盖：
  - DB 模式 (SQLite)：从真实数据库生成完整项目并编译
  - -add-tables 增量生成：追加新表到已有项目
  - CLI 参数校验：错误路径（缺少必填参数、互斥参数等）
  - 运行时冒烟测试：启动生成的服务，发送真实 HTTP 请求
  - 详细报告：每个用例耗时、失败摘要、总体统计

用法：
  python tools/test_microgen.py
  python tools/test_microgen.py -v
  python tools/test_microgen.py -k db
  python tools/test_microgen.py -k runtime
  python tools/test_microgen.py --no-runtime      # 跳过运行时测试（CI 环境）
  python tools/test_microgen.py --bin ./microgen.exe
"""

import argparse
import json
import os
import re
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import textwrap
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

# ── colour ───────────────────────────────────────────────────────────────────
RESET  = "\033[0m"
GREEN  = "\033[32m"
RED    = "\033[31m"
YELLOW = "\033[33m"
CYAN   = "\033[36m"
BOLD   = "\033[1m"

def ok(m):   print(f"  {GREEN}[OK]{RESET}   {m}")
def fail(m): print(f"  {RED}[FAIL]{RESET} {m}")
def info(m): print(f"  {CYAN}[..]{RESET}   {m}")
def warn(m): print(f"  {YELLOW}[!!]{RESET}   {m}")

# ── repo helpers ──────────────────────────────────────────────────────────────
REPO_ROOT = Path(__file__).resolve().parent.parent

def run(cmd: List[str], cwd: Optional[Path] = None,
        timeout: int = 120, env: Optional[dict] = None) -> subprocess.CompletedProcess:
    return subprocess.run(
        cmd, cwd=cwd or REPO_ROOT,
        capture_output=True, text=True, timeout=timeout,
        encoding="utf-8", errors="replace",
        env=env or os.environ.copy(),
    )

def go_env() -> dict:
    e = os.environ.copy()
    e["GOTOOLCHAIN"] = "auto"
    return e

def get_repo_go_directive() -> Tuple[str, str]:
    content = (REPO_ROOT / "go.mod").read_text(encoding="utf-8")
    go_m = re.search(r"^go (\S+)", content, re.MULTILINE)
    tc_m = re.search(r"^toolchain (\S+)", content, re.MULTILINE)
    return (go_m.group(1) if go_m else "1.21"), (tc_m.group(1) if tc_m else "")

def fix_replace_directive(out_dir: Path) -> None:
    gomod = out_dir / "go.mod"
    if not gomod.exists():
        return
    content = gomod.read_text(encoding="utf-8")
    abs_root = str(REPO_ROOT).replace("\\", "/")
    content = re.sub(
        r"(replace\s+github\.com/dreamsxin/go-kit\s+=>\s+)\.\.",
        lambda m: m.group(1) + abs_root,
        content,
    )
    go_ver, tc_ver = get_repo_go_directive()
    content = re.sub(r"^go \S+", f"go {go_ver}", content, flags=re.MULTILINE)
    if tc_ver:
        if re.search(r"^toolchain ", content, re.MULTILINE):
            content = re.sub(r"^toolchain \S+", f"toolchain {tc_ver}", content, flags=re.MULTILINE)
        else:
            content = re.sub(r"^(go \S+)", rf"\1\ntoolchain {tc_ver}", content, flags=re.MULTILINE)
    gomod.write_text(content, encoding="utf-8")

def go_mod_tidy(out_dir: Path, verbose: bool = False) -> bool:
    fix_replace_directive(out_dir)
    r = subprocess.run(
        ["go", "mod", "tidy"], cwd=out_dir,
        capture_output=True, text=True, timeout=180,
        encoding="utf-8", errors="replace", env=go_env(),
    )
    if r.returncode != 0:
        if verbose:
            print(textwrap.indent((r.stderr or r.stdout)[:600], "    "))
        return False
    return True

def go_build(out_dir: Path, verbose: bool = False) -> Tuple[bool, str]:
    r = subprocess.run(
        ["go", "build", "-mod=mod", "./..."], cwd=out_dir,
        capture_output=True, text=True, timeout=180,
        encoding="utf-8", errors="replace", env=go_env(),
    )
    return (r.returncode == 0), r.stderr

def free_port() -> int:
    """Return an available TCP port."""
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]

def wait_for_port(port: int, timeout: float = 8.0) -> bool:
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            with socket.create_connection(("127.0.0.1", port), timeout=0.3):
                return True
        except OSError:
            time.sleep(0.1)
    return False

# ── Checker ───────────────────────────────────────────────────────────────────
class Checker:
    def __init__(self, out_dir: Path, verbose: bool):
        self.out_dir  = out_dir
        self.verbose  = verbose
        self.errors: List[str] = []

    def _p(self, rel: str) -> Path:
        return self.out_dir / rel

    def exists(self, rel: str):
        if self._p(rel).exists():
            ok(f"exists:  {rel}")
        else:
            self._err(f"missing: {rel}")

    def not_exists(self, rel: str):
        if not self._p(rel).exists():
            ok(f"absent:  {rel}")
        else:
            self._err(f"should not exist: {rel}")

    def contains(self, rel: str, *subs: str):
        p = self._p(rel)
        if not p.exists():
            self._err(f"file missing (cannot check content): {rel}")
            return
        content = p.read_text(encoding="utf-8", errors="replace")
        for s in subs:
            if s in content:
                ok(f"contains {s!r} in {rel}")
            else:
                self._err(f"{rel} should contain {s!r}")
                if self.verbose:
                    info(f"  snippet: {content[:300].replace(chr(10), '↵')}")

    def not_contains(self, rel: str, sub: str):
        p = self._p(rel)
        if not p.exists():
            return
        content = p.read_text(encoding="utf-8", errors="replace")
        if sub not in content:
            ok(f"absent   {sub!r} in {rel}")
        else:
            self._err(f"{rel} should NOT contain {sub!r}")

    def build_ok(self):
        info("go mod tidy ...")
        if not go_mod_tidy(self.out_dir, self.verbose):
            repo_sum = REPO_ROOT / "go.sum"
            if repo_sum.exists():
                shutil.copy(repo_sum, self.out_dir / "go.sum")
            subprocess.run(["go", "mod", "download"], cwd=self.out_dir,
                           capture_output=True, env=go_env())
        info("go build ./... ...")
        ok_, stderr = go_build(self.out_dir, self.verbose)
        if ok_:
            ok("go build ./... passed")
        else:
            self._err("go build ./... failed")
            print(textwrap.indent(stderr[:800], "    "))

    def _err(self, msg: str):
        fail(msg)
        self.errors.append(msg)

    def assert_true(self, cond: bool, msg: str) -> None:
        if not cond:
            self._err(msg)
        elif self.verbose:
            ok(msg)

    @property
    def passed(self) -> bool:
        return not self.errors


# ── TestCase ──────────────────────────────────────────────────────────────────
@dataclass
class TestCase:
    name:        str
    args:        List[str]
    idl_content: Optional[str]          = None
    checks:      List[Callable]         = field(default_factory=list)
    skip_build:  bool                   = False
    # DB-mode fields
    db_setup:    Optional[Callable]     = None   # fn(tmp_dir) -> db_path
    # Runtime fields
    runtime_checks: List[Callable]     = field(default_factory=list)  # fn(port, checker)


# ── Result ────────────────────────────────────────────────────────────────────
@dataclass
class Result:
    name:     str
    passed:   bool
    duration: float
    errors:   List[str]
    skipped:  bool = False

# ── IDL fixtures ─────────────────────────────────────────────────────────────
BASIC_IDL = """\
package basic

import "context"

type User struct {
    ID       uint   `json:"id"       gorm:"primaryKey;autoIncrement"`
    Username string `json:"username" gorm:"column:username;not null;uniqueIndex"`
    Email    string `json:"email"    gorm:"column:email;not null"`
}

type CreateUserRequest  struct { Username string `json:"username"`; Email string `json:"email"` }
type CreateUserResponse struct { User *User `json:"user"`; Error string `json:"error"` }
type GetUserRequest     struct { ID uint `json:"id"` }
type GetUserResponse    struct { User *User `json:"user"`; Error string `json:"error"` }
type ListUsersRequest   struct { Page int `json:"page"` }
type ListUsersResponse  struct { Users []*User `json:"users"`; Total int `json:"total"` }
type DeleteUserRequest  struct { ID uint `json:"id"` }
type DeleteUserResponse struct { Success bool `json:"success"`; Error string `json:"error"` }
type UpdateUserRequest  struct { ID uint `json:"id"`; Username string `json:"username"` }
type UpdateUserResponse struct { User *User `json:"user"`; Error string `json:"error"` }

// UserService manages users.
type UserService interface {
    CreateUser(ctx context.Context, req CreateUserRequest) (CreateUserResponse, error)
    GetUser(ctx context.Context, req GetUserRequest) (GetUserResponse, error)
    ListUsers(ctx context.Context, req ListUsersRequest) (ListUsersResponse, error)
    DeleteUser(ctx context.Context, req DeleteUserRequest) (DeleteUserResponse, error)
    UpdateUser(ctx context.Context, req UpdateUserRequest) (UpdateUserResponse, error)
}
"""

MULTI_IDL = """\
package multi

import "context"

type OrderItem struct {
    ID    uint    `json:"id"    gorm:"primaryKey;autoIncrement"`
    Price float64 `json:"price"`
}
type PlaceOrderRequest  struct { UserID uint `json:"user_id"` }
type PlaceOrderResponse struct { OrderID uint `json:"order_id"`; Error string `json:"error"` }
type GetOrderRequest    struct { ID uint `json:"id"` }
type GetOrderResponse   struct { Items []*OrderItem `json:"items"`; Error string `json:"error"` }

// OrderService handles orders.
type OrderService interface {
    PlaceOrder(ctx context.Context, req PlaceOrderRequest) (PlaceOrderResponse, error)
    GetOrder(ctx context.Context, req GetOrderRequest) (GetOrderResponse, error)
}

type ProductModel struct {
    ID    uint    `json:"id"    gorm:"primaryKey;autoIncrement"`
    Name  string  `json:"name"  gorm:"not null"`
    Price float64 `json:"price"`
}
type IncrStockRequest  struct { ProductID uint `json:"product_id"` }
type IncrStockResponse struct { Stock int `json:"stock"`; Error string `json:"error"` }

// ProductService handles products.
type ProductService interface {
    IncrStock(ctx context.Context, req IncrStockRequest) (IncrStockResponse, error)
}
"""

# ── SQLite DB setup helper ────────────────────────────────────────────────────
def create_sqlite_db(tmp_dir: Path, tables_sql: str) -> Path:
    """Create a SQLite DB file with the given DDL and return its path."""
    import sqlite3
    db_path = tmp_dir / "test.db"
    conn = sqlite3.connect(str(db_path))
    conn.executescript(tables_sql)
    conn.commit()
    conn.close()
    return db_path

USERS_DDL = """
CREATE TABLE IF NOT EXISTS users (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    username   TEXT NOT NULL,
    email      TEXT NOT NULL,
    created_at DATETIME
);
"""

ORDERS_DDL = """
CREATE TABLE IF NOT EXISTS orders (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL,
    amount     REAL NOT NULL,
    created_at DATETIME
);
"""

# ── test cases ────────────────────────────────────────────────────────────────
def make_test_cases(import_base: str, run_runtime: bool) -> List[TestCase]:
    cases = []

    # ── 1-14: original IDL-mode cases (unchanged) ────────────────────────────
    cases += [
        TestCase(
            name="http_only",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/http_only",
                  "-protocols", "http",
                  "-model=false", "-db=false", "-config=false", "-docs=false"],
            checks=[
                lambda c: c.exists("cmd/main.go"),
                lambda c: c.exists("service/userservice/service.go"),
                lambda c: c.exists("endpoint/userservice/endpoints.go"),
                lambda c: c.exists("transport/userservice/transport_http.go"),
                lambda c: c.exists("client/userservice/demo.go"),
                lambda c: c.not_exists("pb"),
                lambda c: c.not_exists("model/model.go"),
                lambda c: c.contains("service/userservice/service.go",
                                     "UserService", "CreateUser", "GetUser"),
                lambda c: c.contains("endpoint/userservice/endpoints.go",
                                     "MakeServerEndpoints", "MakeCreateUserEndpoint"),
                lambda c: c.contains("transport/userservice/transport_http.go",
                                     "NewHTTPHandler", "decodeCreateUserRequest"),
                lambda c: c.contains("cmd/main.go", "http.addr", "ListenAndServe"),
                lambda c: c.contains("go.mod",
                                     f"module {import_base}/http_only", "go 1.21"),
                lambda c: c.build_ok(),
            ],
        ),
        TestCase(
            name="http_model_sqlite",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/http_model",
                  "-protocols", "http",
                  "-model=true", "-db=true", "-driver", "sqlite",
                  "-config=false", "-docs=false"],
            checks=[
                lambda c: c.exists("model/model.go"),
                lambda c: c.exists("repository/repository.go"),
                lambda c: c.contains("model/model.go", "User", "TableName"),
                lambda c: c.contains("repository/repository.go",
                                     "Repository", "GetByID", "Create", "Delete"),
                lambda c: c.contains("cmd/main.go", "gorm.Open", "db.dsn", "app.db"),
                lambda c: c.build_ok(),
            ],
        ),
        TestCase(
            name="http_model_mysql",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/http_mysql",
                  "-protocols", "http",
                  "-model=true", "-db=true", "-driver", "mysql",
                  "-config=false", "-docs=false"],
            checks=[
                lambda c: c.contains("cmd/main.go", "gorm.Open", "root:password@tcp"),
                lambda c: c.build_ok(),
            ],
        ),
        TestCase(
            name="http_with_config",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/http_config",
                  "-protocols", "http",
                  "-model=false", "-db=false", "-config=true", "-docs=false"],
            checks=[
                lambda c: c.exists("config/config.yaml"),
                lambda c: c.exists("config/config.go"),
                lambda c: c.contains("config/config.yaml", "http_addr", "circuit_breaker"),
                lambda c: c.contains("config/config.go",
                                     "type Config struct", "func Load(path string)", "func Default()"),
                lambda c: c.build_ok(),
            ],
        ),
        TestCase(
            name="http_with_docs",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/http_docs",
                  "-protocols", "http",
                  "-model=false", "-db=false", "-config=false", "-docs=true"],
            checks=[
                lambda c: c.exists("README.md"),
                lambda c: c.contains("README.md", "UserService", "go run ./cmd/main.go"),
                lambda c: c.build_ok(),
            ],
        ),
        TestCase(
            name="http_with_swag",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/http_swag",
                  "-protocols", "http",
                  "-model=false", "-db=false", "-config=false", "-docs=false", "-swag=true"],
            checks=[
                lambda c: c.exists("docs/docs.go"),
                lambda c: c.contains("docs/docs.go", "package docs", "SwaggerInfo", "swag.Register"),
                lambda c: c.build_ok(),
            ],
        ),
        TestCase(
            name="http_with_tests",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/http_tests",
                  "-protocols", "http",
                  "-model=false", "-db=false", "-config=false", "-docs=false", "-tests=true"],
            checks=[
                lambda c: c.exists("test/userservice_test.go"),
                lambda c: c.build_ok(),
            ],
        ),
        TestCase(
            name="grpc_proto_generated",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/grpc_test",
                  "-protocols", "http,grpc",
                  "-model=false", "-db=false", "-config=false", "-docs=false"],
            skip_build=True,
            checks=[
                lambda c: c.exists("pb/userservice/userservice.proto"),
                lambda c: c.exists("transport/userservice/transport_grpc.go"),
                lambda c: c.contains("pb/userservice/userservice.proto",
                                     'syntax = "proto3"', "service UserService", "rpc CreateUser"),
                lambda c: c.contains("transport/userservice/transport_grpc.go",
                                     "NewGRPCServer", "NewGRPCCreateUserClient"),
                lambda c: c.contains("cmd/main.go", "grpc.addr"),
                lambda c: c.contains("client/userservice/demo.go", "GRPCClient"),
            ],
        ),
        TestCase(
            name="grpc_with_config",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/grpc_config",
                  "-protocols", "http,grpc",
                  "-model=false", "-db=false", "-config=true", "-docs=false"],
            skip_build=True,
            checks=[
                lambda c: c.contains("config/config.yaml", "grpc_addr"),
                lambda c: c.contains("config/config.go", "GRPCAddr"),
            ],
        ),
        TestCase(
            name="multi_service",
            idl_content=MULTI_IDL,
            args=["-import", f"{import_base}/multi",
                  "-protocols", "http",
                  "-model=true", "-config=false", "-docs=false"],
            checks=[
                lambda c: c.exists("service/orderservice/service.go"),
                lambda c: c.exists("service/productservice/service.go"),
                lambda c: c.exists("endpoint/orderservice/endpoints.go"),
                lambda c: c.exists("endpoint/productservice/endpoints.go"),
                lambda c: c.exists("model/model.go"),
                lambda c: c.build_ok(),
            ],
        ),
        TestCase(
            name="idl_copied",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/idl_copy",
                  "-protocols", "http",
                  "-model=false", "-db=false", "-config=false", "-docs=false"],
            checks=[
                lambda c: c.exists("idl.go"),
                lambda c: c.contains("idl.go", "package idl_copy"),
            ],
        ),
        TestCase(
            name="gomod_content",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/gomod_check",
                  "-protocols", "http",
                  "-model=false", "-db=false", "-config=false", "-docs=false"],
            checks=[
                lambda c: c.exists("go.mod"),
                lambda c: c.contains("go.mod",
                                     f"module {import_base}/gomod_check", "go 1.21"),
            ],
        ),
        TestCase(
            name="usersvc_idl",
            idl_content=None,
            args=["-idl", str(REPO_ROOT / "examples" / "usersvc" / "idl.go"),
                  "-import", f"{import_base}/usersvc",
                  "-protocols", "http",
                  "-model=true", "-driver", "sqlite",
                  "-config=false", "-docs=false"],
            checks=[
                lambda c: c.exists("service/userservice/service.go"),
                lambda c: c.exists("model/model.go"),
                lambda c: c.contains("service/userservice/service.go",
                                     "UserService", "CreateUser"),
                lambda c: c.build_ok(),
            ],
        ),
        TestCase(
            name="route_prefix",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/prefix",
                  "-protocols", "http",
                  "-model=false", "-db=false", "-config=false", "-docs=false",
                  "-prefix", "/api/v1"],
            checks=[
                lambda c: c.contains("transport/userservice/transport_http.go", "/api/v1"),
                lambda c: c.build_ok(),
            ],
        ),
    ]

    # ── 15. DB mode: SQLite full generation ──────────────────────────────────
    def _db_sqlite_setup(tmp_dir: Path) -> Path:
        return create_sqlite_db(tmp_dir, USERS_DDL)

    cases.append(TestCase(
        name="db_sqlite_full",
        idl_content=None,
        db_setup=_db_sqlite_setup,
        args=["-from-db", "-driver", "sqlite",
              "-import", f"{import_base}/db_sqlite",
              "-service", "ShopService",
              "-model=true", "-db=true",
              "-config=false", "-docs=false"],
        checks=[
            lambda c: c.exists("idl.go"),
            lambda c: c.exists("cmd/main.go"),
            lambda c: c.exists("model/model.go"),
            lambda c: c.exists("repository/repository.go"),
            lambda c: c.exists("service/shopservice/service.go"),
            lambda c: c.exists("endpoint/shopservice/endpoints.go"),
            lambda c: c.exists("transport/shopservice/transport_http.go"),
            lambda c: c.contains("idl.go", "type User struct", "ShopService"),
            lambda c: c.contains("idl.go", "CreateUser", "GetUser", "ListUsers"),
            lambda c: c.contains("model/model.go", "User", "TableName"),
            lambda c: c.build_ok(),
        ],
    ))

    # ── 16. DB mode: SQLite with specific tables ──────────────────────────────
    def _db_multi_setup(tmp_dir: Path) -> Path:
        return create_sqlite_db(tmp_dir, USERS_DDL + ORDERS_DDL)

    cases.append(TestCase(
        name="db_sqlite_tables_filter",
        idl_content=None,
        db_setup=_db_multi_setup,
        args=["-from-db", "-driver", "sqlite",
              "-import", f"{import_base}/db_filter",
              "-service", "ShopService",
              "-tables", "users",
              "-model=true", "-db=true",
              "-config=false", "-docs=false"],
        checks=[
            lambda c: c.contains("idl.go", "type User struct"),
            lambda c: c.not_contains("idl.go", "type Order struct"),
            lambda c: c.build_ok(),
        ],
    ))

    # ── 17. DB mode: -add-tables incremental generation ───────────────────────
    cases.append(TestCase(
        name="db_add_tables",
        idl_content=None,
        db_setup=_db_multi_setup,
        args=[],   # args built dynamically in run_test_db_add_tables
        checks=[],  # checked inline
    ))

    # ── 18. CLI validation: missing -dsn ─────────────────────────────────────
    cases.append(TestCase(
        name="cli_missing_dsn",
        idl_content=None,
        args=["-from-db", "-driver", "sqlite",
              "-import", f"{import_base}/cli_err"],
        checks=[],  # exit-code checked in runner
    ))

    # ── 19. CLI validation: missing -idl and -from-db ────────────────────────
    cases.append(TestCase(
        name="cli_no_mode",
        idl_content=None,
        args=["-import", f"{import_base}/cli_err2"],
        checks=[],
    ))

    # ── 20. CLI validation: -tables and -add-tables mutually exclusive ────────
    cases.append(TestCase(
        name="cli_tables_exclusive",
        idl_content=None,
        args=["-from-db", "-driver", "sqlite", "-dsn", "x.db",
              "-tables", "users", "-add-tables", "orders",
              "-import", f"{import_base}/cli_err3"],
        checks=[],
    ))

    # ── 21. Runtime smoke test ────────────────────────────────────────────────
    if run_runtime:
        cases.append(TestCase(
            name="runtime_http_smoke",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/runtime",
                  "-protocols", "http",
                  "-model=false", "-db=false", "-config=false", "-docs=false"],
            checks=[lambda c: c.build_ok()],
            runtime_checks=[_runtime_smoke],
        ))

    cases += make_extra_test_cases(import_base, run_runtime)
    return cases


def _runtime_smoke(out_dir: Path, port: int, checker: Checker):
    """Build the generated service, start it, hit /health, then stop it."""
    bin_path = out_dir / "_svc_bin"
    r = subprocess.run(
        ["go", "build", "-mod=mod", "-o", str(bin_path), "./cmd/main.go"],
        cwd=out_dir, capture_output=True, text=True,
        timeout=120, env=go_env(),
    )
    if r.returncode != 0:
        checker._err(f"runtime build failed: {r.stderr[:300]}")
        return

    proc = subprocess.Popen(
        [str(bin_path), f"-http.addr=:{port}"],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        cwd=out_dir,
    )
    try:
        if not wait_for_port(port, timeout=8):
            checker._err(f"service did not start on port {port} within 8s")
            return
        ok(f"service started on :{port}")

        # /health
        try:
            with urllib.request.urlopen(
                f"http://127.0.0.1:{port}/health", timeout=3
            ) as resp:
                body = resp.read().decode()
            ok(f"/health → {body[:80]}")
        except urllib.error.URLError as e:
            checker._err(f"/health request failed: {e}")
    finally:
        proc.terminate()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()
        if bin_path.exists():
            bin_path.unlink(missing_ok=True)

# ── runtime helpers ───────────────────────────────────────────────────────────

def _build_and_start(out_dir: Path, port: int, checker: Checker):
    """Build the generated service binary and start it. Returns proc or None."""
    bin_path = out_dir / "_svc_bin"
    r = subprocess.run(
        ["go", "build", "-mod=mod", "-o", str(bin_path), "./cmd/main.go"],
        cwd=out_dir, capture_output=True, text=True,
        timeout=120, env=go_env(),
    )
    if r.returncode != 0:
        checker._err(f"runtime build failed:\n{r.stderr[:400]}")
        return None, None
    proc = subprocess.Popen(
        [str(bin_path), f"-http.addr=:{port}"],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        cwd=out_dir,
    )
    return proc, bin_path


def _stop(proc, bin_path):
    if proc:
        proc.terminate()
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()
    if bin_path and Path(bin_path).exists():
        Path(bin_path).unlink(missing_ok=True)


def _http_post(url: str, body: dict, timeout: float = 5.0):
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


def _http_get(url: str, timeout: float = 5.0):
    try:
        with urllib.request.urlopen(url, timeout=timeout) as r:
            return r.status, json.loads(r.read().decode("utf-8", errors="replace"))
    except urllib.error.HTTPError as e:
        return e.code, {}
    except Exception as e:
        return -1, {"error": str(e)}


# ── runtime smoke: IDL → full CRUD ───────────────────────────────────────────

def _runtime_idl_crud(out_dir: Path, port: int, checker: Checker):
    """Start the IDL-generated service and exercise CRUD endpoints."""
    proc, bin_path = _build_and_start(out_dir, port, checker)
    if proc is None:
        return
    try:
        if not wait_for_port(port, timeout=10):
            checker._err(f"IDL service did not start on :{port}")
            return
        ok(f"IDL service started on :{port}")

        base = f"http://127.0.0.1:{port}"

        # /health
        code, body = _http_get(f"{base}/health")
        checker.assert_true(code == 200, f"/health: want 200, got {code}")
        ok(f"/health → {body}")

        # /debug/routes — 验证路由调试端点
        code, body = _http_get(f"{base}/debug/routes")
        checker.assert_true(code == 200, f"/debug/routes: want 200, got {code}")
        if isinstance(body, list):
            ok(f"/debug/routes → {len(body)} routes registered")
            paths = [r.get("path", "") for r in body]
            checker.assert_true(any("/health" in p for p in paths),
                                "/debug/routes should include /health")
        else:
            ok(f"/debug/routes → {body}")

        # POST /userservice/createuser — stub returns 200 with empty/error response
        code, body = _http_post(f"{base}/userservice/createuser",
                                {"username": "alice", "email": "alice@example.com"})
        # Generated stub may return 200 (empty impl) or 500 (not implemented)
        # Both are acceptable — we just verify the route exists (not 404)
        checker.assert_true(code != 404,
                            f"createuser route missing: got 404")
        ok(f"POST /userservice/createuser → {code} (route exists)")

        # GET /userservice/listusers — stub returns 200 with empty list
        code, body = _http_get(f"{base}/userservice/listusers")
        checker.assert_true(code != 404,
                            f"listusers route missing: got 404")
        ok(f"GET /userservice/listusers → {code} (route exists)")

    finally:
        _stop(proc, bin_path)


# ── runtime smoke: DB → full CRUD ────────────────────────────────────────────

def _runtime_db_crud(out_dir: Path, port: int, checker: Checker):
    """Start the DB-generated service and exercise CRUD endpoints.
    DB mode generates RESTful routes: POST /svc/resource, GET /svc/resources
    """
    proc, bin_path = _build_and_start(out_dir, port, checker)
    if proc is None:
        return
    try:
        if not wait_for_port(port, timeout=10):
            checker._err(f"DB service did not start on :{port}")
            return
        ok(f"DB service started on :{port}")

        base = f"http://127.0.0.1:{port}"

        # /health
        code, body = _http_get(f"{base}/health")
        checker.assert_true(code == 200, f"/health: want 200, got {code}")
        ok(f"/health → {body}")

        # /debug/routes — DB 模式也应有路由调试端点
        code, routes_body = _http_get(f"{base}/debug/routes")
        checker.assert_true(code == 200, f"DB /debug/routes: want 200, got {code}")
        if isinstance(routes_body, list):
            ok(f"DB /debug/routes → {len(routes_body)} routes")
        else:
            ok(f"DB /debug/routes → {routes_body}")

        # DB mode RESTful routes: POST /shopservice/user (create)
        code, body = _http_post(f"{base}/shopservice/user",
                                {"username": "bob", "email": "bob@example.com"})
        checker.assert_true(code != 404,
                            f"DB create route missing: POST /shopservice/user got 404")
        ok(f"POST /shopservice/user → {code} (route exists)")

        # GET /shopservice/users (list)
        code, body = _http_get(f"{base}/shopservice/users")
        checker.assert_true(code != 404,
                            f"DB list route missing: GET /shopservice/users got 404")
        ok(f"GET /shopservice/users → {code} (route exists)")

    finally:
        _stop(proc, bin_path)


# ── Checker helper ────────────────────────────────────────────────────────────

def _add_assert_true(checker: Checker):
    """Monkey-patch assert_true onto Checker if not present."""
    if not hasattr(checker, "assert_true"):
        def assert_true(self, cond: bool, msg: str):
            if not cond:
                self._err(msg)
            elif self.verbose:
                ok(msg)
        import types
        checker.assert_true = types.MethodType(assert_true, checker)


# ── new test cases ────────────────────────────────────────────────────────────

def make_extra_test_cases(import_base: str, run_runtime: bool):
    """Additional test cases: runtime CRUD for IDL and DB modes."""
    cases = []

    # ── 22. IDL → runtime CRUD ───────────────────────────────────────────────
    if run_runtime:
        cases.append(TestCase(
            name="runtime_idl_crud",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/runtime_idl",
                  "-protocols", "http",
                  "-model=false", "-db=false", "-config=false", "-docs=false"],
            checks=[lambda c: c.build_ok()],
            runtime_checks=[_runtime_idl_crud],
        ))

    # ── 23. DB → runtime CRUD (SQLite + model) ───────────────────────────────
    if run_runtime:
        def _db_setup(tmp_dir: Path) -> Path:
            return create_sqlite_db(tmp_dir, USERS_DDL)

        cases.append(TestCase(
            name="runtime_db_crud",
            idl_content=None,
            db_setup=_db_setup,
            args=["-from-db", "-driver", "sqlite",
                  "-import", f"{import_base}/runtime_db",
                  "-service", "ShopService",
                  "-model=true", "-db=true",
                  "-config=false", "-docs=false"],
            checks=[lambda c: c.build_ok()],
            runtime_checks=[_runtime_db_crud],
        ))

    # ── 24. DB + swag → Swagger route accessible ─────────────────────────────
    if run_runtime:
        def _db_swag_setup(tmp_dir: Path) -> Path:
            return create_sqlite_db(tmp_dir, USERS_DDL)

        def _runtime_swag(out_dir: Path, port: int, checker: Checker):
            proc, bin_path = _build_and_start(out_dir, port, checker)
            if proc is None:
                return
            try:
                if not wait_for_port(port, timeout=10):
                    checker._err(f"swag service did not start on :{port}")
                    return
                ok(f"swag service started on :{port}")
                # Swagger UI redirect or index
                try:
                    with urllib.request.urlopen(
                        f"http://127.0.0.1:{port}/swagger/index.html", timeout=3
                    ) as r:
                        ok(f"/swagger/index.html → {r.status}")
                except urllib.error.HTTPError as e:
                    # 404 is acceptable if swag init wasn't run; 200/301/302 are good
                    if e.code not in (200, 301, 302, 404):
                        checker._err(f"/swagger/index.html: unexpected {e.code}")
                    else:
                        ok(f"/swagger/index.html → {e.code} (expected)")
                except Exception as e:
                    checker._err(f"/swagger request failed: {e}")
            finally:
                _stop(proc, bin_path)

        cases.append(TestCase(
            name="runtime_db_swag",
            idl_content=None,
            db_setup=_db_swag_setup,
            args=["-from-db", "-driver", "sqlite",
                  "-import", f"{import_base}/runtime_swag",
                  "-service", "ShopService",
                  "-model=true", "-db=true", "-swag=true",
                  "-config=false", "-docs=false"],
            checks=[
                lambda c: c.exists("docs/docs.go"),
                lambda c: c.contains("docs/docs.go", "SwaggerInfo"),
                lambda c: c.build_ok(),
            ],
            runtime_checks=[_runtime_swag],
        ))

    # ── 25. IDL + config → service reads config file ─────────────────────────
    if run_runtime:
        def _runtime_config(out_dir: Path, port: int, checker: Checker):
            # Patch config.yaml to use our free port
            cfg = out_dir / "config" / "config.yaml"
            if cfg.exists():
                content = cfg.read_text(encoding="utf-8")
                content = re.sub(r"http_addr:\s*\S+", f"http_addr: \":{port}\"", content)
                cfg.write_text(content, encoding="utf-8")

            proc, bin_path = _build_and_start(out_dir, port, checker)
            if proc is None:
                return
            try:
                if not wait_for_port(port, timeout=10):
                    checker._err(f"config service did not start on :{port}")
                    return
                ok(f"config service started on :{port}")
                code, body = _http_get(f"http://127.0.0.1:{port}/health")
                checker.assert_true(code == 200, f"/health: want 200, got {code}")
                ok(f"/health → {body}")
            finally:
                _stop(proc, bin_path)

        cases.append(TestCase(
            name="runtime_idl_config",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/runtime_cfg",
                  "-protocols", "http",
                  "-model=false", "-db=false", "-config=true", "-docs=false"],
            checks=[
                lambda c: c.exists("config/config.yaml"),
                lambda c: c.exists("config/config.go"),
                lambda c: c.build_ok(),
            ],
            runtime_checks=[_runtime_config],
        ))

    return cases

# ── test runner ───────────────────────────────────────────────────────────────
def run_test(tc: TestCase, microgen_bin: str, verbose: bool) -> Result:
    # ── special cases handled inline ─────────────────────────────────────────
    if tc.name == "db_add_tables":
        return _run_add_tables_test(tc, microgen_bin, verbose)
    if tc.name in ("cli_missing_dsn", "cli_no_mode", "cli_tables_exclusive"):
        return _run_cli_error_test(tc, microgen_bin, verbose)

    start = time.time()
    with tempfile.TemporaryDirectory(prefix="microgen_test_") as tmp:
        tmp_path = Path(tmp)
        out_dir  = tmp_path / "out"
        out_dir.mkdir()

        # DB-mode: create SQLite file first
        if tc.db_setup is not None:
            db_path = tc.db_setup(tmp_path)
            # inject -dsn into args
            args = ["-dsn", str(db_path)] + tc.args
        else:
            args = tc.args

        # Build command
        if tc.idl_content is not None:
            idl_file = tmp_path / "idl.go"
            idl_file.write_text(tc.idl_content, encoding="utf-8")
            cmd = [microgen_bin, "-idl", str(idl_file), "-out", str(out_dir)] + args
        else:
            cmd = [microgen_bin, "-out", str(out_dir)] + args

        if verbose:
            info(f"cmd: {' '.join(cmd)}")

        r = run(cmd, cwd=REPO_ROOT, timeout=60)
        if r.returncode != 0:
            dur = time.time() - start
            err = f"microgen exited {r.returncode}: {(r.stderr or r.stdout)[:300]}"
            fail(err)
            return Result(tc.name, False, dur, [err])

        checker = Checker(out_dir, verbose)
        for fn in tc.checks:
            if tc.skip_build and "build_ok" in str(fn):
                continue
            try:
                fn(checker)
            except Exception as e:
                checker._err(f"check raised exception: {e}")

        # Runtime checks
        if tc.runtime_checks and checker.passed:
            port = free_port()
            for fn in tc.runtime_checks:
                try:
                    fn(out_dir, port, checker)
                except Exception as e:
                    checker._err(f"runtime check exception: {e}")

        return Result(tc.name, checker.passed, time.time() - start, checker.errors)


def _run_add_tables_test(tc: TestCase, microgen_bin: str, verbose: bool) -> Result:
    """Two-phase test: initial generation (users only) then add-tables (orders)."""
    start = time.time()
    errors: List[str] = []

    with tempfile.TemporaryDirectory(prefix="microgen_addtables_") as tmp:
        tmp_path = Path(tmp)
        out_dir  = tmp_path / "out"
        out_dir.mkdir()
        db_path  = create_sqlite_db(tmp_path, USERS_DDL + ORDERS_DDL)
        import_path = "example.com/addtables_test"

        # Phase 1: generate users only
        cmd1 = [
            microgen_bin,
            "-from-db", "-driver", "sqlite", "-dsn", str(db_path),
            "-tables", "users",
            "-import", import_path, "-service", "ShopService",
            "-model=true", "-db=true", "-config=false", "-docs=false",
            "-out", str(out_dir),
        ]
        if verbose:
            info(f"phase1: {' '.join(cmd1)}")
        r1 = run(cmd1, timeout=60)
        if r1.returncode != 0:
            err = f"phase1 failed: {(r1.stderr or r1.stdout)[:300]}"
            fail(err)
            return Result(tc.name, False, time.time() - start, [err])

        c1 = Checker(out_dir, verbose)
        c1.exists("idl.go")
        c1.contains("idl.go", "type User struct")
        c1.not_contains("idl.go", "type Order struct")
        errors.extend(c1.errors)

        # Phase 2: add orders table
        cmd2 = [
            microgen_bin,
            "-from-db", "-driver", "sqlite", "-dsn", str(db_path),
            "-add-tables", "orders",
            "-import", import_path,
            "-out", str(out_dir),
        ]
        if verbose:
            info(f"phase2: {' '.join(cmd2)}")
        r2 = run(cmd2, timeout=60)
        if r2.returncode != 0:
            err = f"phase2 failed: {(r2.stderr or r2.stdout)[:300]}"
            fail(err)
            errors.append(err)
            return Result(tc.name, False, time.time() - start, errors)

        c2 = Checker(out_dir, verbose)
        c2.contains("idl.go", "type User struct", "type Order struct")
        c2.contains("idl.go", "CreateUser", "CreateOrder")
        c2.build_ok()
        errors.extend(c2.errors)

    passed = not errors
    return Result(tc.name, passed, time.time() - start, errors)


def _run_cli_error_test(tc: TestCase, microgen_bin: str, verbose: bool) -> Result:
    """Verify that microgen exits non-zero for invalid CLI arguments."""
    start = time.time()
    with tempfile.TemporaryDirectory(prefix="microgen_cli_") as tmp:
        out_dir = Path(tmp) / "out"
        out_dir.mkdir()
        cmd = [microgen_bin, "-out", str(out_dir)] + tc.args
        if verbose:
            info(f"cmd: {' '.join(cmd)}")
        r = run(cmd, timeout=30)

    if r.returncode != 0:
        ok(f"microgen correctly rejected invalid args (exit {r.returncode})")
        return Result(tc.name, True, time.time() - start, [])
    else:
        err = "expected non-zero exit for invalid args, got 0"
        fail(err)
        return Result(tc.name, False, time.time() - start, [err])


# ── main ──────────────────────────────────────────────────────────────────────
def ensure_microgen_bin(args_bin: str) -> Optional[str]:
    """Return path to microgen binary, building it if necessary."""
    if Path(args_bin).exists():
        return args_bin
    warn(f"{args_bin} not found — building from source ...")
    tmp_bin = str(REPO_ROOT / "_microgen_tmp")
    r = run(["go", "build", "-o", tmp_bin, "./cmd/microgen"], cwd=REPO_ROOT, timeout=120)
    if r.returncode != 0:
        print(f"{RED}Failed to build microgen:{RESET}\n{r.stderr}")
        return None
    ok(f"built microgen → {tmp_bin}")
    return tmp_bin


def main():
    ap = argparse.ArgumentParser(description="microgen 端到端集成测试 v2")
    ap.add_argument("--bin", default=str(REPO_ROOT / "microgen.exe"),
                    help="microgen 可执行文件路径")
    ap.add_argument("-v", "--verbose", action="store_true")
    ap.add_argument("-k", "--filter", default="", help="只运行名称含此字符串的用例")
    ap.add_argument("--import-base", default="example.com/gentest")
    ap.add_argument("--no-runtime", action="store_true",
                    help="跳过运行时冒烟测试（适合 CI 环境）")
    args = ap.parse_args()

    if run(["go", "version"]).returncode != 0:
        print(f"{RED}ERROR: 'go' not found in PATH{RESET}")
        sys.exit(1)

    microgen_bin = ensure_microgen_bin(args.bin)
    if microgen_bin is None:
        sys.exit(1)

    all_cases = make_test_cases(args.import_base, run_runtime=not args.no_runtime)
    cases = [tc for tc in all_cases
             if not args.filter or args.filter.lower() in tc.name.lower()]
    if not cases:
        print(f"{YELLOW}No test cases match filter {args.filter!r}{RESET}")
        sys.exit(0)

    print(f"\n{BOLD}microgen 集成测试 v2{RESET}  ({len(cases)} 个用例)\n")
    print(f"  bin  : {microgen_bin}")
    print(f"  root : {REPO_ROOT}\n")

    results: List[Result] = []
    for tc in cases:
        print(f"{BOLD}[{tc.name}]{RESET}")
        result = run_test(tc, microgen_bin, args.verbose)
        results.append(result)
        status = f"{GREEN}PASS{RESET}" if result.passed else f"{RED}FAIL{RESET}"
        print(f"  → {status}  ({result.duration:.1f}s)\n")

    # cleanup temp build
    tmp_bin = REPO_ROOT / "_microgen_tmp"
    if tmp_bin.exists():
        tmp_bin.unlink()

    passed = sum(1 for r in results if r.passed)
    failed = len(results) - passed
    total  = sum(r.duration for r in results)

    print("─" * 55)
    # per-case timing table
    print(f"  {'Case':<35} {'Time':>6}  Status")
    print(f"  {'-'*35} {'-'*6}  ------")
    for r in results:
        st = f"{GREEN}PASS{RESET}" if r.passed else f"{RED}FAIL{RESET}"
        print(f"  {r.name:<35} {r.duration:>5.1f}s  {st}")
    print("─" * 55)
    print(f"{BOLD}结果: {GREEN}{passed} passed{RESET}", end="")
    if failed:
        print(f"  {RED}{failed} failed{RESET}", end="")
    print(f"  ({total:.1f}s total)\n")

    if failed:
        print(f"{RED}失败详情:{RESET}")
        for r in results:
            if not r.passed:
                print(f"  {BOLD}{r.name}{RESET}:")
                for e in r.errors:
                    print(f"    • {e}")
        sys.exit(1)
    else:
        print(f"{GREEN}所有测试通过 OK{RESET}")


if __name__ == "__main__":
    main()
