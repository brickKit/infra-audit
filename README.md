# infra-audit

组件 `infra/audit`（审计，兼任证据存储）的仓库。新组件，按终局设计从零开始（D59），版本从 `3.0.0` 起（A110）。

设计依据：装配项目 [be-assembly-standard](https://github.com/brickKit/be-assembly-standard) 的边界卡 `docs/zh/07-components/infra.md`（`infra/audit` 一节）与协议 03、15。

## 在项目里使用

在装配项目里 `brickkit add infra/audit@<版本>`，再按 `BRICKKIT.md` 的“部署前准备”配置。

## 文档

| 问题 | 看哪里 |
| --- | --- |
| 它做什么、怎样部署与配置 | `BRICKKIT.md` |
| 怎样改它 | `AGENTS.md` |
| 契约与审计组件的义务 | `contract/README.md` |

## 开发

见 `AGENTS.md` 的“构建与测试”。
