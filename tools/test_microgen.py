#!/usr/bin/env python3
"""
microgen 端到端集成测试
======================
测试流程：
  1. 用 microgen 从 IDL 文件生成项目
  2. 对生成的项目运行 go build ./...
  3. 验证关键文件存在且内容正确

用法：
  python tools/test_microgen.py                  # 使用默认 microgen.exe
  python tools/test_microgen.py --bin ./microgen.exe
  python tools/test_microgen.py --verbose
  python tools/test_microgen.py -k grpc          # 只跑名称含 grpc 的用例
"""

import argparse
import os
import re
import shutil
import subprocess
import sys
import tempfile
import textwrap
import time
from dataclasses import dataclass, field
from pathlib import Path
from typing import List, Optional

# ─────────────────────────── 颜色输出 ────────────────────────────

RESET = "\033[0m"
GREEN = "\033[32m"
RED   = "\033[31m"
YELLOW= "\033[33m"
CYAN  = "\033[36m"
BOLD  = "\033[1m"

def ok(msg):   print(f"  {GREEN}[OK]{RESET} {msg}")
def fail(msg): print(f"  {RED}[FAIL]{RESET} {msg}")
def info(msg): print(f"  {CYAN}[..]{RESET} {msg}")
def warn(msg): print(f"  {YELLOW}[!!]{RESET} {msg}")

# ─────────────────────────── 工具函数 ────────────────────────────

REPO_ROOT = Path(__file__).resolve().parent.parent

def run(cmd: List[str], cwd: Optional[Path] = None, timeout: int = 120) -> subprocess.CompletedProcess:
    return subprocess.run(
        cmd, cwd=cwd,
        capture_output=True, text=True, timeout=timeout,
        encoding="utf-8", errors="replace",
    )

def get_repo_go_directive() -> tuple[str, str]:
    """读取 repo go.mod 里的 go 和 toolchain 指令，返回 (go_ver, toolchain_ver)。
    toolchain_ver 可能为空（旧版 go.mod 没有该指令）。"""
    gomod = REPO_ROOT / "go.mod"
    content = gomod.read_text(encoding="utf-8")
    go_m = re.search(r'^go (\S+)', content, re.MULTILINE)
    tc_m = re.search(r'^toolchain (\S+)', content, re.MULTILINE)
    go_ver = go_m.group(1) if go_m else "1.21"
    tc_ver = tc_m.group(1) if tc_m else ""
    return go_ver, tc_ver


def fix_replace_directive(out_dir: Path) -> None:
    """修正生成的 go.mod：
    1. replace ../  → repo 根目录绝对路径
    2. go / toolchain 版本 → 与 repo go.mod 保持一致（避免工具链版本冲突）
    """
    gomod = out_dir / "go.mod"
    if not gomod.exists():
        return
    content = gomod.read_text(encoding="utf-8")

    # 1. replace 路径 → 绝对路径
    abs_root = str(REPO_ROOT).replace("\\", "/")
    content = re.sub(
        r'(replace\s+github\.com/dreamsxin/go-kit\s+=>\s+)\.\.',
        lambda m: m.group(1) + abs_root,
        content,
    )

    # 2. 同步 go / toolchain 版本
    go_ver, tc_ver = get_repo_go_directive()
    content = re.sub(r'^go \S+', f'go {go_ver}', content, flags=re.MULTILINE)
    if tc_ver:
        if re.search(r'^toolchain ', content, re.MULTILINE):
            content = re.sub(r'^toolchain \S+', f'toolchain {tc_ver}', content, flags=re.MULTILINE)
        else:
            # 在 go 行后面插入 toolchain 行
            content = re.sub(r'^(go \S+)', rf'\1\ntoolchain {tc_ver}', content, flags=re.MULTILINE)

    gomod.write_text(content, encoding="utf-8")


