# 教程：一个 CRUD 服务

[English](tutorial-crud.md) | 简体中文

本教程从零构建一个数据库支撑的 todo 服务：SQLite 存储、类型化 JSON 路由、已分类
错误与优雅停机。完整代码即可运行的 [examples/todosvc](../examples/README_zh.md)
示例。

## 1. 目标

一个拥有五条路由的服务：

| 方法 | 路径 | 动作 |
| --- | --- | --- |
| `POST` | `/todos` | 创建 todo |
| `GET` | `/todos` | 列出 todo |
| `GET` | `/todos/{id}` | 获取单个 todo |
| `POST` | `/todos/{id}/done` | 标记 todo 完成 |
| `DELETE` | `/todos/{id}` | 删除 todo |

## 2. 存储层

仓储拥有每一条 SQL 语句并保持 context 感知，因此请求取消能传到数据库：

```go
type todoStore struct{ db *sql.DB }

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
```

`init` 在表缺失时创建它。驱动是 `modernc.org/sqlite`，因此服务无需 CGO 即可
构建。

## 3. 服务层

服务层校验输入，并把存储失败映射为 `apperror` kind；传输层永远看不到 SQL：

```go
func (svc todoService) Create(ctx context.Context, title string) (Todo, error) {
	if title == "" {
		return Todo{}, apperror.New(
			apperror.KindInvalidArgument, "todo.title_required", "title is required",
		)
	}
	return svc.store.create(ctx, title, svc.now())
}
```

缺失的行变成 `KindNotFound`，传输层把它编码为 404。

## 4. 传输层

以请求体为形态的请求使用类型化 JSON 路径（严格解码、中间件、信封）。路径参数
使用原生 handler，因为 JSON 请求体无法携带它们：

```go
kit.HandleJSONTyped(svc, "POST /todos", func(ctx context.Context, req createTodoRequest) (Todo, error) {
	return todos.Create(ctx, req.Title)
})

svc.HandleFunc("GET /todos/{id}", func(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	todo, err := todos.Get(r.Context(), id)
	if err != nil {
		writeServiceError(w, err) // apperror kind -> HTTP status
		return
	}
	writeJSON(w, http.StatusOK, todo)
})
```

## 5. 装配与停机

存储在服务启动前打开，并通过一个 `kit.Lifecycle` 组件在优雅停机期间关闭：

```go
store, err := openTodoStore(ctx, *dbDSN)
if err != nil {
	log.Fatalf("open database: %v", err)
}
svc, err := kit.NewHTTP(*httpAddr,
	kit.WithRequestID(),
	kit.WithTimeout(5*time.Second),
)
host, err := kit.NewHost(kit.WithLifecycle(&storeLifecycle{store: store}, svc))
```

## 6. 运行它

```bash
go run ./examples/todosvc

curl -X POST http://localhost:8080/todos -d '{"title":"write the tutorial"}'
curl http://localhost:8080/todos
curl -X POST http://localhost:8080/todos/1/done
curl -i -X DELETE http://localhost:8080/todos/1
curl -i http://localhost:8080/todos/999   # 404 with a stable code
```

## 接下来去哪

- [错误处理](errors_zh.md)：自定义错误信封
- [生命周期](lifecycle_zh.md)：HTTP 服务器旁边的后台任务
- [教程：生成一个服务](tutorial-microgen_zh.md)：改为从 IDL 生成同样的项目形态
