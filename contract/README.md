# infra-audit 的契约

`infra/audit` 的系统面：每个组件经 SDK 把本地审计记录交给审计组件，并在删除本地行之前读取已备份水位。它是被调方发布的契约包，放在组件仓库的 `contract/` 子目录里单独发布（与探针的做法相同），不另建仓库。

依据：装配项目 [be-assembly-standard](https://github.com/brickKit/be-assembly-standard) 的协议 03（审计的转运与回执）、协议 14（库内计数、纪元与分叉点）、D76（线上怎样传），组件协议 [be-protocol](https://github.com/brickKit/be-protocol) 第 08 章（P8.19–P8.23，调用方一侧）。

## 内容

| 位置 | 是什么 |
| --- | --- |
| `proto/be/audit/v3/audit.proto` | `IntakeService.Submit`（交付记录、拿回执）、`IntakeService.Watermark`（已备份水位） |
| `gen/go/be/audit/v3/` | Go 代码，包 `auditv3`；模块 `github.com/brickKit/infra-audit/contract/v3` |
| `python/` | Python 包 `contract-infra-audit`，模块 `be.audit.v3`；`be` 是各契约包共用的命名空间 |

**线上怎样传**（D76）：每条记录是 `be.audit.record` 的规范 JSON 字节（字段见 be-protocol 第 08 章），不带自己的摘要；摘要、来源的产生序号与纪元放在旁边的字段里。

## 审计组件的义务

### `IntakeService.Submit`

1. **准入**：只接受带合格服务证书的连接（P8.5）。记录的来源组件就是证书里的组件，请求里不另写。
2. **逐条校验**：
   - 记录字节按 `be-protocol` 的 `schemas/security/be.audit.record.schema.json` 校验；
   - 核对字节是规范形式、摘要相符、记录的 `id` 等于条目的 `id`；
   - 不合格的答 `RECEIPT_STATUS_INVALID`，并按完整性事件告警，不影响同一批的其他条目。
3. **按身份幂等**：同一来源、同一 `id`：
   - 已存在且摘要相同，返回原来的回执；
   - 已存在但摘要不同，答 `RECEIPT_STATUS_CONFLICT`，并按完整性事件告警（协议 03 第 2 步）。
4. **保存与回执**：
   - 新记录在审计库里拿一个存储序号，带审计库的纪元（协议 14 的“计数带纪元”）；
   - 链摘要按来源计算：`chain = sha256:` + SHA-256（上一条的 `chain` + 换行 + 本条的 `digest`），来源的第一条以空字符串为“上一条”；
   - 写入同步提交之后才回答。回执只给已经持久的记录，否则来源会删掉审计组件其实没保存的行。
5. **发现丢失**：同一来源、同一纪元内的产生序号应当连续。出现空位时告警（协议 03：产生序号用来发现转运丢失的空位），记录照常保存。
6. **回答**：每个条目一个回执，顺序与条目相同。

### `IntakeService.Watermark`

- `epoch`：审计库的当前纪元。
- `seq`：已完成的备份里包含的、这个纪元的最高存储序号。
- `ancestors`：当前历史上的祖先纪元与各自的分叉点。
- `backed_up_in_epoch`：当前纪元里是否已经完成过一次备份。

来源按 P8.22 判断哪些本地行可以删除（向量 `audit-purge`）。审计组件不知道有过成功的备份时，答 `seq = 0`、`backed_up_in_epoch = false`，于是来源一行也不删：宁可多留，不能删掉没备份的。第 5 步的证据存储只留形状，备份完成怎样登记随协议 15 的实现加入。

## 引用

- Go：`go get github.com/brickKit/infra-audit/contract/v3@v3.0.0-alpha.1`（标签 `contract/v3.0.0-alpha.1`），导入 `github.com/brickKit/infra-audit/contract/v3/gen/go/be/audit/v3`。
- Python（uv）：`contract-infra-audit = { git = "https://github.com/brickKit/infra-audit", tag = "contract/v3.0.0-alpha.1", subdirectory = "contract/python" }`。

## 修改与发布

在 `contract/` 里：改 `proto/` 之后运行 `make gen`，与 proto 一起提交；`make check` 核对风格、生成的代码与 Go 模块；发布之后只做加法（`make breaking BASE=contract/<上一个版本>`）。标签 `contract/v3.0.0-alpha.N` 同时发布 Go 与 Python（`3.0.0aN`），与组件自己的版本分开。