def go_mod_tidy(out_dir: Path, verbose: bool = False) -> bool:
    """在生成目录运行 go mod tidy，返回是否成功。"""
    fix_replace_directive(out_dir)
    env = os.environ.copy()
    env["GOTOOLCHAIN"] = "auto"
    r = subprocess.run(
        ["go", "mod", "tidy"], cwd=out_dir,
        capture_output=True, text=True, timeout=180,
        encoding="utf-8", errors="replace", env=env,
    )
    if r.returncode != 0:
        print(textwrap.indent((r.stderr or r.stdout)[:600], "    "))
        return False
    return True

def go_build(out_dir: Path, verbose: bool = False) -> tuple[bool, str]:
    """在生成目录运行 go build ./...，返回 (成功, 错误信息)。"""
    env = os.environ.copy()
    env["GOTOOLCHAIN"] = "auto"
    r = subprocess.run(
        ["go", "build", "-mod=mod", "./..."], cwd=out_dir,
        capture_output=True, text=True, timeout=180,
        encoding="utf-8", errors="replace", env=env,
    )
    if r.returncode != 0:
        return False, r.stderr
    return True, ""

# ─────────────────────────── 断言辅助 ────────────────────────────

@dataclass
class AssertionError_(Exception):
    message: str

class Checker:
    def __init__(self, out_dir: Path, verbose: bool):
        self.out_dir = out_dir
        self.verbose = verbose
        self.errors: List[str] = []

    def _path(self, rel: str) -> Path:
        return self.out_dir / rel

    def exists(self, rel: str):
        p = self._path(rel)
        if p.exists():
            ok(f"exists: {rel}")
        else:
            msg = f"missing: {rel}"
            fail(msg)
            self.errors.append(msg)

    def not_exists(self, rel: str):
        p = self._path(rel)
        if not p.exists():
            ok(f"absent:  {rel}")
        else:
            msg = f"should not exist: {rel}"
            fail(msg)
            self.errors.append(msg)

    def contains(self, rel: str, *substrings: str):
        p = self._path(rel)
        if not p.exists():
            msg = f"file missing (cannot check content): {rel}"
            fail(msg)
            self.errors.append(msg)
            return
        content = p.read_text(encoding="utf-8", errors="replace")
        for s in substrings:
            if s in content:
                ok(f"contains {s!r} in {rel}")
            else:
                msg = f"{rel} should contain {s!r}"
                fail(msg)
                self.errors.append(msg)
                if self.verbose:
                    snippet = content[:400].replace("\n", "↵")
                    info(f"  snippet: {snippet}")

    def not_contains(self, rel: str, substring: str):
        p = self._path(rel)
        if not p.exists():
            return
        content = p.read_text(encoding="utf-8", errors="replace")
        if substring not in content:
            ok(f"absent {substring!r} in {rel}")
        else:
            msg = f"{rel} should NOT contain {s!r}"
            fail(msg)
            self.errors.append(msg)

    def build_ok(self):
        """运行 go mod tidy + go build ./..."""
        info("running go mod tidy ...")
        tidy_ok = go_mod_tidy(self.out_dir, self.verbose)
        if not tidy_ok:
            # tidy 失败时：复制 repo 的 go.sum（依赖相同），再尝试 go mod download
            info("go mod tidy failed, copying repo go.sum and running go mod download ...")
            repo_sum = REPO_ROOT / "go.sum"
            if repo_sum.exists():
                shutil.copy(repo_sum, self.out_dir / "go.sum")
            env = os.environ.copy()
            env["GOTOOLCHAIN"] = "auto"
            subprocess.run(
                ["go", "mod", "download"], cwd=self.out_dir,
                capture_output=True, encoding="utf-8", errors="replace", env=env,
            )

        info("running go build ./... ...")
        success, stderr = go_build(self.out_dir, self.verbose)
        if success:
            ok("go build ./... passed")
        else:
            msg = "go build ./... failed"
            fail(msg)
            self.errors.append(msg)
            print(textwrap.indent(stderr[:800], "    "))

    @property
    def passed(self) -> bool:
        return len(self.errors) == 0

# ─────────────────────────── 测试用例定义 ────────────────────────

