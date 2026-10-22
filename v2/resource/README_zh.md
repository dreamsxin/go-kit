# resource（查询词汇）

[English](README.md) | 简体中文

`resource` 包是资源路由所说的查询词汇：一棵过滤树、校验它用的字段元数据、条件被存储
时的规范形式，以及前两者被交给的仓储契约。

它存在的理由很具体：写成字符串的条件，权限层无法收窄、审计日志无法比对、缺陷
报告无法重放。

## 为什么是值，不是查询字符串

过滤条件如果是文本，它就属于解析它的数据库驱动。从请求到驱动之间没有任何一层
能往里加条件——需要限定可见行的策略只能回答「行/不行」，因为它伸不进那根字符串。

过滤条件如果是树，它就属于框架，于是：

- 策略约束是 `resource.And(policy, client)`：客户端的条件保留，策略的条件叠加，
  两者都不能放宽对方；
- 审计记录存 `{"op":"and","children":[...]}`，两条查询读一遍就能比出差异；
- 被拒绝的请求能指出出错的那个操作数，`children[1].children[0]`，而不是引用一段
  没人能指认的字符串。

## 树

```go
type Filter struct {
    Op       Op       // eq、ne、in、nin、gt、gte、lt、lte、like、prefix、
                      // dateRange、empty、notEmpty、and、or、not
    Field    string   // 比较作用于一个字段
    Value    any      // 它的操作数
    Children []Filter // 连接词的操作数
}
```

`Op` 是封闭集合。应用不能注册操作符：操作符是每个仓储适配器都要兑现的承诺，
自定义操作符会成为权限层无法推理、审计日志无法重放的查询片段。

零值 `Filter` 匹配所有记录——它是「没有条件」，不是「丢掉了的条件」。这个区分
就是全部安全性的来源：

| 调用 | 行为 | 理由 |
| --- | --- | --- |
| `And(zero, x)` | `x` | 匹配一切的条件对合取没有贡献 |
| `And(a, b)` | 两者的 `and`，保持顺序 | 摊平、去重、按字段合并，都是约束消失的方式 |
| `Or(zero, x)` | 保留，并由 `Validate` 拒绝 | `or` 的单位元是「什么都不匹配」，丢掉空操作数会把它变成「匹配一切」 |
| `Not(zero)` | 拒绝 | 「非一切」不是一个有人想写的条件 |

`And` 只做一次化简：丢弃零值操作数。

## 客户端发送的方言

一个 JSON 对象。键是字段名或连接词 `$and`、`$or`、`$not`。同一字段上的多个
操作符是合取。

```json
{"status": "active"}
{"age": {"$gte": 18, "$lt": 65}}
{"$or": [{"ownerId": 7}, {"shared": true}]}
{"id": {"$in": [3, 4]}, "publishedAt": {"$dateRange": ["2026-01-01T00:00:00Z","2026-02-01T00:00:00Z"]}}
{"deletedAt": {"$empty": true}}
```

| 操作符 | 适用字段 | 操作数 |
| --- | --- | --- |
| `$eq`、`$ne` | 除不透明文档外的所有类型 | 一个值 |
| `$in`、`$nin` | 所有类型 | 非空列表 |
| `$gt`、`$gte`、`$lt`、`$lte` | string、int、float、time | 一个可排序的值 |
| `$like`、`$prefix` | string | 字面量；`%` 和 `_` 就是字符，不是通配符 |
| `$dateRange` | time | `[from, to)`，半开区间，相邻区间不会同时命中边界 |
| `$empty`、`$notEmpty` | 所有类型 | `true` 或不写 |

`ParseFilter` 接受调用方真正持有的各种形态：请求体的 `json.RawMessage`、查询参数
的 `string`（包括字符串里再包一层文档）、工具调用解码出的 `map[string]any`、以及
表示「没有过滤」的 `nil`。

键按排序后访问，所以同一份文档永远产出同一棵树。同一查询的两条审计记录只会在
查询真的变了时才不同。

## 校验

`Filter.Validate(schema)` 是文本变成条件的边界：

- 资源没有声明的字段是 `filter.unknown_field`，绝不会被悄悄丢掉；
- 声明为 `Filterable: false` 的字段是 `filter.field_not_filterable`；
- 在该字段类型上无意义的操作符是 `filter.operator_not_allowed`——int 上的
  `$like`、bool 上的 `$gt`、string 上的 `$dateRange`；
- 操作数不是该字段的类型是 `filter.type_mismatch`——int 上的小数、time 上的散
  文、`$in` 列表里的一个坏元素。

数字以 `json.Number` 承载，超过 2^53 的标识符不丢精度。

无论树是解析来的还是用 Go 构造的，都限制在 32 层、256 个节点：一个愿意用单次
请求构造上千个条件的查询接口就是一个拒绝服务原语，而无界的深度是藏在鉴权后面
的栈溢出。

## 条件是什么意思

`Matcher` 是可执行的定义，用的是 SQL 的三值逻辑：

| 条件 | 没有该字段的记录 |
| --- | --- |
| `name == "ling"` | unknown |
| `name != "ling"` | unknown |
| `not(name == "ling")` | unknown |
| `empty(name)` | true |

只有答案为 true 的记录会被返回。第三行就是这套区分的全部理由：如果用二值逻辑，
「除 ling 之外的全部」会把每一条没有 name 的记录也返回——那不是查询说的事，也不是
数据库会给出的答案。

