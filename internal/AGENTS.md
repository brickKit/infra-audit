# internal/：业务模块

审计组件的业务，每个模块一个包。只给本仓库引用（Go 的 `internal`）：别的组件编译期就引用不了，跨组件只走契约（决定 0101）。先按下表找到模块，再读那个目录的 `AGENTS.md`。

## 模块

| 包 | 一句话 | 依赖（本组件内） |
| --- | --- | --- |
| `intake` | 接收审计记录、链、水位 | `archive`、`core` |
| `chain` | 链校验（AU14） | `archive`、`core` |
| `evidence` | 证据存储 | `core` |
| `reading` | 用户面的查询与 `RecordService.List` | `archive`、`core` |
| `archive` | 审计记录的数据类别、按身份读归档、历史 | `core` |
| `core`（共用） | 名字与资源定义、纪元、链摘要、存储位置、`Base` | — |

分层（每个包只引用比它低的层）：`intake`、`chain`、`reading` → `archive` → `core`；`evidence` → `core`。

## 共同的写法

- 每个模块的函数先收 `m *core.Base`（组件的运行时），入口包的 `Module` 持有它、登记时适配。
- 每次接收在存储计数行上加锁，锁内连续编号、更新来源的链；回执在提交之后给出（协议 03）。
- 链按来源计算，顺序就是存储的顺序（`core.ChainNext`）；接收查重遇到已封存的记录时查归档（`archive.Archived`）。

## 测试

单元：`core` 的链摘要、`evidence` 的类别；其余由 `itest/` 的流程测试覆盖。
