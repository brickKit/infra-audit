# infra/audit

审计组件的代码。先读 `BRICKKIT.md`；依据是装配项目 be-assembly-standard 的协议 03（转运与回执）、协议 14（纪元与分叉点）、协议 15（证据存储），以及本仓库 `contract/README.md`（审计组件的义务）。

## 代码地图

| 位置 | 内容 |
| --- | --- |
| `member/member.go` | 组件定义、系统面（接收与水位）、用户面（查询） |
| `member/migrations/` | 表：记录、存储计数、纪元历史、来源、备份 |
| `contract/` | 本组件发布的契约，是一个单独的模块 |

## 构建与测试

- `go test ./...`：单元测试，覆盖链摘要与合并安全。
- `go test -tags integration ./member/`：
  - 依赖：本机的 PostgreSQL（环境变量 `AUDIT_IT_PG_PASSWORD`），以及 be-sdk-go 的 `sectest` 测试部署；
  - 覆盖：经 SDK 自己的发送器转运；幂等、冲突、不合格与空位；链摘要；水位；停下再起；受授权保护的查询。
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