比较前先归一化：驱动返回的 `int64`、请求解码出的 `json.Number`、应用代码里的 `int`，
与 `3` 比较时相等。少了这一步的适配器会把「查 3」答成空结果。

## 一个请求是三层

```go
type Params struct {
    Filter   Filter
    Sort     []Sort
    Fields   []string   // nil：全部可读字段；空切片：一个也不要
    Appends  []string
    Window   Window
    Writable WriteMask
}
```

`MergeParams(defaults, client, policy)` 按顺序叠加。每个键只有一条规则，而且每条规则
都只会收窄：

| 键 | 规则 | 理由 |
| --- | --- | --- |
| `Filter` | 合取 | 客户端只能加条件，不能去掉策略的条件 |
| `Fields` | 求交 | 客户端可以少要列，不能多要 |
| `Appends` | 求并 | 展开关联是叠加，不携带权限 |
| `Sort` | 覆盖 | 排序没有安全含义，后一层说了算 |
| `Window` | 取更小的尺寸 | 页大小是唯一有拒绝服务含义的维度 |
| `Writable` | 白名单求交、黑名单求并 | 这是唯一两种不会把某层拒绝的字段放行的运算 |

`nil` 与非 nil 空切片在这里是两个值：`nil` 是「这一层没有意见」，非 nil 空切片是
「这一层一个也不允许」。所以两个互不相交的白名单会求交成空集，也就是失败关闭。把它
塌缩成「无限制」的合并，会用「全部列」去回答一个本该被拒绝的请求——这就是为什么手写
`[]string{}` 这种有歧义的写法宁可什么也不允许。当两个白名单都是投影时，
`MergeParams` 返回 `resource.fields_disjoint`（拒绝），而不是返回没有字段的记录。

## 一个集合就是那份声明

```go
collection, err := resource.NewCollection("products", schema, store, "id")
```

名字、schema、主键字段、仓储。它是值，不是注册表条目：同一张表上两套不同 schema 的
集合，就是应用可以同时持有的两个值——给一个客户端更窄的面，给另一个更宽的面。

构造函数的重点是校验：不是路径段的名字、不是字段的主键、nil 仓储，各自产出一个后续
层无法修补的东西。绕过构造函数写出来的值可以用 `Collection.Validate` 检查，每个传输
层都会检查。

`KeyFromText` 把一个路径段变成键，并在字段自己的域里完成转换：整数键按整数解析，
解析不了的值是指名字段的 invalid-argument 错误，绝不是零值——一个悄悄变成 0 的标识符
指向的是另一条记录。

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

三个单记录操作收的是 `Scope`——主键加上调用方被授予的条件——而不是一个光秃秃的
主键。主键是地址，不是权限：`Get(ctx, key)` 回答的是「这一行存在吗」，而这恰恰是
一个租户之外的调用方不能一个标识符一个标识符地问下去的问题。条件与主键作为同一条
语句执行，因为「先检查再写」中间有一个窗口，而升级就住在这个窗口里。被条件排除的
记录是 `NotFound`，绝不是 `PermissionDenied`：「它存在但你不能看」等于把一张不允许
读的表描述给了对方。

接口归本包，实现归拥有存储的一侧——所以只用 HTTP 的服务不会被迫编进一个数据库驱动。
每个实现都要兑现的约定：

- 查询的每个子句都生效，过滤条件绝不被丢掉或放宽；
- scope 的条件与主键一起生效，被它排除的记录与不存在的记录报同一个结果；
- `Page.Total` 数的是命中的记录，不是窗口里的记录；
- 响应是 schema 的投影，所以 `SELECT *` 也带不出只写列；
- 记录不存在是 `NotFound`，空 key 是 `InvalidArgument`，写到掩码之外是
  `PermissionDenied`，schema 没声明的字段是 `InvalidArgument`——一个都不静默丢弃。

`Page` 序列化为 `{"items","total","page","size","has_next"}`，与
`transport/http.PageResult` 完全同形。两个包不能互相导入，所以传输层里有一个测试逐
字节比较两者的载荷，并把默认值与页大小上限钉在一起。

## 参考实现

`resource/memory` 用一个切片和一个键索引实现了这套接口。它不是生产存储——进程结束就
全没了——但那不是它的用途：它证明契约可以被满足，并给后来每个适配器一个语义基准。
同一个查询在它和 SQL 适配器上必须选出同样的记录，因为它实现的就是上面那套语义。

它同时也是「不接数据库测路由、测策略」的夹具。

## 刻意不做

- **运行期注册操作符。** 集合是封闭的；它表达不了的条件是 schema 问题。
- **本包不含 SQL。** 过滤由拥有存储的一侧翻译——应用自己的适配器，或独立包——
  这样本包不会把驱动塞进每个使用者的依赖图。
- **暂时不做游标分页。** 游标语义是 keyset 语义，通用实现会强加一套本层并不拥有
  的排序键纪律。
- **不做关联过滤与展开。** 第一版里 `posts.comments` 既不是过滤路径，也不实现展开；
  `Appends` 会被携带和求并，而实现不了展开的仓储返回 `Unimplemented`，而不是返回
  静默缺少关联的记录。
- **写入不做静默剥离。** 掩码之外的字段是被拒绝而不是被丢弃：一个答 200 却存了别的
  东西的 create，比一个答 403 的更糟。
