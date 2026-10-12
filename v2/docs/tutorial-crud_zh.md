# 教程：一个 CRUD 服务

[English](tutorial-crud.md) | 简体中文

本教程从零构建一个数据库支撑的 todo 服务：SQLite 存储、类型化 JSON 路由、已分类
错误与优雅停机。下面每个代码块都是完整的——按节顺序操作，`main.go` 就能拼装成
一个可运行的服务。最终结果与可运行的
[examples/todosvc/main.go](../examples/todosvc/main.go) 示例是同一个文件，示例只
多一段更长的包注释。

## 1. 目标

一个拥有五条路由的服务：

| 方法 | 路径 | 动作 |
| --- | --- | --- |
| `POST` | `/todos` | 创建 todo |
| `GET` | `/todos` | 列出 todo |
| `GET` | `/todos/{id}` | 获取单个 todo |
| `POST` | `/todos/{id}/done` | 标记 todo 完成 |
| `DELETE` | `/todos/{id}` | 删除 todo |

## 2. 建立模块

```bash
mkdir todosvc && cd todosvc
go mod init example.com/todosvc
go get github.com/dreamsxin/go-kit/v2@latest
go get modernc.org/sqlite
```

创建 `main.go`，写入包子句和后续每一节都要用到的 import：

```go
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/dreamsxin/go-kit/v2/apperror"
	"github.com/dreamsxin/go-kit/v2/kit"
	httpserver "github.com/dreamsxin/go-kit/v2/transport/http/server"

	_ "modernc.org/sqlite"
)
```

空白 import 注册纯 Go 的 SQLite 驱动，因此服务无需 CGO 即可构建。默认数据库在
内存里，数据只存活一个进程周期。

## 3. 领域类型与存储层

仓储拥有每一条 SQL 语句并保持 context 感知，因此请求取消能传到数据库。追加整
个存储层——领域类型、建表引导，以及每个查询一个方法：

```go
// ── 领域类型（不依赖框架或数据库） ────────────────────────────────────────────

// Todo 是存储的领域对象。
type Todo struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Done      bool   `json:"done"`
	CreatedAt string `json:"created_at"`
}

type createTodoRequest struct {
	Title string `json:"title"`
}

type todoList struct {
	Todos []Todo `json:"todos"`
}

// ── 存储层 ───────────────────────────────────────────────────────────────────

// todoStore 拥有每一条 SQL 语句。所有查询都尊重请求 context，
// 因此客户端取消能传到数据库驱动。
type todoStore struct {
	db *sql.DB
}

func openTodoStore(ctx context.Context, dsn string) (*todoStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	store := &todoStore{db: db}
	if err := store.init(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *todoStore) init(ctx context.Context) error {
	const create = `CREATE TABLE IF NOT EXISTS todos (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		title      TEXT NOT NULL,
		done       INTEGER NOT NULL DEFAULT 0,
		created_at TEXT NOT NULL
	)`
	_, err := s.db.ExecContext(ctx, create)
	return err
}

func (s *todoStore) close() error { return s.db.Close() }

func (s *todoStore) create(ctx context.Context, title string, createdAt time.Time) (Todo, error) {
	const q = `INSERT INTO todos (title, created_at) VALUES (?, ?) RETURNING id`
	var id int64
	if err := s.db.QueryRowContext(ctx, q, title, createdAt.UTC().Format(time.RFC3339)).Scan(&id); err != nil {
		return Todo{}, err
	}
	return Todo{ID: id, Title: title, CreatedAt: createdAt.UTC().Format(time.RFC3339)}, nil
}

func (s *todoStore) get(ctx context.Context, id int64) (Todo, error) {
	const q = `SELECT id, title, done, created_at FROM todos WHERE id = ?`
	return s.scanOne(s.db.QueryRowContext(ctx, q, id))
}

func (s *todoStore) list(ctx context.Context) ([]Todo, error) {
	const q = `SELECT id, title, done, created_at FROM todos ORDER BY id`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var todos []Todo
	for rows.Next() {
		var t Todo
		var done int
		if err := rows.Scan(&t.ID, &t.Title, &done, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.Done = done != 0
		todos = append(todos, t)
	}
	return todos, rows.Err()
}

func (s *todoStore) markDone(ctx context.Context, id int64) (Todo, error) {
	const q = `UPDATE todos SET done = 1 WHERE id = ?`
	res, err := s.db.ExecContext(ctx, q, id)
	if err != nil {
		return Todo{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Todo{}, sql.ErrNoRows
	}
	return s.get(ctx, id)
}

func (s *todoStore) delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM todos WHERE id = ?`
	res, err := s.db.ExecContext(ctx, q, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *todoStore) scanOne(row *sql.Row) (Todo, error) {
	var t Todo
	var done int
	if err := row.Scan(&t.ID, &t.Title, &done, &t.CreatedAt); err != nil {
		return Todo{}, err
	}
	t.Done = done != 0
	return t, nil
}
```

## 4. 服务层

服务层校验输入，并把存储失败映射为 `apperror` kind；传输层永远看不到 SQL。缺失
的行变成 `KindNotFound`，传输层把它编码为 404。追加服务层：

```go
// ── 服务层：校验与错误分类 ───────────────────────────────────────────────────

