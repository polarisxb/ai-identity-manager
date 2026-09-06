# AI Identity Manager

Clash Verge Rev / Mihomo **之上**的 AI 出口身份控制面：把 Claude / ChatGPT / Codex / OpenAI 等流量钉死在 IPRoyal Static ISP，持续证明出口没变，身份不可用时 **fail closed**（断，而不是换出口）。

这不是代理、VPN、PAC、TUN，也不是 Mihomo 的 fork。数据面永远交给 Clash；本仓库只负责发现 profile、幂等写入增强配置、读运行态证据、判定身份、以及必要的修复/切断。

> 真实出口 IP、ASN、Clash profile UID 只写在本机的 `proxy-secret.local` 和 `RECOVERY.local.md`，不要提交。密钥、日志、`recovery-kit/`、`tester-results/` 同样 gitignore。

## 不变量

1. AI 客户端的最终出口固定为 IPRoyal Static ISP。
2. 不自动切换最终 IP / 国家 / ASN。
3. AI 规则不允许 fallback 到 JMS / DIRECT / 普通 `Proxies`。
4. 静态身份不可用时，AI 流量 fail closed。
5. `JMS-Bootstrap` 只是到达 IPRoyal 入口的前置中转，可以换候选；最终身份不可以换。
6. 工具默认不主动打外部健康检查；`verify` 或显式配置才产生出口流量。
7. 真实写入前备份；`apply --dry-run` 不写文件、不输出 secret；所有输出脱敏。
8. 不污染普通 `Proxies` 大组，不影响微信、国内软件和普通流量。

## 快速开始

需要：Go 1.26+、Clash Verge Rev、一份可用的 JMS（或同类）订阅、以及 IPRoyal Static 账密。

```powershell
git clone https://github.com/polarisxb/ai-identity-manager.git
cd ai-identity-manager

copy proxy-secret.example proxy-secret.local
# 填入 PROXY_* / EXPECTED_* / CONTROLLER_SECRET

copy identity.local.example.yaml identity.local.yaml   # 可选，默认值通常够用

go build -o ai-identity-manager.exe ./cmd/ai-identity-manager
```

把当前 Clash profile 写成静态链（需要 JMS 节点名作为 bootstrap）：

```powershell
.\ai-identity-manager.exe apply --bootstrap <JMS节点名>
# 或直接跑脚本：
powershell -ExecutionPolicy Bypass -File .\scripts\install-clash-iproyal-chain.ps1
```

在 Verge 里重新激活该 profile，然后：

```powershell
.\ai-identity-manager.exe doctor
.\ai-identity-manager.exe verify
.\ai-identity-manager.exe status
```

`verify` 应为 GREEN，且与 `proxy-secret.local` 里的 `EXPECTED_*` 一致。

## 命令

全局参数：`--config`、`--secret`、`--clash-dir`、`--mixed-port`、`--state`、`--json`、`--verbose`。

退出码：`GREEN=0` / `YELLOW=1` / `RED=2`。

| 命令 | 作用 | 外部流量 | 写增强文件 |
| --- | --- | --- | --- |
| `status` | 磁盘链 + 证据窗口 + 最近 verify，判颜色 | 否 | 否 |
| `apply` | 幂等写入当前 profile 的 proxies/groups/rules 增强文件 | 否 | 是 |
| `apply --dry-run` | 预览将改哪些文件，不写盘、不打印 secret | 否 | 否 |
| `verify` | 经 mixed-port 探测出口 IP/ASN/country | **是（手动）** | 否 |
| `watch` | 轮询 `status`；`--once` 跑一轮 | 默认否 | 否 |
| `monitor` | 会话期路由 + 出口身份守护 | 采样出口时是 | 否 |
| `doctor` | 本地诊断（目录、链完整性、controller/pipe、killswitch） | 否 | 否 |
| `reload` | 让 mihomo 重读运行态配置并复核 AI 仍走 `AI-Static` | 否 | 否 |
| `audit` | 打印最近审计事件（脱敏） | 否 | 否 |
| `switch-bootstrap --node <name>` | 只调整 `JMS-Bootstrap` 候选顺序 | 否 | 仅 groups |
| `killswitch status\|apply\|remove` | Windows 防火墙 UDP 阻断（STUN/WebRTC/QUIC） | 否 | 否 |

