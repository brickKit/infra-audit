# evidence：证据存储

证据记录（线协议 P12.9–P12.13）：记录者即调用者，按（记录身份，阶段）只收一次，存储序号跨纪元只增；冲突记完整性事件；读者由 `AUDIT_EVIDENCE_READERS` 限定（默认恢复工具 `ops/recover`）。

## 负责与不负责

- 负责：表 `audit_evidence`、`audit_evidence_counter`；`EvidenceService.Append`、`List`；证据的类别（`Categories`，与各协议一致）。
- 不负责：审计记录（`intake`）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `evidence.go` | `EvidenceRecordType`、`Categories`、`evidence`、`parseEvidence`、`uuidText`、`sameSubject`、`Append`、`keep`、`conflict`、`List` |
| `evidence_test.go` | 单元测试：类别就是各协议的类别 |

## 依赖与被依赖

- 依赖：`core`。
- 被依赖：入口 `audit/services.go`。

## 规则在哪

线协议第 12 章；装配项目协议 15。

## 测试

`evidence_test.go`；`itest/evidence_integration_test.go`（`TestTheEvidenceStore`：重复、冲突、结论对意图、冒名的记录者、读者、顺序与分页）。