@dataclass
class TestCase:
    name: str
    args: List[str]                    # microgen 参数（不含 -out）
    idl_content: Optional[str] = None  # 若非 None，写入临时 idl.go
    checks: List[callable] = field(default_factory=list)  # lambda(Checker)
    skip_build: bool = False           # 跳过 go build（如 grpc 需要 protoc）

# ─────────────────────────── IDL 内容 ────────────────────────────

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
    // CreateUser creates a new user.
    CreateUser(ctx context.Context, req CreateUserRequest) (CreateUserResponse, error)
    // GetUser retrieves a user by ID.
    GetUser(ctx context.Context, req GetUserRequest) (GetUserResponse, error)
    // ListUsers lists all users.
    ListUsers(ctx context.Context, req ListUsersRequest) (ListUsersResponse, error)
    // DeleteUser removes a user.
    DeleteUser(ctx context.Context, req DeleteUserRequest) (DeleteUserResponse, error)
    // UpdateUser modifies a user.
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

# ─────────────────────────── 测试用例列表 ────────────────────────

def make_test_cases(import_base: str) -> List[TestCase]:
    return [
        # ── 1. HTTP only, no model ──────────────────────────────
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
                lambda c: c.not_exists("transport/userservice/transport_grpc.go"),
                lambda c: c.not_exists("model/model.go"),
                lambda c: c.contains("service/userservice/service.go",
                                     "UserService", "CreateUser", "GetUser"),
                lambda c: c.contains("endpoint/userservice/endpoints.go",
                                     "MakeServerEndpoints", "MakeCreateUserEndpoint"),
                lambda c: c.contains("transport/userservice/transport_http.go",
                                     "NewHTTPHandler", "decodeCreateUserRequest"),
                lambda c: c.contains("cmd/main.go",
                                     "http.addr", "ListenAndServe"),
                lambda c: c.contains("go.mod",
                                     f"module {import_base}/http_only", "go 1.21"),
                lambda c: c.build_ok(),
            ],
        ),

        # ── 2. HTTP + model + sqlite ────────────────────────────
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
                lambda c: c.contains("cmd/main.go",
                                     "gorm.Open", "db.dsn", "app.db"),
                lambda c: c.build_ok(),
            ],
        ),

        # ── 3. HTTP + model + mysql ─────────────────────────────
        TestCase(
            name="http_model_mysql",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/http_mysql",
                  "-protocols", "http",
                  "-model=true", "-db=true", "-driver", "mysql",
                  "-config=false", "-docs=false"],
            checks=[
                lambda c: c.contains("cmd/main.go",
                                     "gorm.Open", "root:password@tcp"),
                lambda c: c.build_ok(),
            ],
        ),

        # ── 4. HTTP + config ────────────────────────────────────
        TestCase(
            name="http_with_config",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/http_config",
                  "-protocols", "http",
                  "-model=false", "-db=false", "-config=true", "-docs=false"],
            checks=[
                lambda c: c.exists("config/config.yaml"),
                lambda c: c.exists("config/config.go"),
                lambda c: c.contains("config/config.yaml",
                                     "http_addr", "circuit_breaker"),
                lambda c: c.contains("config/config.go",
                                     "type Config struct", "func Load(path string)",
                                     "func Default()"),
                lambda c: c.build_ok(),
            ],
        ),

        # ── 5. HTTP + docs ──────────────────────────────────────
        TestCase(
            name="http_with_docs",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/http_docs",
                  "-protocols", "http",
                  "-model=false", "-db=false", "-config=false", "-docs=true"],
            checks=[
                lambda c: c.exists("README.md"),
                lambda c: c.contains("README.md",
                                     "UserService", "go run ./cmd/main.go"),
                lambda c: c.build_ok(),
            ],
        ),

        # ── 6. HTTP + swag ──────────────────────────────────────
        TestCase(
            name="http_with_swag",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/http_swag",
                  "-protocols", "http",
                  "-model=false", "-db=false", "-config=false", "-docs=false", "-swag=true"],
            checks=[
                lambda c: c.exists("docs/docs.go"),
                lambda c: c.contains("docs/docs.go",
                                     "package docs", "SwaggerInfo", "swag.Register"),
                lambda c: c.build_ok(),
            ],
        ),

        # ── 7. HTTP + tests ─────────────────────────────────────
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

        # ── 8. gRPC（跳过 go build，需要 protoc）───────────────
        TestCase(
            name="grpc_proto_generated",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/grpc_test",
                  "-protocols", "http,grpc",
                  "-model=false", "-db=false", "-config=false", "-docs=false"],
            skip_build=True,   # pb.go 需要 protoc，跳过 build
            checks=[
                lambda c: c.exists("pb/userservice/userservice.proto"),
                lambda c: c.exists("transport/userservice/transport_grpc.go"),
                lambda c: c.contains("pb/userservice/userservice.proto",
                                     'syntax = "proto3"',
                                     "service UserService",
                                     "rpc CreateUser"),
                lambda c: c.contains("transport/userservice/transport_grpc.go",
                                     "NewGRPCServer",
                                     "NewGRPCCreateUserClient"),
                lambda c: c.contains("cmd/main.go", "grpc.addr"),
                lambda c: c.contains("client/userservice/demo.go", "GRPCClient"),
            ],
        ),

        # ── 9. gRPC + config ────────────────────────────────────
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

        # ── 10. 多服务 IDL ──────────────────────────────────────
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

        # ── 11. IDL 文件被复制到输出目录 ────────────────────────
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

        # ── 12. go.mod 内容正确 ─────────────────────────────────
        TestCase(
            name="gomod_content",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/gomod_check",
                  "-protocols", "http",
                  "-model=false", "-db=false", "-config=false", "-docs=false"],
            checks=[
                lambda c: c.exists("go.mod"),
                lambda c: c.contains("go.mod",
                                     f"module {import_base}/gomod_check",
                                     "go 1.21"),
            ],
        ),

        # ── 13. 使用 examples/usersvc/idl.go（真实 IDL）─────────
        TestCase(
            name="usersvc_idl",
            idl_content=None,   # 使用项目自带 IDL
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

        # ── 14. 路由前缀 ─────────────────────────────────────────
        TestCase(
            name="route_prefix",
            idl_content=BASIC_IDL,
            args=["-import", f"{import_base}/prefix",
                  "-protocols", "http",
                  "-model=false", "-db=false", "-config=false", "-docs=false",
                  "-prefix", "/api/v1"],
            checks=[
                lambda c: c.contains("transport/userservice/transport_http.go",
                                     "/api/v1"),
                lambda c: c.build_ok(),
            ],
        ),
    ]

