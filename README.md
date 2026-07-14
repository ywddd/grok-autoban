# Grok Auto Ban

CLIProxyAPI（CPA）插件：检测 Grok/xAI 账号的权限拒绝（403）和认证失败（401），并自动将对应账号移出调度池。

插件 ID / 仓库：`grok-autoban`（原 `grok-429-autoban`，v0.1.6 起更名）。

> 从 `v0.1.5` 起，本插件**不再处理**免费额度耗尽（429 `subscription:free-usage-exhausted`）。
> 该项请使用 CPA Manager Plus `v1.11.0+` 的 **Provider 额度冷却**。

## 生效范围

只对 Provider 为 `xai`、`x-ai` 或 `grok` 的账号生效。其他 Provider 的 401/403 不会触发禁用。

## 检测条件

只有以下条件同时满足才会禁用账号：

1. Provider 是 `xai`、`x-ai` 或 `grok`
2. 命中下面任一错误：

| HTTP | 匹配条件 | 恢复方式 |
| --- | --- | --- |
| `403` | JSON `code` = `permission-denied` | 手动解禁 |
| `401` | 任意 Grok/xAI 认证失败响应 | 手动解禁 |

普通 403、其他错误码、Cloudflare 信息和 `X-Should-Retry` 都不会触发禁用。401 只要求状态码，因为认证失败 body 格式可能不一致。429 免费额度耗尽不会触发本插件。

## 恢复规则

`permission-denied`（403）和认证失败（401）通常表示凭证权限或登录状态有问题，不是临时额度窗口。插件会长期禁用该账号，直到你调用管理接口解禁，或在状态页手动解禁。

升级到 `v0.1.5` 后，历史中由本插件记录的 `free-usage-exhausted` 禁用会在启动时清理，并尽量重新启用对应账号，交给 Manager Plus 管理后续 429。

## CPA 配置

```yaml
plugins:
  enabled: true
  configs:
    grok-autoban:
      enabled: true
      priority: 100
      # fallback_hours 仅为兼容字段，v0.1.5 起不再使用
      fallback_hours: 24
      persist_state: true
      state_file: plugins/data/grok-autoban/bans.json
      log_matches: true
```

`state_file` 留空时只保存在内存。建议设置一个可写路径，这样 CPA 重启后未解禁的 401/403 禁用仍然保留。

## 安装

Windows amd64：

```text
plugins/windows/amd64/grok-autoban.dll
```

也可以直接放在 CPA 的 `plugins/` 目录。文件名去掉扩展名就是插件 ID。

安装后重启 CPA，或在 CPAMP 插件管理页面刷新。插件商店来源使用本项目的 `registry.json`。

## 管理接口

以下接口需要 CPA Management Key：

```text
GET  /v0/management/plugins/grok-autoban/bans
POST /v0/management/plugins/grok-autoban/unban
POST /v0/management/plugins/grok-autoban/unban-all
GET  /v0/resource/plugins/grok-autoban/status
```

状态页默认每页 20 条，可切换 50 / 100。

单个解禁请求：

```json
{"auth_id":"你的 CPA auth_id"}
```

接口只返回账号 ID、禁用时间和恢复时间，不返回 token、Cookie 或完整认证文件。

## 本地构建

要求：

- Go 1.21 或更高版本
- CGO
- Windows 使用 MinGW-w64/LLVM-MinGW，Linux 使用 gcc，macOS 使用 clang

Windows：

```powershell
.\build.ps1
```

Linux/macOS：

```bash
./build.sh
```

测试：

```text
go test ./...
go test -race ./...
go vet ./...
```
