# reading：读审计记录

两种读法：用户面的查询（受授权视图保护，按调用者的授予裁剪字段，查询本身先写一条敏感读取的审计，AU13；`history=true` 读含归档的历史）；`RecordService.List`（给配置的读者，按发生时刻与类别列出、带来源纪元与产生序号、按存储位置分页，整套恢复第 9 步，只答在线部分）。

## 负责与不负责

- 负责：路由 `/api/audit/records` 的 `Query`；`RecordList`（实现 `RecordService`）。
- 不负责：记录怎样写（`intake`）；归档的读法（`archive`）。

## 文件

| 文件 | 内容 |
| --- | --- |
| `reading.go` | `RecordList` 与它的 `List`；`Query` |

## 依赖与被依赖

- 依赖：`archive`（`History`）、`core`。
- 被依赖：入口 `audit/module.go`。

## 规则在哪

装配项目协议 03 的 AU13；协议 15 第 9 步；决定 A158。

## 测试

`itest/reading_integration_test.go`（`recordService`、`query`）。
