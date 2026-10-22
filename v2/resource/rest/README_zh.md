# resource/rest（HTTP 绑定）

[English](README.md) | 简体中文

`rest` 把一个 `resource.Collection` 挂到 HTTP 上。它是绑定，不是第二套栈：一条路由
是一个值——模式、端点、解码器——挂在哪里由应用决定。

```go
// WithOpenAccess 声明这个集合谁都能用；另一个答案是 WithPolicy，
// 两样都不给的集合会被拒绝。
routes, err := rest.Routes(products, rest.WithOpenAccess())
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

同样的理由，query string 本身也是严格读取的。`Request.URL.Query` 就是把错误丢掉的
`url.ParseQuery`——能读的留下，读不了的丢掉——所以 `?filter=%zz` 会变成一个没有
filter 的请求，而 `?filter={"status":"active"};size=10`（分号分隔符，Go 已不再接受）
两个参数一起消失，两者都答回全部记录。因此，URL 解析器读不完整的 query string 会在
任何解码器读到一半之前就被 `rest.query_malformed`（`rest.query-malformed`）拒绝：
问了一个更窄问题的客户端会被告知，而不是被回答一个更宽的答案。

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

## 谁能用

一个集合要么带着一个决定挂载，要么挂不上：

```go
routes, err := rest.Routes(products, rest.WithPolicy(
    func(ctx context.Context, action rest.Action) (rest.Decision, error) {
        subject, ok := auth.Subject(ctx)
        if !ok {
            return rest.Decision{}, nil // 零值就是拒绝
        }
        switch action {
        case rest.ActionList, rest.ActionGet, rest.ActionAggregate:
            return rest.Allow(resource.Params{Filter: ownedBy(subject)}), nil
        case rest.ActionCreate, rest.ActionUpdate:
            return rest.Allow(resource.Params{
                Filter:   ownedBy(subject),
                Writable: resource.WriteMask{Deny: []string{"owner_id"}},
            }), nil
        default:
            return rest.Deny("删除是管理员动作"), nil
        }
    }))
```

另一个答案是 `rest.WithOpenAccess()`——所有人、所有事。两样都不给的集合会被 `Routes`
拒绝（`rest.access_required`），两样都给的会被同样拒绝（`rest.access_conflict`）：
策略是白名单，开放是白名单的缺席，无论谁赢，另一行都不再描述这个进程的实际行为。

有三条性质值得写清楚，因为每一条都是这类代码出错的常见方式：

- **`Decision` 的零值是拒绝。** 没写完的策略、漏了分支的 switch、从没被填过的值——
  它们全都是沉默，而沉默必须等于「不行」。`rest.Allow(params)` 才是有人明确做出的
  声明。
- **装上策略就是关上了门。** 没有任何策略授予的动作会被拒绝
  （`rest.action_not_allowed`），所以客户端能做什么等于策略允许什么，而不是
  「全部减去有人记得禁止的那些」。多个策略就是多个权威——租户范围与角色——每一个
  都必须放行，否则求并会让一个权威推翻另一个权威的拒绝。
- **错误不是拒绝。** 拒绝是策略给出的答案；错误是策略给不出答案，它让请求以 500
  停下，而不是藏在 403 后面耗掉一天。

策略拿到的是上下文与动作（包括 `destroy`），拿不到记录：决定是一个条件，不是一次
查看。这个条件随主键一起进存储，所以落在条件之外的 `get`、`update`、`delete` 答
`resource.record_not_found`——与记录不存在同一个答案，因为「它存在但你不能看」等于
把一张不允许读的表描述给了对方。create 反过来校验：要被存下的记录必须满足同一个
条件，而记录没带的字段会让条件变成未知，于是以 `rest.constraint_unsatisfied` 拒绝。

约束和其他层一样合并，排在资源默认值与客户端参数之后，`resource.MergeParams` 只
收窄：过滤合取、投影求交、页大小变小。所以默认值可以被请求覆盖，策略的条件不能。

不能变的条件就是 schema 标成不可写的字段：写掩码在每个适配器上都会拒绝它，这才是
阻止一次 patch 把记录搬出被授予范围的手段。条件与主键由同一条 `WHERE` 回答，问的是
这一行当下的样子——框架不去猜一次 patch 之后它会变成什么。

## 刻意不做

- **不做 `PUT`。** 整体替换需要 schema 声明哪些字段必填；它没有声明，而给缺失的列
  编一个零值再覆盖，是带着 200 的数据丢失。
- **路径里不做复合主键。** 一个路径段命名一个值。主键有多个字段时 `Routes` 直接
  拒绝，而不是只寻址半条记录。
- **对读不了的参数不容忍。** URL 解析器拒绝的 query string、方言表达不出的过滤条件、
  schema 里没有的排序字段：全都是拒绝。读一半，正是把一个窄问题变成宽答案的方式，
  而客户端分辨不出区别。
- **不做认证。** 它不知道调用者是谁：没有身份、没有角色、没有会话。它拥有的是
  一个答案的形状——带约束的决定、默认拒绝、以及能走到存储的条件。
- **不对记录做第二次鉴权。** 策略在请求执行前回答，依据是上下文与动作。「先取回来
  再问一次」会是一个可能推翻第一次的决定，而第一次返回的条件本身已经在选记录了。
- **不做游标分页与关联展开。** 这两件先是 `resource` 的决定，本包不会替它们发明
  写法。
- **不提供中间件选项。** 中间件属于挂载路由的那一方。这里的选项在 kit 路径上会被
  静默忽略，那比没有更糟。
