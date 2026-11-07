# archive：审计记录的冷热分层

审计记录作为一个数据类别（A158）：主键 `(source, id)` 用生成列 `rkey`；没有可擦除的列；保留期由部署给（`TIERING_RETENTION_INFRA_AUDIT_RECORDS`）。

## 负责与不负责

- 负责：`RecordsClass`（数据类别）、`Archived`（接收查重时查已封存的记录）、`Reader`（按身份读归档，入口登记）、`History`（含归档的历史列表）、保全与删除批准的操作（名字在 `core`）。
- 不负责：在线的记录怎样写（`intake`）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `archive.go` | `RecordClass`、`Residence`、`RecordsClass`、`StoredRecord`、`Archived`、`historyParams`、`shaped`、`Reader`、`History` |

## 依赖与被依赖

- 依赖：`core`。
- 被依赖：`intake`、`chain`、`reading`；入口 `audit/definition.go`。

## 规则在哪

装配项目协议 03 的保留与法律保全、协议 11；决定 A158。

## 测试

`itest/archive_integration_test.go`（`TestTheAuditRecordsTier`）。
