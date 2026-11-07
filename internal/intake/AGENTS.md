# intake：接收审计记录

各组件经 SDK 的发送器把审计记录送来：先按 schema、规范形式与摘要校验，再按身份查重（含已封存的），只收一次；按来源连续编号、算链；来源的产生序号出现空位只告警；回执在提交之后给出。水位答来源可以删到哪里（在本纪元里备份过的）。

## 负责与不负责

- 负责：表 `audit_record`、`audit_counter`、`audit_epoch`（纪元变化时的分叉点）、`audit_source`、`audit_backup`；`IntakeService.Submit`、`Watermark`。
- 不负责：链的定期校验（`chain`）；读取（`reading`）；归档（`archive`）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `intake.go` | `Submit`、`item`（一条记录的核对、查重、编号、链）、`Watermark` |

## 依赖与被依赖

- 依赖：`archive`（`Archived`）、`core`。
- 被依赖：入口 `audit/services.go`。

## 规则在哪

`docs/design.md` 的规则；装配项目协议 03（转运与回执）、协议 14（纪元与分叉点）；“设计取舍”与“易错点”（`AGENTS.md`，仓库根）。

## 测试

`itest/intake_integration_test.go`（`receive`、`refusals`、`restart`）。