Monitor / killswitch 用法详见 [`docs/monitor-usage.md`](docs/monitor-usage.md)。

```powershell
.\ai-identity-manager.exe monitor --once --secret proxy-secret.local
.\ai-identity-manager.exe monitor --watch --interval 5s --ip-interval 90s --secret proxy-secret.local
.\ai-identity-manager.exe reload --secret proxy-secret.local
.\ai-identity-manager.exe killswitch apply --secret proxy-secret.local   # 需要管理员
```

## 配置

两份文件，密钥与配置物理隔离：

| 文件 | 是否入库 | 内容 |
| --- | --- | --- |
| [`identity.local.example.yaml`](identity.local.example.yaml) | 是（模板） | Clash 路径、组名、watch/killswitch。复制为 `identity.local.yaml` 后本地改 |
| [`proxy-secret.example`](proxy-secret.example) | 是（模板） | 字段说明 |
| `proxy-secret.local` | **否** | IPRoyal 账密、`EXPECTED_*`、`CONTROLLER_SECRET` |
| `identity.local.yaml` | **否** | 本机覆盖（如果有） |

`CONTROLLER_SECRET` 必须与 Verge 的 `clash-verge.yaml` 里 `secret:` 一致，`monitor` / `reload` 会用它做 Bearer。

## 仓库布局

```
cmd/ai-identity-manager/     主 CLI
internal/cli/                命令解析与输出
internal/identity/           配置、增强文件、状态机、verify、审计
internal/identity/evidence/  日志证据引擎
internal/identity/session/   会话期连接快照与掉线分类
internal/controller/         Mihomo controller（TCP + Windows named pipe）
internal/killswitch/         Windows 防火墙 UDP 阻断
scripts/                     安装静态链、备份、诊断
extensions/block-webrtc/     浏览器 WebRTC 阻断扩展
docs/monitor-usage.md        monitor / killswitch 用法
archive/                     已退役的 identity-keeper 与旧脚本
```

## 脚本

| 脚本 | 用途 |
| --- | --- |
| `scripts/install-clash-iproyal-chain.ps1` | 往当前 Verge profile 写入静态链（幂等，可当 `apply` 的紧急回退） |
| `scripts/backup-config.ps1` | 生成 `recovery-kit/`（密钥 + 当前增强文件快照） |
| `scripts/canary-monitor.ps1` | 金丝雀监控 |
| `scripts/capture-iproyal-*.ps1` | 手动抓隧道失败 / 长连接 |
| `scripts/enable-webrtc-privacy.ps1` / `disable-webrtc-privacy.ps1` | 浏览器 WebRTC 策略 |

换机、换 IP、全量备份步骤见 [`RECOVERY.md`](RECOVERY.md)。本机身份摘要放 `RECOVERY.local.md`（不入库）。

**全量备份 = 本 git 仓库 + `recovery-kit/` 文件夹。** `recovery-kit` 含密钥，只放云盘 / U 盘，不要提交、不要外传。

## 开发

```powershell
go test ./...
go build -o ai-identity-manager.exe ./cmd/ai-identity-manager
```

## 安全

不要提交、不要贴到 issue / 日志里的东西：

- `proxy-secret.local`、`RECOVERY.local.md`、`recovery-kit/`、`*.local`
- `docs/architecture/`、`docs/superpowers/` 等设计文档（本机保留，不入库）
- `tester-results/`（出口 IP 信誉探测）
- `*.log`、`*.exe`、`.ai-identity/`

所有 CLI 输出会脱敏 `PROXY_USER` / `PROXY_PASS` / `CONTROLLER_SECRET` 以及 URL 内嵌凭据。
