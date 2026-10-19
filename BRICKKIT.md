# infra/audit

## 组件定位

审计组件，兼任证据存储（协议 15，第 5 步只留了形状），做三件事：
- 经系统面接收每个组件转运来的审计记录，记录是规范字节；
- 按来源与记录身份幂等保存，每条给一个回执，回执带审计库的纪元、存储序号与来源链的摘要；
- 告诉来源审计记录备份到了哪里（已备份水位），并在用户面提供查询。

各组件的 SDK 只在回执被已备份水位覆盖之后才删除本地行，所以审计组件不知道有过备份时，水位答 0，来源一行也不删。备份完成怎样登记、证据存储、保留与法律保全，随协议 15 的实现加入。

## 部署前准备

- 数据库：自己的 schema、运行角色与属主角色（装配项目注册表的 `infra_audit`）。高可用部署全部写入同步提交；回执只在提交之后给出。
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

## 契约索引

- 系统面：本仓库 `contract/` 的 `be.audit.v3.IntakeService/Submit` 与 `be.audit.v3.IntakeService/Watermark`。模块 `github.com/brickKit/infra-audit/contract/v3`，标签 `contract/v3.…`；审计组件的义务写在 `contract/README.md`。
- 用户面：`GET /api/audit/records?source=&operation=&limit=`，操作是 `infra.audit.record.read`，按收到的时间从新到旧，最多 500 条。

## 外壳声明

不是外壳。可以被 Go 的基础外壳（`be/go-infra`）承载。
