# audit/：入口与组装

组件对外的一个包：`main.go` 与外壳只引用它（`audit.Definition()`）。这里只**组装**：组件定义、配置键，把 `internal/` 下模块的函数登记成 SDK 要的系统面、用户面、作业与恢复核对；业务一行也不写。

## 负责与不负责

- 负责：版本、端口、配置键（`Keys`）、迁移、组件定义（含审计记录的数据类别与历史的读取者的登记）；哪个 gRPC 方法、路由、作业交给哪个模块；证据与记录的读者（`AUDIT_EVIDENCE_READERS`）。
- 不负责：业务规则（去 [internal/AGENTS.md](../internal/AGENTS.md) 找模块）；资源与操作的名字（`internal/core/names.go`）；集成测试（`itest/`）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `definition.go` | `Version`、`GRPCPort`、`Keys`、迁移；`Definition()`（`New` 建 `Module`，有冷热分层时登记审计记录的历史读取者） |
| `module.go` | `Module`；`Services`（接收与水位任何组件可调；证据的 `List` 与 `RecordService` 只给读者）、`Routes`（查询 → `reading.Query`）、`Jobs`（链校验 → `chain.ChainCheck`，间隔 `AUDIT_CHAIN_CHECK_INTERVAL`） |
| `services.go` | gRPC 方法 `Submit`、`Watermark` 转给 `internal/intake`，`Append`、`List` 转给 `internal/evidence` |
| `checks.go` | 整套恢复的核对：本组件审计过的对象一律答“还在”（审计存储不随其他组件回退） |
| `audit_test.go` | 单元测试：合并安全 |
| `manifest_test.go` | 单元测试：清单的 `configSchema` 就是 SDK 与组件读的键 |
| `migrations/` | 表 |

## 依赖与被依赖

- 依赖：`internal/core`、`intake`、`chain`、`evidence`、`reading`、`archive`。
- 被依赖：`main.go`、外壳 `be/go-infra`、`itest/`。

## 测试

`go test ./audit/`。
