# infra/audit

审计组件的代码。用法与边界在 `BRICKKIT.md`，为什么这样设计（术语、数据、规则、实现状态、未决问题）在 [docs/design.md](docs/design.md)；依据是装配项目 be-assembly-standard 的协议 03（转运与回执）、协议 14（纪元与分叉点）、协议 15（证据存储），以及本仓库 `contract/README.md`（审计组件的义务）。

## 代码地图

代码按业务模块分层。**先读这一节，再进目录**：每个代码目录都有自己的 `AGENTS.md`（同目录的 `CLAUDE.md` 让 Claude Code 读这个目录的文件时自动带上它），写这一层负责什么、不负责什么、每个文件、依赖谁、测在哪。

| 目录 | 是什么 |
| --- | --- |
| [`audit/`](audit/AGENTS.md) | 入口：组件定义、配置键与组装（系统面、用户面、链校验的作业、恢复核对都在这里登记，转给业务模块）；不写业务。`main.go` 与外壳只引用它 |
| [`internal/`](internal/AGENTS.md) | 业务：下面的模块各一个包；模块之间的依赖表在它的 `AGENTS.md` |
| [`itest/`](itest/AGENTS.md) | 集成测试，按业务流程分文件；它的 `AGENTS.md` 是测试索引 |
| `audit/migrations/` | 表：记录、存储计数、纪元历史、来源、备份、链的断点、证据 |
| `contract/` | 本组件发布的契约，是一个单独的模块 |
| `docs/design.md` | 设计：术语、数据、规则、命令、实现状态、未决问题 |

业务模块（`internal/` 下）：

| 模块 | 负责 | 拥有的表 |
| --- | --- | --- |
| [`intake`](internal/intake/AGENTS.md) | 接收各组件的审计记录（`IntakeService.Submit`）：核对、只收一次、按来源的链、空位告警；水位（`Watermark`） | `audit_record`、`audit_counter`、`audit_epoch`、`audit_source`、`audit_backup` |
| [`chain`](internal/chain/AGENTS.md) | 链校验（协议 03 的 AU14）：定期重算每个来源的链、对照链头，断了记下 | `audit_chain_break` |
| [`evidence`](internal/evidence/AGENTS.md) | 证据存储（P12.9–P12.13）：`EvidenceService.Append` 的核对与只收一次、`List` | `audit_evidence`、`audit_evidence_counter` |
| [`reading`](internal/reading/AGENTS.md) | 读审计记录：用户面的查询（按授予裁剪字段，查询本身先审计，AU13）；`RecordService.List`（给恢复工具，整套恢复第 9 步） | — |
| [`archive`](internal/archive/AGENTS.md) | 审计记录作为冷热分层的数据类别（A158）：封存、按身份读归档、历史 | — |

共用的：[`core`](internal/core/AGENTS.md)（名字与资源定义、纪元、链摘要、存储位置）。

| 想改 | 从这里开始 | 然后 |
| --- | --- | --- |
| 接收的核对、查重、链 | `internal/intake` 的 `Submit`、`item` | `itest/intake_integration_test.go`；`docs/design.md` 的规则 |
| 链校验（AU14） | `internal/chain` 的 `ChainCheck`、`verifyChain` | `itest/chain_integration_test.go` |
| 查询的字段裁剪与审计（AU13） | `internal/reading` 的 `Query` | `itest/reading_integration_test.go` |
| 证据的类别与核对 | `internal/evidence` 的 `Categories`、`parseEvidence`、`Append` | `itest/evidence_integration_test.go`；类别与协议对照的单元测试 |
| 审计记录的保留与转层 | `internal/archive` 的 `RecordsClass` | `itest/archive_integration_test.go` |

## 构建与测试

- `make test`（`go vet ./... && go test ./...`）：单元测试：链摘要（`internal/core`）、证据的类别与协议一致（`internal/evidence`）、清单与合并安全（`audit/`，查入口与 `internal/` 下全部包）。
- `make test-integration`（`go test -count=1 -tags integration ./itest/`）：真实的 PostgreSQL（环境变量 `AUDIT_IT_PG_PASSWORD`）与 be-sdk-go 的 `sectest`；每个文件测哪条流程，在 [itest/AGENTS.md](itest/AGENTS.md)。
- `brickkit lint`

## 设计取舍

- **每次接收都在存储计数行上加锁**，在锁内连续编号、更新来源的链；回执在提交之后给出。
- **纪元变化时**，旧纪元的最后序号作为分叉点写进纪元历史；纪元历史在库内，随库一起恢复（协议 14 第 2 条）。
- **来源的产生序号出现空位时只告警**，记录照常保存：空位说明转运中丢了记录，补发由来源负责。

## 易错点

- 回执只能在记录持久之后给出：不要把接收拆成“先回答、后写入”，来源凭回执删除本地行。
- 记录要先按 schema、规范形式与摘要校验，再按身份查重：不合格的字节不能凭一个旧的身份拿到“已收到”。
- 链摘要按来源计算，顺序就是存储的顺序；同一个来源的接收必须串行（存储计数行的锁保证这一点）。

## 改代码前自查

- 改了契约（`contract/proto`）：先在 `contract/` 里 `make gen` 并发布 `contract/v…`，再改这里的依赖。
- 改了版本号吗？每次改动都升版本。

<!-- brickkit:managed:begin lang=zh -->
<!-- 由 brickkit 维护（init、add、remove、upgrade、skills update）：这对标记之间的改动会被覆盖 -->

## BrickKit

这是一个 BrickKit 组件：平台只读 `component.yaml`。它依赖的规则：

- `configSchema` 的键就是代码读的环境变量名。不能用保留名：`COMPONENT_ID`、`COMPONENT_VERSION`、`PORT`、`BRICKKIT_SERVED_MEMBERS`、`BRICKKIT_SERVED_MEMBERS_CONFIG`，以及任何 `*_ENDPOINT`。
- 依赖写精确版本。依赖的地址以 `<ID>_ENDPOINT` 注入；缺席的可选依赖根本没有这个变量，读的时候要带兜底。
- `/healthz` 只查本进程，不查依赖。迁移命令用同一个镜像跑，遇到不认识的参数必须直接失败；`local.migrationCommand` 从源码跑同一条迁移，给 `mode: local` 用。
- `BRICKKIT.md` 会随版本进入每个使用它的项目，在那里是脱离仓库单独读的：跟代码一起改，不放相对链接。
- 发版：改 `metadata.version`，提交、推送，`brickkit release`。`brickkit lint` 会检查清单和这些文档——在项目里、在这个目录下跑，只查这个组件（`--all` 查整个项目）。
- 完整规则在 `brickkit-component` 技能里（装了技能的项目或仓库根目录下的 `.claude/skills/brickkit-component/SKILL.md`；`brickkit skills update` 会装上）；参数问 `brickkit <命令> --help`；BrickKit 自己的文档用 `brickkit docs` 看。
<!-- brickkit:managed:end -->
