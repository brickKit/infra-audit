# itest/：集成测试与测试索引

审计组件对着真实的 PostgreSQL、经它真实的接口（来源组件的记录经 SDK 自己的发送器走 mTLS；用户面的查询带 be-sdk-go `sectest` 的令牌，受授权成员保护）的测试。构建标签 `integration`；一条业务流程一个文件。

## 怎样跑

- `go test -count=1 -tags integration ./itest/`，约 23 秒（链校验一步等作业几轮）。
- 环境变量：`AUDIT_IT_PG_PASSWORD`（be-postgres 的 postgres 超级用户口令，装配项目 `.env` 的 `POSTGRES_PASSWORD`）；库 `brickkit_test_db`；schema `audit_it` 每次重建。
- 端口 28206、9206 要空着，被占时测试直接失败。
- 三个测试：`TestAuditEndToEnd`（按下表的顺序跑各步，后面的步用前面送的记录，一步不能单独跑）、`TestTheEvidenceStore`、`TestTheAuditRecordsTier`（各自独立，可以 `-run` 单独跑）。

## 文件

| 文件 | 测试与步 | 测的流程 | 被测的模块 |
| --- | --- | --- | --- |
| `intake_integration_test.go` | `TestAuditEndToEnd`：`receive`、`refusals`、`restart` | 三条记录按序收下、存储序号 1–3、链；同样再送答原来的回执；另一个摘要、不是记录的字节、身份不符；产生序号的空位告警、照常保存；没备份过的不能删；审计组件停下再起，来源补送，早先的记录保持回执 | `intake` |
| `reading_integration_test.go` | `TestAuditEndToEnd`：`recordService`、`query` | `RecordService.List` 只给读者，按时刻、类别、位置分页；查询受授权保护、查询本身先审计、没有授予看不见、Bob 的字段按授予裁剪（AU13） | `reading` |
| `chain_integration_test.go` | `TestAuditEndToEnd`：`chainCheck` | AU14：改了的记录、拿掉最后一条（只有链头看得出）、拿掉第一条、拿掉中间一条 | `chain` |
| `evidence_integration_test.go` | `TestTheEvidenceStore` | 证据：重复、冲突、结论对意图、冒名的记录者、读者、顺序与分页 | `evidence` |
| `archive_integration_test.go` | `TestTheAuditRecordsTier` | 审计记录的封存、释放、按身份读、历史、链校验在释放之后 | `archive`、`chain` |

`TestAuditEndToEnd` 共用的：

| 文件 | 内容 |
| --- | --- |
| `endtoend_integration_test.go` | `TestAuditEndToEnd`（各步的顺序）；`wire`；`fixture`（运行中的成员、启停、来源的发送器、步与步之间传的记录）；`send`；`start`；`portFree` |

## 加测试

新流程加一个文件；属于端到端的加一步并在 `TestAuditEndToEnd` 里排进顺序，独立的写自己的测试函数；在上表加一行；测试不为了通过而改。
