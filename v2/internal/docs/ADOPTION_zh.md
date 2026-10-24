# 采用

这份文件是维护者侧的台账，记录框架声誉中不是代码的那部分：项目被收录在哪、提交
材料写了什么、发布保持什么节奏。指南（`AGENTS.md`）说"没人要的工作总结文档"不进
树——这份是例外，因为采用推进对用户侧的承诺就是材料存在且保持最新。

## 项目现状

`dreamsxin/go-kit` 创建于 2026-08-27，尚无 star 与 fork。本项目与之为对标对象的
Go 框架梯队（gin、fiber、echo、kratos、go-zero、经典 go-kit）在 2 万～8 万
star，是多年社区采用的积累。star 是采用的结果，不是写代码能直接朝它推进的目标；
杠杆是内容质量、可发现性、以及一份公开的"承诺被守住"的历史——门禁体系本来就
在生产这份历史，现在需要把它展示出来，而不只是拥有。

## 渠道与材料

- **awesome-go** —— 首要收录列表。提交是向 `avelino/awesome-go` 发 PR，在
  Microservices 下加一行：
  `- [go-kit](https://github.com/dreamsxin/go-kit) - A standard library for microservices: service/endpoint/transport layering, endpoint middleware, HTTP and gRPC adapters, service discovery, MCP support, and a contract-driven generator.`

  它的收录门槛是文档化、在维护、可证明有用：本仓库的手册、可运行示例、双操作
  系统 CI、以及被评审的承诺门禁就是这份证据。PR 描述应链接 `docs/index.md`、
  `examples/README.md` 与基准方法论（`docs/benchmarks.md`），而不是宣称数字。
- **r/golang 与 Gophers Slack #go-projects** —— 帖子以这里的独特之处开头
  （机器校验的承诺、默认 W3C 关联、生成器），链接手册，欢迎批评。诚实的定位：
  早期项目、API 仍在冻结前、寻找有意见的用户。
- **Go Blog / This Week in Go** —— 在有采用事实可报之前不适用；等第一位外部
  贡献者之后再议。

## 发布节奏

可信度靠一份可见的、小步兑现的发布历史复利：

- 路线图还有里程碑期间，每 4-6 周一个 minor 发布；每次发布点名它的里程碑与
  有意的不决策；
- patch 发布只为用户可能遇到的缺陷——纯文档与纯 tools 的改动不打 tag（发布
  指南的规则）；
- changelog 保持面向用户：改了什么、谁付账、该做什么；
- 连续两次发布没有 `!` 提交之后，提议宣布兼容性冻结——那是谨慎的采用者停止
  给项目打折的时刻。

## 这份台账不得破坏的规则

本页的每一条主张都必须是仓库能"展示"而非"声称"的：基准引用它的 harness，承诺
引用它的门禁，节奏承诺要么被守住、要么本页写明没守住。跑在仓库前面的采用材料，
就是设计立场所禁止的那种营销。
