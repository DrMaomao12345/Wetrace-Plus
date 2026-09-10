# Wetrace Plus

Wetrace Plus 是一个**只做导入与分析**的本地微信聊天记录工作台。

它接收第三方工具已经导出的 JSON、CSV 或 ZIP 文件，将不同格式统一成分析模型，再提供聊天浏览、搜索、年度报告、联系人统计、词云、关系洞察和可选的 AI 分析。

> 隐私边界：Wetrace Plus 不连接微信，不读取微信进程，不提取密钥，也不读取或解密微信数据库。原始导出文件由用户自行通过其他工具获得。

## 已支持的导入格式

| 来源 | 格式 | 识别依据 |
| --- | --- | --- |
| [chatlog-keeper](https://github.com/labazhou2024/chatlog-keeper) | `wechat_messages.json` | `ts`、`conversation_id`、`sender_wxid`、`is_self`、`msg_type`、`account_id` 消息数组 |
| [chatlog](https://github.com/sjzar/chatlog) | JSON | `seq`、`time`、`talker`、`sender`、`isSelf`、`type`、`content` 消息数组 |
| chatlog | CSV | `Time,SenderName,Sender,TalkerName,Talker,Content` |
| MemoTrace / WeChatMsg | 会话 CSV | `消息ID,类型,发送人,时间,内容,...` |
| MemoTrace / WeChatMsg | 全量 CSV | `localId,TalkerId,Type,SubType,IsSender,CreateTime,StrContent,...` |
| Wetrace Plus | 标准 JSON | `format: "wetrace-plus"`、`version: 1` |
| 以上任一格式 | ZIP | 自动扫描压缩包内的 `.json` 与 `.csv` |

详细字段、方向判断规则和标准 JSON 示例见 [导入格式说明](docs/13-导入格式.md)。

## 使用方式

1. 用你信任的导出工具生成 JSON、CSV 或 ZIP。
2. 打开 Wetrace Plus 的「导入」页面并选择文件。
3. 对缺少发送方向的旧式 CSV，填写自己的导出昵称。
4. 导入完成后直接进入聊天、报告或分析页面。

重复文件可以安全再次导入：系统按消息指纹去重，并保留每次导入的文件哈希、格式、消息数和告警记录。

## 分析能力

- 聊天浏览、全局搜索和多格式再次导出
- 年度报告、联系人趋势、收发比例、活跃时段、消息类型与通话统计
- 词云、关系星图、陪伴时间轴、情感分析与联系提醒
- 对话摘要、待办与关键信息提取、模拟聊天等可选 AI 功能
- 本地 Whisper 或兼容接口的语音转文字（需要导入包包含可用音频）
- 密码保护、移动端只读访问、备份和监控告警

导入文件不含图片、语音、视频等附件时，相关页面只显示占位或元数据；Wetrace Plus 不会回头访问微信目录补取文件。

## 本地运行

要求 Go 1.25+、Node.js 20+，以及用于编译 `go-sqlite3` 的 C 编译器。

```bash
git clone https://github.com/DrMaomao12345/Wetrace-Plus.git
cd Wetrace-Plus/ui
npm ci
npm run build
cd ..
CGO_ENABLED=1 go run .
```

默认打开 `http://127.0.0.1:5200`。分析库保存在 `WORK_DIR`（默认 `data/`），配置见 [.env.example](.env.example)。

## 开发导入适配器

适配层位于 `internal/importer/`。所有适配器只负责把来源字段转换成统一 `Message`，持久化、事务、去重和导入历史由共享写入器处理。新增适配器时请同时添加最小真实样例测试，并更新兼容矩阵。

## 数据与联网说明

- 导入、索引和常规分析都在本地完成。
- 上传的临时文件在一次导入结束后删除，不额外保存副本。
- 只有用户主动启用在线 AI、语音识别、Webhook 或机器人功能时，相关数据才会发送到用户指定的服务。
- 聊天记录属于高度敏感数据，请只处理你有权使用的内容，并妥善保护导出文件、分析库和备份。

## 项目来源与致谢

Wetrace Plus 从 Wetrace Pro 分出，并继承了 [afumu/wetrace](https://github.com/afumu/wetrace) 的分析框架。Plus 已移除原项目中的微信进程访问、密钥获取、数据库解密和源目录同步能力。

感谢 [chatlog](https://github.com/sjzar/chatlog)、MemoTrace / WeChatMsg 社区导出格式，以及 [go-ego/gse](https://github.com/go-ego/gse)、[Recharts](https://recharts.org/) 和 [Gin](https://github.com/gin-gonic/gin) 等开源项目。

本仓库沿用现有 [CC BY-NC-SA 4.0](LICENSE) 许可；第三方依赖分别遵循其自身许可证。
