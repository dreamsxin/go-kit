# 资源

[English](resources.md) | 简体中文

一条资源路由就是 HTTP pattern 背后的一张表：列表、创建、读取、更新、删除、聚合。
`resource` 包是这条路由所说的语言——一棵过滤树、过滤所依据的字段元数据、把请求
收窄的合并规则，以及这一切交给存储的仓储契约。本章是入口；精确契约在
[`resource` README](../resource/README.md) 与 godoc 里。

## 为什么是树，不是查询字符串

写成文本的过滤属于解析它的数据库驱动。请求与驱动之间的任何一层都无法收窄它：
一个必须限制行的策略只能说是或否，因为它够不到字符串内部。是值的过滤属于框架：

- 策略约束就是 `resource.And(policy, client)`——客户端的条件保留，策略的条件
  叠加，谁也不能放宽谁；
- 审计记录存的是规范化树，两条查询读一眼就能比对；
- 被拒绝的请求点名错在哪个操作数，而不是引用一段没人指得动的字符串。

客户端发送一个 JSON 对象——`{"age": {"$gte": 18, "$lt": 65}}`，配
`$and`/`$or`/`$not`、`$in`、`$dateRange`、`$empty`——由 `resource.ParseFilter`
变成树。键按排序顺序访问，同一份文档永远产出同一棵树。随后
`Filter.Validate(schema)` 拒绝 schema 未声明的一切：未知的字段、不可过滤的字段、
在该字段类型上无意义的操作符、类型不符的操作数。没有任何静默收窄。

## 一次请求是三层

每个请求携带 `resource.Params`——过滤、排序、投影、append、窗口、写掩码——由
三层合并而成：默认层、客户端层、策略层。`MergeParams` 对每个键应用一条规则，
且每条规则都在收窄：过滤取 `and`、投影取交集、页大小取更小值、白名单取交集而
黑名单取并集。什么都没写的一层（`nil`）没有意见；写了空切片的一层什么都不
允许——正是这个区分让两个不相交的白名单失败关闭，而不是坍缩成"不受限"。

## 挂载一个 collection

```go
collection, err := resource.NewCollection("products", schema, store, "id")
if err != nil {
    return err
}
routes, err := rest.Routes(collection, rest.WithPolicy(policy))
if err != nil {
    return err
}
for _, route := range routes {
    kit.HandleEndpoint(http, route.Pattern, route.Endpoint, route.Decode)
}
```

`resource.NewCollection` 在构造时就校验声明——不是路径段的名称、不是字段的主键、
nil 仓储都会当场失败。`rest.Routes` 把它变成路由值：pattern、endpoint、decoder。
没有 `kit` 组件时，同样的路由能挂在裸 `*http.ServeMux` 上；包围 endpoint 的东西
不同，它回答的东西相同。

路由表与严格的参数读取（路由不读的参数被拒绝而不是忽略；URL 解析器无法完整读取
的查询串整体拒绝）见 [`resource/rest` README](../resource/rest/README.md)。

## 仓储契约

```go
type Repository interface {
    List(ctx context.Context, query Query) (Page, error)
    Get(ctx context.Context, scope Scope) (Record, error)
    Create(ctx context.Context, record Record, mask WriteMask) (Record, error)
    Update(ctx context.Context, scope Scope, patch Record, mask WriteMask) (Record, error)
    Destroy(ctx context.Context, scope Scope) error
    Aggregate(ctx context.Context, query AggregateQuery) (AggregateResult, error)
}
```

单记录操作拿的是 `Scope`——键加上调用方被授予的条件——因为键是地址不是权限，
先检查再写入中间有一个窗口。被条件排除的记录返回 `NotFound`，绝不是
`PermissionDenied`。`resource/memory` 是参考实现：不是生产存储，而是每个 SQL
适配器都必须与之一致的样板，也是不带数据库测试路由或策略的夹具。

## 生成项目

`microgen -resource`（配合 `-db`）把每张生成的表声明为
`resource.Collection` 并挂载其 REST 面，生成的服务因此在表上回答资源协议。参数
与生成布局见 [microgen 指南](../MICROGEN_zh.md)。

## 有意缺席的东西

操作符集合是封闭的——操作符是每个适配器都要兑现的承诺，自定义操作符是策略层
无法推理的查询片段。本包不含 SQL（翻译归拥有存储的一方），暂无游标分页，也没有
关联过滤：无法展开 `Appends` 条目的仓储返回 `Unimplemented`，而不是悄悄缺少关联
的记录。完整的清单与理由在 [`resource` README](../resource/README.md)。