# ─────────────────────────── 测试运行器 ──────────────────────────

@dataclass
class Result:
    name: str
    passed: bool
    duration: float
    errors: List[str]
    skipped: bool = False

def run_test(tc: TestCase, microgen_bin: str, verbose: bool) -> Result:
    start = time.time()
    with tempfile.TemporaryDirectory(prefix="microgen_test_") as tmp:
        out_dir = Path(tmp) / "out"
        out_dir.mkdir()

        # 写入临时 IDL 文件
        if tc.idl_content is not None:
            idl_file = Path(tmp) / "idl.go"
            idl_file.write_text(tc.idl_content, encoding="utf-8")
            cmd = [microgen_bin, "-idl", str(idl_file), "-out", str(out_dir)] + tc.args
        else:
            # 使用 args 中已有的 -idl 参数
            cmd = [microgen_bin, "-out", str(out_dir)] + tc.args

        if verbose:
            info(f"cmd: {' '.join(cmd)}")

        r = run(cmd, cwd=REPO_ROOT, timeout=60)
        if r.returncode != 0:
            duration = time.time() - start
            err = f"microgen exited {r.returncode}: {r.stderr[:300]}"
            fail(err)
            return Result(tc.name, False, duration, [err])

        checker = Checker(out_dir, verbose)
        for check_fn in tc.checks:
            if tc.skip_build and check_fn.__code__.co_consts and "build_ok" in str(check_fn):
                continue
            try:
                check_fn(checker)
            except Exception as e:
                checker.errors.append(f"check raised exception: {e}")
                fail(str(e))

        duration = time.time() - start
        return Result(tc.name, checker.passed, duration, checker.errors)

