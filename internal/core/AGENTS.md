# core：全组件共用的名字与写法（共用）

每个模块都引用、自己什么业务也不做的东西。改这里会影响所有模块。

## 负责与不负责

- 负责：组件 ID、资源与操作、`MaxBatch`、默认的链校验间隔与证据读者、资源定义 `Definitions()`（`names.go`）；纪元（`EpochOf`）、链摘要（`ChainNext`）、存储位置（`Position`、`ParsePosition`）、记录的时钟偏差（`RecordSkew`）（`rules.go`）；`Base`（`base.go`）。
- 不负责：任何业务规则（在各模块）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `base.go` | `Base`：组件的运行时 |
| `names.go` | 常量（含审计记录的保全与删除批准的操作）；`Definitions()` |
| `rules.go` | `EpochOf`、`ChainNext`、`RecordSkew`、`Position`、`ParsePosition` |
| `rules_test.go` | 单元测试：链摘要 |

## 依赖与被依赖

- 依赖：只有 SDK 与契约。
- 被依赖：全部模块与入口。

## 规则在哪

`docs/design.md`；装配项目协议 03、14。

## 测试

`rules_test.go`；其余由 `itest/` 间接覆盖。
