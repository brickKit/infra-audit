# infra/audit

## 组件定位

审计组件，兼任证据存储（协议 15），做四件事：
- 经系统面接收每个组件转运来的审计记录，记录是规范字节；
- 按来源与记录身份幂等保存，每条给一个回执，回执带审计库的纪元、存储序号与来源链的摘要；
- 告诉来源审计记录备份到了哪里（已备份水位），并在用户面提供查询（3.0.20 起：字段按调用者的授予裁剪，查询本身写一条敏感读取的审计）；
- 定期核对每个来源的链（3.0.20 起）：记录被改、从中间或末尾被删都会被发现，记下断点并以错误级日志 `audit_chain_broken` 告警；
- 证据存储（3.0.15 起）：收下各组件的证据记录（收紧的意图与结论、放宽、对外命令等，`be.evidence.record`），按（记录身份，阶段）只收一次，与审计记录分表；整套恢复时由恢复工具列出。

各组件的 SDK 只在回执被已备份水位覆盖之后才删除本地行，所以审计组件不知道有过备份时，水位答 0，来源一行也不删。备份完成怎样登记，随后续加入。

保留与法律保全（3.0.30 起，装配项目协议 11 与协议 03 的 AU17）：审计记录是一个数据类别 `infra.audit.records`，接收满一年后封存、转层、释放在线行（转层与读回由 SDK 做）；保留期由部署给，没给的从不删除，启动时每次记一条警告 `tiering_retention_pending`；法律保全挡住保留期删除，删除还要一份批准。已离开在线表的记录再到达时答原回执；查询加 `history=true` 读全部历史。

证据存储的规则：组件只能交自己的记录（记录者就是调用者，否则整批拒绝）；同一（记录身份，阶段）再来、摘要相同答“重复”与原来的位置，摘要不同答“冲突”、不保存并记一条完整性事件（`evidence_conflict`）；结论与同一身份的意图的记录者、类别、主体、对象不同也是冲突；回执在提交之后给出，带证据存储自己的纪元与序号（序号跨纪元只增）。整套恢复时证据存储的库不回退。

## 部署前准备

- 数据库：PostgreSQL 16 及以上；schema `infra_audit`，属主角色 `infra_audit_owner`（只供迁移）与运行角色 `infra_audit_rw`，名字来自装配项目的注册表。三者由部署方在第一次启动前建好（be-assembly-standard 的 `make db-init` 按装配进项目的组件配置生成并执行）；表由本组件的迁移建。高可用部署全部写入同步提交；回执只在提交之后给出。
- 部署的服务证书（`SERVICE_TLS_*`）。系统面只接受 mTLS，部署里每个组件都可以调用它的两个系统方法。
- 身份与授权（`IAM_*`、`AUTHZ_GRPC_URL`、`DEPLOYMENT_ID`）：查询受授权保护。
- `AUDIT_GRPC_URL` 指向本组件自己的系统面（`$endpoint:infra/audit:grpc`）：本组件自己的审计记录也走同一个入口。

## 依赖说明

- 必需：数据库、身份成员、授权成员（只为查询）。
- 接收记录不调用任何组件。

## 配置指南

全部键见 `component.yaml` 的 `configSchema`。

| 键 | 说明 |
| --- | --- |
| `DEPLOYMENT_ID`、`IAM_ISSUER`、`IAM_JWKS_URL`、`AUTHZ_GRPC_URL`、`SERVICE_TLS_CERT_FILE`、`SERVICE_TLS_KEY_FILE`、`SERVICE_TLS_CA_FILE`（必填） | 受保护的查询与 mTLS 的系统面要用 |
| `PG_HOST`、`PG_DATABASE`、`PG_USER`、`PG_PASSWORD`、`PG_SCHEMA`（必填） | 自己的数据库，密码是密钥；迁移另用 `PG_OWNER_USER`、`PG_OWNER_PASSWORD` |
| `AUDIT_CHAIN_CHECK_INTERVAL` | 链校验的间隔（3.0.20 起），默认 24 小时；每次读全部记录，记录多时按磁盘读取的代价调长 |
| `AUDIT_EVIDENCE_READERS` | 能列出证据与审计记录的组件（逗号分隔的组件 ID），默认 `ops/recover`（恢复工具的服务身份） |
| `TIERING_RETENTION_INFRA_AUDIT_RECORDS` | 审计记录的保留期 `最短天数/最长天数/依据`（3.0.30 起），按类别、法人所在地区与客户要求在上线时给；不给则从不删除 |
| `TIERING_RESIDENCE_INFRA_AUDIT_RECORDS`、`TIERING_LIST_DAYS`、`TIERING_GRACE`、`TIERING_WINDOW` | 在线保留的天数（默认 365，不短于 `TIERING_LIST_DAYS`）、列表的默认范围、发布后的宽限期、封存的低峰时段（cron），见 be-sdk-go 的转层 |
| `TIERING_STORE_URL` 与 `TIERING_*_ACCESS_KEY`、`TIERING_*_SECRET_KEY` | 冷层的对象存储（异步导出用；审计记录本身走同库的温层），三套凭据：写、读、删 |
| `RUN_PROFILE_*` | 运行档案：释放在线行要一次在发布之后完成的备份记录（`be-ops recover backup-record`）；不配则在线行一直不释放 |

## 契约索引

- 系统面：本仓库 `contract/` 的 `be.audit.v3.IntakeService/Submit` 与 `be.audit.v3.IntakeService/Watermark`；证据存储的 `be.audit.v3.EvidenceService/Append`（任何组件，只交自己的）与 `List`（`AUDIT_EVIDENCE_READERS`），契约 `contract/v3.0.0-alpha.2` 起；审计记录的 `be.audit.v3.RecordService/List`（同样的读者，整套恢复的损失范围），契约 `contract/v3.0.0-alpha.3` 起。模块 `github.com/brickKit/infra-audit/contract/v3`，标签 `contract/v3.…`；审计组件的义务写在 `contract/README.md`。
- 用户面：`GET /api/audit/records?source=&operation=&limit=`，操作是 `infra.audit.record.read`，按收到的时间从新到旧，最多 500 条；加 `history=true` 读在线与归档的全部（时限内完整，否则答 202 与导出的身份）。
- 用户面（SDK 的标准路由，3.0.30 起）：`POST /be/tiering/infra.audit.records/holds`、`…/holds/{id}/lift`（`infra.audit.record.hold`），`POST …/approvals`、`…/approvals/{id}/withdraw`（`infra.audit.record.approve_destruction`），`GET …/exports/{id}`（请求者本人）。
- 系统面：证据的重放 `be.evidence.v1.ReplayService/Replay`（`infra.audit.system.replay`，恢复工具），重新执行恢复点之后的保留期删除。

## 外壳声明

不是外壳。可以被 Go 的基础外壳（`be/go-infra`）承载。