# ─────────────────────────── 主入口 ──────────────────────────────

def main():
    parser = argparse.ArgumentParser(description="microgen 端到端集成测试")
    parser.add_argument("--bin", default=str(REPO_ROOT / "microgen.exe"),
                        help="microgen 可执行文件路径")
    parser.add_argument("--verbose", "-v", action="store_true",
                        help="显示详细输出")
    parser.add_argument("-k", "--filter", default="",
                        help="只运行名称含此字符串的用例")
    parser.add_argument("--import-base", default="example.com/gentest",
                        help="生成项目的 import path 前缀")
    args = parser.parse_args()

    # 检查 microgen 是否存在
    bin_path = args.bin
    if not Path(bin_path).exists():
        # 尝试 go run
        bin_path = None
        warn(f"{args.bin} not found, will use 'go run ./cmd/microgen'")

    # 检查 go 是否可用
    if run(["go", "version"]).returncode != 0:
        print(f"{RED}ERROR: 'go' not found in PATH{RESET}")
        sys.exit(1)

    test_cases = make_test_cases(args.import_base)
    if args.filter:
        test_cases = [tc for tc in test_cases if args.filter.lower() in tc.name.lower()]
        if not test_cases:
            print(f"{YELLOW}No test cases match filter {args.filter!r}{RESET}")
            sys.exit(0)

    # 确定实际执行命令
    if bin_path:
        microgen_cmd = bin_path
    else:
        # 用 go run 代替（慢但不需要预先 build）
        microgen_cmd = "go"

    print(f"\n{BOLD}microgen 集成测试{RESET}  ({len(test_cases)} 个用例)\n")
    print(f"  bin  : {bin_path or 'go run ./cmd/microgen'}")
    print(f"  root : {REPO_ROOT}\n")

    results: List[Result] = []
    for tc in test_cases:
        print(f"{BOLD}[{tc.name}]{RESET}")

        if bin_path is None:
            # go run 模式：把 -idl / -out 等参数拼到 go run 后面
            # 需要特殊处理，这里直接 build 一次再用
            build_r = run(["go", "build", "-o", str(REPO_ROOT / "_microgen_tmp"),
                           "./cmd/microgen"], cwd=REPO_ROOT)
            if build_r.returncode != 0:
                print(f"{RED}Failed to build microgen: {build_r.stderr}{RESET}")
                sys.exit(1)
            microgen_cmd = str(REPO_ROOT / "_microgen_tmp")
            bin_path = microgen_cmd  # 后续复用

        result = run_test(tc, microgen_cmd, args.verbose)
        results.append(result)

        status = f"{GREEN}PASS{RESET}" if result.passed else f"{RED}FAIL{RESET}"
        print(f"  → {status}  ({result.duration:.1f}s)\n")

    # 清理临时 build 产物
    tmp_bin = REPO_ROOT / "_microgen_tmp"
    if tmp_bin.exists():
        tmp_bin.unlink()

    # ── 汇总 ──
    passed = sum(1 for r in results if r.passed)
    failed = len(results) - passed
    total_time = sum(r.duration for r in results)

    print("─" * 50)
    print(f"{BOLD}结果: {GREEN}{passed} passed{RESET}", end="")
    if failed:
        print(f"  {RED}{failed} failed{RESET}", end="")
    print(f"  ({total_time:.1f}s total){RESET}\n")

    if failed:
        print(f"{RED}失败用例:{RESET}")
        for r in results:
            if not r.passed:
                print(f"  {r.name}:")
                for e in r.errors:
                    print(f"    - {e}")
        sys.exit(1)
    else:
        print(f"{GREEN}所有测试通过 ✓{RESET}")

if __name__ == "__main__":
    main()
