# 恢复 / 迁移 / 备份指南

这套配置的“真身”**不在** Clash Verge 里。Verge 只是执行引擎——换电脑、重装、换 Verge 版本，都不影响下面这些“源头”。

本机的出口 IP、ASN、Clash profile UID 写在 **`RECOVERY.local.md`**（已 gitignore），不要提交。

## 配置源头（真身）

1. **`proxy-secret.local`** — 静态 ISP 账密 + 期望身份（`EXPECTED_*`）+ `CONTROLLER_SECRET`。**唯一密钥源、不可再生。**（不在 git 里。）
2. **`scripts/install-clash-iproyal-chain.ps1`** — 重建脚本：读 secret，往当前 profile 写静态节点 + `AI-Static` / `JMS-Bootstrap` 组 + AI 规则。幂等、可重跑、profile UID 变了也能自动找。
   - 等价物：`ai-identity-manager apply --bootstrap <bootstrap 节点名>`。
3. **订阅 URL** — 在 Verge 的 `profiles.yaml` 里；换机要重新导入基础 profile。
4. **自定义规则**（加站走静态 IP）— 在当前订阅 profile 的 merge 文件里（本机路径见 `RECOVERY.local.md`）。

## 换新电脑 / 重装 Verge 的复现步骤

1. 装 Clash Verge Rev，导入基础订阅，设为当前（脚本要往当前 profile 注入，且需要 bootstrap 节点存在）。
2. 拿回本仓库（git clone 或直接拷），把备份的 **`proxy-secret.local` 放回项目根**（它不在 git 里）。
3. 跑 `powershell -ExecutionPolicy Bypass -File .\scripts\install-clash-iproyal-chain.ps1`。
4. 在 Verge 里激活该 profile → 静态链回来了。
5. （可选）把备份的自定义规则粘回新 Verge 的 merge，reload。
6. 验证：`ai-identity-manager verify` 应 GREEN，且与 `proxy-secret.local` 里的 `EXPECTED_*` 一致。

## 静态 ISP 换了新 IP 怎么办

“静态”IP 偶尔会更换。拿到新代理后：

1. 更新 `proxy-secret.local`：`PROXY_HOST` / `PROXY_USER` / `PROXY_PASS` 换成新的，并把 `EXPECTED_EXIT_IP` / `EXPECTED_ASN` 同步成新身份。
2. 重跑 `powershell -ExecutionPolicy Bypass -File .\scripts\install-clash-iproyal-chain.ps1`（用新密钥重写节点）。
3. 在 Verge 里重新激活 profile。
4. `ai-identity-manager verify` 应 GREEN，且与 `EXPECTED_*` 一致。
5. 重跑 `scripts\backup-config.ps1` 刷新 recovery-kit（里面存的是旧密钥）。
6. 同步更新本机 `RECOVERY.local.md` 里的身份摘要。

> 第 2 步会**重写规则增强文件**。所以**自定义规则必须写进 install 脚本的规则列表**（脚本里写 rules 的那段），否则重跑就丢。

## 两层备份（都要留）

- **git 仓库**（本项目）：源码 / 文档 / 脚本 / recipe。**不含密钥**（`.gitignore` 排除 `proxy-secret.local`、`*.local`、`*.local.md`、`*.log`、exe、`.ai-identity/`、`recovery-kit/`、`tester-results/`）。
- **`recovery-kit/`**（`scripts/backup-config.ps1` 生成，已 gitignore）：把 git 不收的东西快照出来——`proxy-secret.local` + Verge 当前 profile 的增强文件（含自定义规则）+ 订阅 URL 清单。把这**整个文件夹**拷到云盘 / U 盘（里面有密钥，别外传）。

> **全量备份 = git 仓库 + recovery-kit 文件夹 + `RECOVERY.local.md`。** 都留，才能在裸机上完整复现。

跑一次备份：

```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\backup-config.ps1
```

## 自定义规则（加站走静态 IP）

维护在当前订阅 profile 的 merge 文件（`%APPDATA%\io.github.clash-verge-rev.clash-verge-rev\profiles\<uid>.yaml`，具体 UID 见 `RECOVERY.local.md`）。
去掉里面 `prepend-rules` 的注释、按格式加行（`- DOMAIN-SUFFIX,某站.com,AI-Static`），在 Verge 里重激活 profile（或 `ai-identity-manager reload`）生效。**工具的 `apply` 不碰它**；`backup-config.ps1` 会把它快照进 recovery-kit。

每多一个站走静态 IP，它就和你的静态身份绑在一起，且加重静态 ISP 负载（偶发 403 / 有量限）。只加你真需要“固定出口”的少数站。