// todoService 把存储结果转换为已分类的应用错误，
// 于是任何传输层无需了解 SQL 就能映射到正确的状态码。
type todoService struct {
	store *todoStore
	now   func() time.Time
}

func (svc todoService) Create(ctx context.Context, title string) (Todo, error) {
	if title == "" {
		return Todo{}, apperror.New(
			apperror.KindInvalidArgument, "todo.title_required", "title is required",
		)
	}
	return svc.store.create(ctx, title, svc.now())
}

func (svc todoService) Get(ctx context.Context, id int64) (Todo, error) {
	todo, err := svc.store.get(ctx, id)
	return todo, classify(err)
}

func (svc todoService) List(ctx context.Context) (todoList, error) {
	todos, err := svc.store.list(ctx)
	if err != nil {
		return todoList{}, err
	}
	return todoList{Todos: todos}, nil
}

func (svc todoService) MarkDone(ctx context.Context, id int64) (Todo, error) {
	todo, err := svc.store.markDone(ctx, id)
	return todo, classify(err)
}

func (svc todoService) Delete(ctx context.Context, id int64) error {
	return classify(svc.store.delete(ctx, id))
}

func classify(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return apperror.New(apperror.KindNotFound, "todo.not_found", "todo not found")
	}
	return err
}
```

重复键值得同样的待遇，上面的 not-found 模式就是它的模板。在 model 模式的项目
里，生成的仓储已经把驱动报告的唯一约束冲突包进 `repository.ErrDuplicated`
（生成的 main 以 `TranslateError` 打开 gorm，驱动错误在链条中以
`gorm.ErrDuplicatedKey` 到达），一条 `errors.Is` 分支把它映射到状态码 409
的那个 kind：

```go
if errors.Is(err, repository.ErrDuplicated) {
	return apperror.New(apperror.KindAlreadyExists, "todo.exists", "title already exists")
}
```

像本教程这样手写存储时，同样的检查读驱动自己的错误——SQLite 驱动会报告唯一
约束的错误码——落进同一个 kind。要紧的是形状：在服务层分类，重复插入应答
409，而不是以 500 露出。

## 5. 传输层

除删除外的每条路由都是类型化的：请求结构体从请求本身填充——GET 从查询串与路径值，
其他方法从严格解码的 JSON 体再加路径值——因此路径参数是类型化路由的一部分，而不是
退回原生 handler 的理由。没有请求体的请求解为零值，这让不带体的 done 路由也能保持
类型化。删除保留原生逃生舱，因为它的应答是 204 无响应体，并且错误走框架的错误编码
器保持形态一致。追加整个传输层：

```go
// ── HTTP 层 ──────────────────────────────────────────────────────────────────

