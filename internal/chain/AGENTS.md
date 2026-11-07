# chain：链校验（AU14）

单例作业按 `AUDIT_CHAIN_CHECK_INTERVAL` 逐个来源按存储顺序核对每条记录的字节与摘要、重算链、对照链头；断了就写 `audit_chain_break` 并记错误级日志 `audit_chain_broken`。从每个来源第一条在线记录接着算（来源的记录全部已释放时只对照链头，A158）。

## 负责与不负责

- 负责：表 `audit_chain_break`；作业名 `JobChainCheck`、`ChainCheck`。
- 不负责：链怎样产生（`intake`）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `chain.go` | `JobChainCheck`、`chainBreak`、`head`、`ChainCheck`、`verifyChain`、`lastArchivedChain`、`releasedBefore` |

## 依赖与被依赖

- 依赖：`archive`、`core`。
- 被依赖：入口 `audit/module.go` 的 `Jobs`。

## 规则在哪

装配项目协议 03 的 AU14；决定 A158。

## 测试

`itest/chain_integration_test.go`（`chainCheck`：改了的记录、拿掉最后一条、拿掉第一条、拿掉中间一条）。
