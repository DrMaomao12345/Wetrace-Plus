# Windows 端测试清单

> 2026-08-17 · 给 Windows 上的 Claude Code 自动跑
> 对应版本 **v2.5.1**（commit `f79ca0a`）
> macOS 侧已验证通过，本清单用来找 **Windows 特有**的问题。

## 怎么用这份清单

**逐项执行，每项记录「命令 / 实际输出 / 判定」。** 判定只有三种：

- ✅ 通过
- ❌ 失败 —— 贴完整报错
- ⚪ 不适用 —— 命中第七节「已知不支持」，**不要报成 bug**

跑完按第八节的格式汇总。**不要为了让测试通过去改代码**，先如实报告。

---

## 一、先确认这几件事（决定后面全部结果）

```powershell
# 微信版本 —— 最关键的一项
# 在微信里看「设置 → 关于」，或：
Get-ItemProperty "HKCU:\Software\Tencent\WeChat" -ErrorAction SilentlyContinue
Get-ChildItem "$env:APPDATA\Tencent\WeChat" -ErrorAction SilentlyContinue | Select-Object Name
```

| 项 | 记录 |
|---|---|
| Windows 版本 | |
| 微信版本号 | |
| 数据目录 | |
| Go 版本（`go version`） | |
| gcc/mingw（`gcc --version`） | |
| Node 版本（`node -v`） | |

> ⚠️ **微信 3.x 与 4.x 结果完全不同**，见第七节第 1 条。先记下来再往下走。

---

## 二、编译

后端依赖 `go-sqlite3` 等 cgo 包，**Windows 上必须有 gcc（mingw-w64）**。

```powershell
cd <repo>
$env:CGO_ENABLED="1"
$env:GOPROXY="https://goproxy.cn,direct"
$env:GOFLAGS="-mod=mod"
go build -o wetrace.exe .
```

- [ ] **2.1** 后端编译通过，产出 `wetrace.exe`
  - 失败常见原因：没装 mingw-w64、gcc 不在 PATH、CGO_ENABLED=0
- [ ] **2.2** 前端编译通过

```powershell
cd ui
npm ci        # 或 npm install
npm run build # 产出 ui/dist，被 go:embed 打进二进制
```

> 注意顺序：**先 `npm run build` 再 `go build`**，否则二进制里嵌的是旧前端。

---

## 三、启动与密钥

```powershell
.\wetrace.exe
```

- [ ] **3.1** 服务起来了，控制台无 panic
- [ ] **3.2** 浏览器打开 `http://127.0.0.1:5200`（或日志里打印的端口）能出界面
- [ ] **3.3** 「获取密钥」流程能拿到 data key
  - Windows 走的是 DLL 注入（`web/api/handler_wxkey.go`，`//go:build windows`），
    与 macOS 的 lldb hook 完全不同 —— **这条是 Windows 独有路径，重点测**
  - 记录：是否需要微信正在运行？是否需要管理员权限？
- [ ] **3.4** 解密数据库成功，`/api/v1/system/status` 返回 200

```powershell
curl.exe -s http://127.0.0.1:5200/api/v1/system/status
```

---

## 四、基础功能冒烟

每项都用接口验，不要只看界面。把 `<PORT>` 换成实际端口。

- [ ] **4.1** 会话列表非空

```powershell
curl.exe -s "http://127.0.0.1:<PORT>/api/v1/sessions?limit=3&format=json"
```

- [ ] **4.2** 消息能读出来（拿上一步的 talker 填进去）

```powershell
curl.exe -s "http://127.0.0.1:<PORT>/api/v1/messages?talker=<TALKER>&limit=5"
```

- [ ] **4.3** 联系人列表非空 `/api/v1/contacts`
- [ ] **4.4** 年度报告能生成（耗时可能几秒到几十秒）

```powershell
curl.exe -s -X POST "http://127.0.0.1:<PORT>/api/v1/report/annual" `
  -H "Content-Type: application/json" `
  -d '{\"year\":2026,\"default_tz_offset\":480,\"tz_segments\":[],\"exclude_talkers\":[],\"talker\":\"\"}'
```

- [ ] **4.5** 关系星图 `/api/v1/galaxy/graph`
  - 若返回 **409 + `need_profile:true`**：正常，去网页端设出生年月后重试

---

## 五、本轮新增功能（v2.5.1）—— 重点测这三项

### 5.1 语音消息转写字数

```powershell
curl.exe -s "http://127.0.0.1:<PORT>/api/v1/analysis/extras/<TALKER>?year=2026"
```

- [ ] 返回的 `voice` 对象里有这四个**新字段**：
  `transcribed_count`、`transcribed_chars`、`sent_chars`、`recv_chars`