// getTodoRequest 以路径值指名一个 todo。类型化路由从请求本身填充它：
// GET 读查询串与路径值，其他方法读严格解码的 JSON 体再加路径值，
// 不带请求体的请求解为零值。
type getTodoRequest struct {
	ID int64 `form:"id" json:"id"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// pathID 读取原生 handler 的路径值。上面的类型化路由自己填充请求结构体；
// 这是逃生舱的那一份。
func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"code": "todo.invalid_id", "message": "todo id must be a positive integer"})
		return 0, false
	}
	return id, true
}

func registerRoutes(svc *kit.HTTP, todos todoService) {
	// 类型化路由承载完整请求路径：严格解码、端点中间件，
	// 共享的错误编码器把分类后的错误应答为 {"code","message","request_id"}。
	kit.HandleJSONTyped(svc, "POST /todos", func(ctx context.Context, req createTodoRequest) (Todo, error) {
		return todos.Create(ctx, req.Title)
	})

	kit.HandleJSONTyped(svc, "GET /todos", func(ctx context.Context, _ struct{}) (todoList, error) {
		return todos.List(ctx)
	})

	kit.HandleJSONTyped(svc, "GET /todos/{id}", func(ctx context.Context, req getTodoRequest) (Todo, error) {
		return todos.Get(ctx, req.ID)
	})

	// 不带请求体的 POST：请求解为零值，路径值填充 ID，
	// 于是按钮路由和其余路由一样是类型化的。
	kit.HandleJSONTyped(svc, "POST /todos/{id}/done", func(ctx context.Context, req getTodoRequest) (Todo, error) {
		return todos.MarkDone(ctx, req.ID)
	})

	// 原生逃生舱保留在应答不是 JSON 的地方：204 无响应体。
	// 错误走框架编码器，因此形态与类型化路由一致。
	svc.HandleFunc("DELETE /todos/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		if err := todos.Delete(r.Context(), id); err != nil {
			httpserver.JSONErrorEncoder(r.Context(), err, w)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

// storeLifecycle 教会 kit 在优雅停机期间关闭数据库。
type storeLifecycle struct {
	store *todoStore
	errs  chan error
}

func (l *storeLifecycle) Start() error                   { return nil }
func (l *storeLifecycle) Errors() <-chan error           { return l.errs }
func (l *storeLifecycle) Shutdown(context.Context) error { return l.store.close() }
```

成功的类型化响应直接写出值本身，不做包装：`POST /todos` 返回的就是 `Todo` 对象。
只有错误才有固定形态（`{"code","message","request_id"}`）。若你也想给成功响应加一
层包装，用 `kit.WithJSONServerOptions(httpserver.ServerResponseEncoder(...))` 为
整个服务安装一个——见[定制](customization_zh.md)。

## 6. 装配与停机

存储在服务启动前打开，并在优雅停机期间关闭。组件按给定顺序启动、按相反顺序停
机，因此存储比它支撑的服务器活得更久。追加装配代码，文件就完整了：

```go
// ── 装配 ─────────────────────────────────────────────────────────────────────

func main() {
	httpAddr := flag.String("http.addr", ":8080", "HTTP listen address")
	dbDSN := flag.String("db.dsn", ":memory:", "SQLite DSN; :memory: keeps data for one process run")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := openTodoStore(ctx, *dbDSN)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	todos := todoService{store: store, now: time.Now}

	svc, err := kit.NewHTTP(*httpAddr,
		kit.WithRequestID(),
		kit.WithTimeout(5*time.Second),
	)
	if err != nil {
		log.Fatal(err)
	}

	registerRoutes(svc, todos)

	log.Println("todosvc example listening on", *httpAddr)

	host, err := kit.NewHost(kit.WithLifecycle(&storeLifecycle{store: store}, svc))
	if err != nil {
		log.Fatal(err)
	}
	if err := host.Run(ctx); err != nil {
		log.Fatal(err)
	}
}
```

`ctx` 来自 `signal.NotifyContext`，因此 Ctrl-C 触发优雅停机，而不是直接杀掉进程。

## 7. 运行它

在教程模块里运行你刚拼装好的文件：

```bash
go run .

curl -X POST http://localhost:8080/todos \
     -H "Content-Type: application/json" \
     -d '{"title":"write the tutorial"}'
curl http://localhost:8080/todos
curl -X POST http://localhost:8080/todos/1/done
curl -i -X DELETE http://localhost:8080/todos/1
curl -i http://localhost:8080/todos/999
# 404 {"code":"todo.not_found","message":"todo not found","request_id":"…"}
```

每条命令都按注释所说响应。默认 DSN 是 `:memory:`，数据只存活一个进程周期；传
`-db.dsn=todos.db` 可持久化。

同一个服务也可以在仓库检出里从 `v2/` 模块目录用 `go run ./examples/todosvc`
运行。

## 接下来去哪

- [错误处理](errors_zh.md)：自定义错误信封
- [生命周期](lifecycle_zh.md)：HTTP 服务器旁边的后台任务
- [教程：生成一个服务](tutorial-microgen_zh.md)：改为从 IDL 生成同样的项目形态
