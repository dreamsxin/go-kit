# resource/rest（HTTP 绑定）

[English](README.md) | 简体中文

`rest` 把一个 `resource.Collection` 挂到 HTTP 上。它是绑定，不是第二套栈：一条路由
是一个值——模式、端点、解码器——挂在哪里由应用决定。

```go
routes, err := rest.Routes(products)
if err != nil {
    return err
}
for _, route := range routes {
    kit.HandleEndpoint(http, route.Pattern, route.Endpoint, route.Decode)
}
```

用普通的 `*http.ServeMux` 一样可以：

```go
for _, route := range routes {
    mux.Handle(route.Pattern, route.Handler())
}
```

两者差的只是端点外面包着什么，不是它回答什么：走 kit 的那条保留组件安装的端点
中间件、记录器和 JSON 服务端选项，裸 handler 只有 `net/http` 给的那些。响应体、
状态码、错误码完全一致，因为两者都用传输层的 JSON 编码器与错误编码器——有一个
逐字节比较两者的测试。

## 路由表

| 模式 | 动作 | 成功响应 |
| --- | --- | --- |
| `GET {prefix}/{name}` | list | 200，一页 |
| `POST {prefix}/{name}` | create | 201，记录本身，带 `Location` |
| `GET {prefix}/{name}:aggregate` | aggregate | 200，分组行 |
| `GET {prefix}/{name}/{id}` | get | 200，记录本身 |
| `PATCH {prefix}/{name}/{id}` | update | 200，记录本身 |
| `DELETE {prefix}/{name}/{id}` | destroy | 204，无响应体 |

聚合路由是 `{name}:aggregate`，不是 `{name}/aggregate`：通配符占满一整个路径段，
所以 `/products/aggregate` 会遮住标识为 `aggregate` 的那条记录；后缀不会。

集合名限制为字母、数字、`-`、`_`、`~` 和 `.`，这既是模式里的安全字符集，也让它
与那个后缀拼接后不会产生歧义。

## 参数

| 参数 | 适用路由 | 含义 |
| --- | --- | --- |
| `filter` | list、aggregate | `resource.ParseFilter` 读的方言；写多个是合取 |
| `sort` | list | `-field,other`，减号降序，也接受加号 |
| `fields` | list、get | 投影；`fields=`（空值）被拒绝，而不是放大 |
| `append` | list | 要展开的关联；本版没有仓储实现展开 |
| `page`、`size` | list | 由传输层自己的 `ParsePage` 解析 |
| `function`、`field`、`group_by` | aggregate | 要算的指标与分组 |

路由不读的参数会被 `rest.parameter_not_allowed` 拒绝，绝不忽略：被丢掉的过滤条件
回答的是一个更宽的问题，而更宽的回答在调用方看来与鉴权失败无法区分。

列限制对每一个回答都成立，不只是列表：写入返回的是投影后的记录，而对被限制的列做
聚合会被 `rest.aggregate_field_not_allowed` 拒绝——求和或分组一个列，泄露的正是
列出它所能泄露的东西。

## 请求体

`POST` 与 `PATCH` 读一个 JSON 对象。每个名字都必须是 schema 声明的字段
（`resource.unknown_field`），每个值都必须符合字段类型
（`resource.value_type_mismatch`）；请求体用 `UseNumber` 解码，所以超过 2^53 的
整数原样到达。写到掩码之外的字段是 `resource.field_not_writable`（403），不会被
丢弃。

空请求体是空记录：create 依赖 schema 的默认值，patch 什么也不改。请求体不是对象
是 `rest.body_not_an_object`。

## 层与层的接缝

```go
rest.WithDefaults(resource.Params{Sort: []resource.Sort{resource.Asc("name")}})
rest.WithLayer(func(ctx context.Context, action rest.Action) (resource.Params, error) {
    return resource.Params{Filter: ownerFilter(ctx)}, nil
})
```

一个请求是三层按同一个顺序合并出来的：资源自己的默认值、客户端要的、以及应用
安装的每一个权威。`resource.MergeParams` 给每个键一条规则，每条规则都只收窄——
过滤只能合取、投影只能求交、页大小只能变小。所以默认值可以被请求覆盖，策略不能。

`WithLayer` 就是鉴权接入的位置：层能拿到上下文与动作（包括 `destroy`），返回的是
约束而不是「行/不行」。

## 刻意不做

- **不做 `PUT`。** 整体替换需要 schema 声明哪些字段必填；它没有声明，而给缺失的列
  编一个零值再覆盖，是带着 200 的数据丢失。
- **路径里不做复合主键。** 一个路径段命名一个值。主键有多个字段时 `Routes` 直接
  拒绝，而不是只寻址半条记录。
- **不做鉴权。** 它只提供接缝，不做决定；那是策略层的职责。
- **不做游标分页与关联展开。** 这两件先是 `resource` 的决定，本包不会替它们发明
  写法。
- **不提供中间件选项。** 中间件属于挂载路由的那一方。这里的选项在 kit 路径上会被
  静默忽略，那比没有更糟。