- [ ] `transcribed_chars` == `sent_chars + recv_chars`
- [ ] `transcribed_count` <= `total_count`
- [ ] 界面：聊天页 → 右上「更多」→「会话分析」→ 语音消息卡片
  - [ ] 「总条数」右侧有 ⇄ 图标，**点击切换成「转写字数 我 X · ta Y」**
  - [ ] 切换后下方出现「N / M 条已转写」
  - [ ] 「最长一条」下方标着「我 发于 …」或「ta 发于 …」

> **Windows 上 `transcribed_count` 很可能远小于 macOS**，因为本地 Whisper 补转是
> macOS 侧做的。只要**字段存在、数值自洽**就算通过；数值小不是 bug。
> 若为 0 也可以，说明这台机器没有任何转写数据。

### 5.2 日历热力图

同一个「会话分析」面板，日历热力图卡片：

- [ ] 格子明显比以前大（格距 16px / 格子 14px）
- [ ] **没有消息的那天是灰色方块，不是白色/透明**
  - 这是本轮修的 bug：原代码写的是 `background: var(--muted)`，
    而 `--muted` 存的是 HSL 三元组不是颜色值，属于无效 CSS
- [ ] 明暗主题切换后灰色都看得见（右下角「切换主题」）

### 5.3 年度报告私密模式

网页端 → 年度报告：

- [ ] 顶部工具栏有「私密模式」按钮（眼睛图标）
- [ ] 生成报告后点开，**联系人姓名变成「首字 + •••」**
- [ ] **头像不再加载**（显示首字占位）
- [ ] 「这一年陪伴你最多」那行也被遮住
- [ ] **排除设置面板里的姓名不遮**（那是选人用的，故意不遮）
- [ ] 刷新页面后私密模式**保持开启**（存在 localStorage）
- [ ] 关掉后姓名恢复

用这段在浏览器控制台自查有没有漏网真名：

```javascript
const t = document.body.innerText;
console.log("遮罩样本:", (t.match(/[^\s]•••/g) || []).slice(0, 10));
// 把下面换成你自己认得的几个真实联系人名字
console.log("疑似泄漏:", ["张三","李四"].filter(n => t.includes(n)));
```

---

## 六、回归检查（确认没被这轮改动碰坏）

- [ ] **6.1** 关系洞察页其余卡片正常（互动与回复、通话统计等）
- [ ] **6.2** 图片页能出图
- [ ] **6.3** 词云 / 情感分析 / 对话回放 能打开不报错
- [ ] **6.4** 公众号画像页正常
- [ ] **6.5** 移动端配对能生成二维码（设置 → 移动端配对）

---

## 七、已知不支持 / 预期失败（**命中就标 ⚪，别报 bug**）

1. **微信 3.x 的数据读不出来。**
   store 层是 **V4 独占**的 —— `store/store_impl.go` 里写死 `strategy.NewV4()`，
   仓库里根本没有 V3 策略。所有分析函数按 `Msg_<md5(talker)>` 找表（V4 分表结构），
   V3 是单表结构，找不到表就返回空。
   > 注意有个落差：**密钥提取那边有 `v3_windows.go`，能拿到 v3 的密钥，
   > 但拿到了也读不了数据。** 如果这台机器是微信 3.x，第四节起会大面积返回空，
   > 这是**预期行为**，请在报告里注明微信版本，不要逐条报 bug。

2. **加密图片可能打不开。**
   图片密钥推导（`internal/wxkey/imagekey.go`）靠读微信埋点文件里的 uin，
   那个路径是 macOS 的（`app_data/*/kvcomm/key_<uin>_*.statistic`）。
   Windows 上大概率推导失败 —— `main.go:88` 里是 `if err == nil` 静默跳过，
   **不会崩，只是加密图看不了**。可用 `.env` 的 `IMAGE_KEY` / `XOR_KEY` 手动兜底。

3. **语音转写数量少或为 0。** 见 5.1 的说明。

4. **`transcribed_*` 全为 0 但 `total_count > 0`** —— 只要字段存在且自洽即通过。

---

## 八、报告格式

```markdown
## 环境
Windows 版本 / 微信版本 / Go / gcc / Node

## 结果汇总
| 编号 | 项目 | 判定 | 备注 |
|---|---|---|---|
| 2.1 | 后端编译 | ✅ | |
| ... | | | |

## 失败项详情
### <编号> <项目>
- 命令：
- 实际输出：
- 期望：
- 初步判断：

## 环境相关（⚪）
（列出命中第七节的项，并注明命中的是哪一条）
```

**特别注意**：如果第二节编译就过不去，**先只报编译问题，不要继续往下猜**。
Windows 的 cgo 环境是最容易出问题的一环，编译不过后面全是无效结论。
