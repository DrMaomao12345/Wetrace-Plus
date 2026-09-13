#!/usr/bin/env python3
"""生成一份**完全虚构**的演示导入文件，让人不用自己的聊天记录也能把 Wetrace Plus 跑起来看效果。

    python3 scripts/gen_demo_import.py --out /tmp/wetrace-plus-demo.json
    # 然后打开「导入」页面选这个文件即可

输出的是 Wetrace Plus 标准 JSON v1（见 docs/13-导入格式.md），内容全是编的：
联系人、群、聊天内容、语音、通话都由固定随机种子生成，跑两次结果完全一样。

数据刻意做出层次，好让各个统计功能都有东西可看：
  · 作息 —— 晚上是高峰，凌晨很少；周末比工作日多
  · 逐年增长 —— 每天的消息量随年份缓慢上升，月度趋势图才不是一条直线
  · 有人年中才出现（新面孔），有人聊着聊着就淡了（关系洞察要用）
  · 两个人通话特别多（含视频、未接、拒接），几个人爱发语音
"""
import argparse, json, os, random
from datetime import datetime, timedelta

ME = "wxid_demo0123456789"
ME_NAME = "我（演示账号）"

# (wxid, 显示名, 权重, 起始月, 结束月, 语音倾向, 我方发送占比)
#   月份是相对开始时间的偏移，None = 一直都在
PEERS = [
    ("wxid_demo_linxiaoman",  "林小满", 34, 0,  None, 0.10, 0.48),
    ("wxid_demo_zhoumuyun",   "周慕云", 24, 0,  None, 0.04, 0.46),
    ("wxid_demo_chenjiusi",   "陈九思", 18, 0,  None, 0.03, 0.50),
    ("wxid_demo_xuyaoyao",    "许遥遥", 15, 20, None, 0.06, 0.45),   # 新面孔：第 20 个月才出现
    ("wxid_demo_hejingyi",    "何静仪", 13, 0,  30,   0.02, 0.47),   # 淡出：第 30 个月后没了
    ("wxid_demo_lubotao",     "陆伯涛", 11, 0,  None, 0.12, 0.44),
    ("wxid_demo_fangyuxin",   "方雨欣", 10, 8,  None, 0.05, 0.49),
    ("wxid_demo_qinshuo",     "秦朔",    9, 0,  None, 0.02, 0.52),
    ("wxid_demo_dengwenwen",  "邓文文",  8, 0,  None, 0.07, 0.46),
    ("wxid_demo_gaoyuan",     "高远",    7, 0,  None, 0.01, 0.51),
    ("wxid_demo_shenqinghe",  "沈青禾",  7, 4,  None, 0.09, 0.47),
    ("wxid_demo_yanzhixia",   "颜知夏",  6, 0,  None, 0.03, 0.45),
    ("wxid_demo_moyanbei",    "莫言北",  6, 12, None, 0.02, 0.50),
    ("wxid_demo_suqingyuan",  "苏清源",  5, 0,  24,   0.04, 0.48),   # 中途淡出
    ("wxid_demo_jiangwanting","江晚晴",  5, 0,  None, 0.06, 0.46),
    ("wxid_demo_hanmuzhou",   "韩木舟",  4, 16, None, 0.03, 0.49),
    ("wxid_demo_weilanshan",  "卫兰山",  4, 0,  None, 0.02, 0.53),
    ("wxid_demo_tangyunuo",   "唐雨诺",  3, 6,  None, 0.08, 0.44),
    ("wxid_demo_luchenxi",    "陆辰曦",  3, 0,  None, 0.01, 0.50),
    ("wxid_demo_baiyuting",   "白雨亭",  3, 28, None, 0.05, 0.45),   # 最近一年才出现
    ("wxid_demo_ningzhaoyan", "宁朝颜",  2, 0,  None, 0.02, 0.52),
    ("wxid_demo_xiaheming",   "夏鹤鸣",  2, 0,  14,   0.01, 0.48),
]

GROUPS = [
    ("11111111111@chatroom", "读书会 · 周三",   14, ["林小满", "周慕云", "陈九思", "许遥遥", "颜知夏", "莫言北"]),
    ("22222222222@chatroom", "羽毛球局",        10, ["陆伯涛", "秦朔", "高远", "卫兰山"]),
    ("33333333333@chatroom", "家庭群",           9, ["方雨欣", "邓文文", "江晚晴"]),
    ("44444444444@chatroom", "项目 · 星尘",     12, ["陈九思", "沈青禾", "韩木舟", "陆辰曦", "秦朔"]),
    ("55555555555@chatroom", "周末爬山",         6, ["林小满", "唐雨诺", "白雨亭", "宁朝颜"]),
]

BIZ = [
    ("gh_demo_reading", "读书日报"),
    ("gh_demo_tech",    "科技早知道"),
    ("gh_demo_city",    "城市漫步"),
]

# 词云要好看就得有足够多不重复的词，所以分主题写，别只有寒暄
TEXTS = [
    "今天下班早，一起吃个饭吧", "刚看完那本书，比想象中好看", "明天的会议改到十点了",
    "这个方案我再想想，晚点回你", "路上有点堵车，可能晚二十分钟", "周末去爬山吗，天气预报说晴",
    "照片已经发到你邮箱了", "刚泡了杯咖啡，提提神", "这段代码终于跑通了",
    "昨天睡得太晚，今天有点困", "那家面馆搬到马路对面了", "下周三有空吗，想约个时间",
    "文件收到了，我看完给你反馈", "刚到家，路上下了点小雨", "这首歌挺好听的，推给你",
    "我先睡了，明天再聊", "机票订好了，坐早班机", "今天跑了五公里，累但是舒服",
    "报告改了三版，应该差不多了", "你说得对，是我想复杂了", "楼下新开了家书店，环境不错",
    "会议纪要我整理好发群里", "周末有场展览，要一起去吗", "冰箱里还有水果，记得吃",
    "这个季度的目标定得有点高", "刚看到你发的链接，很有意思", "晚点给你打电话细说",
    "明天记得带伞，说是有雨", "训练计划我调整了一下", "这次旅行拍了好多照片",
    "新买的耳机音质真不错", "项目延期一周，压力小了点", "今天天气真好，适合出门走走",
    "我在图书馆，信号不太好", "菜谱我保存了，周末试试", "谢谢你，这次帮了大忙",
    "今晚要加班，你们先吃", "刚开完会，脑子有点乱", "地铁上人太多了，挤不上去",
    "预算这块还要再核一遍", "数据我重新跑了一遍，结论没变", "设计稿收到了，整体没问题",
    "客户那边的反馈还算正面", "需求又改了，我真的服气", "版本发出去了，先观察一天",
    "线上有个小问题，已经修好了", "文档我补了一节，你看看合不合适", "这次复盘收获挺大",
    "明早的火车，晚上早点休息", "酒店订在会场旁边，走路五分钟", "落地了，一切顺利",
    "海边风好大，但是很舒服", "这家店的红烧肉一绝", "咖啡喝多了，晚上睡不着",
    "健身房今天人少，练得挺爽", "打球去不去，缺一个人", "游泳完全身都轻了",
    "新剧第一集就上头了", "这部电影我看了两遍，值得", "演唱会门票抢到了，太不容易",
    "展览排队排了一个小时", "书看到一半，先放着", "最近在学做菜，进步明显",
    "阳台的花开了，拍给你看", "猫又把杯子打翻了", "楼上装修，吵得头疼",
    "降温了，多穿一件", "空气不错，窗户开着", "今年的秋天来得特别早",
    "钱转过去了，你查收", "快递到了吗，我看显示已签收", "帮你带了份早餐，放桌上了",
    "生日快乐，礼物晚点到", "新年快乐，一切都好", "恭喜恭喜，实至名归",
    "辛苦了，早点休息", "别太累了，身体要紧", "有事随时找我",
    "刚才没看到消息，抱歉", "我大概二十分钟后到", "到楼下了，你下来吧",
    "会员到期了，要不要续", "这个软件比之前那个好用多了", "备份做完了，放心",
    "论文投出去了，等结果", "答辩定在下个月中旬", "实习的事基本敲定了",
    "房子看了三套，都不太满意", "合同条款我标了几处", "保险明天到期，记得续",
    "车保养做完了，换了机油", "驾照终于拿到手了", "停车费涨价了，离谱",
    "计划下个月去趟成都", "行李箱轮子坏了，得换", "签证下来了，松口气",
    "早上跑步遇到日出，太值了", "晚饭随便对付了一下", "今天喝了三杯水，不够",
    "刚才那句话我收回", "你先忙，不着急", "我这边随时都可以",
]
GROUP_TEXTS = [
    "@全体成员 周三晚上八点老地方", "这本书我看完了，强烈推荐", "有人一起吗，缺两个",
    "地点定了，我发个定位", "费用 AA，人均六十", "报名的在下面接龙",
    "明天的场地改到二号馆", "谁有上次那份资料", "下次读《百年孤独》怎么样",
    "照片我传到群相册了", "今天打得真过瘾", "下周同一时间",
    "这个进度我们得赶一赶", "需求文档更新到 v3 了", "周会挪到周四上午",
    "恭喜发财，新年好", "都到齐了吗，准备出发", "山顶风大，多带件衣服",
]
EMOJIS = ["[微笑]", "[加油]", "[偷笑]", "[鼓掌]", "[捂脸]", "[玫瑰]", "[让我看看]", "[破涕为笑]"]

def hour_weight(h):
    # 深夜留一条明显的「睡觉缺口」：真实数据里 1-5 点基本没消息，
    # 年度亮点的「最早/最晚一条」靠的就是这个缺口，铺满 24 小时会让它失去意义
    return [0.8, 0.15, 0.06, 0.04, 0.08, 0.5, 2.5, 6, 10, 12, 13, 14, 12, 11, 12, 13, 14, 16, 20, 26, 30, 28, 18, 5][h]

def hhmmss(sec):
    h, m, s = sec // 3600, (sec % 3600) // 60, sec % 60
    return f"{h:02d}:{m:02d}:{s:02d}" if h else f"{m:02d}:{s:02d}"

def voice_xml(sender, ms):
    return (f'<msg><voicemsg endflag="1" cancelflag="0" forwardflag="0" voiceformat="4" voicelength="{ms}" '
            f'length="{ms // 10}" clientmsgid="demo{random.randint(10**9, 10**10)}" fromusername="{sender}" /></msg>')

def call_xml(text, video=False):
    return ('<voipmsg type="VoIPBubbleMsg"><VoIPBubbleMsg>'
            f'<msg><![CDATA[{text}]]></msg>\n<room_type>{0 if video else 1}</room_type>\n'
            f'<red_dot>false</red_dot>\n<msg_type>100</msg_type>\n<duration>0</duration>\n'
            '</VoIPBubbleMsg></voipmsg>')

def link_xml(title):
    return f'<msg><appmsg><title>{title}</title><type>5</type></appmsg></msg>'

ARTICLES = ["一周书单：把时间留给长句子", "为什么我们越来越难专注", "城市里的十个散步路线",
            "小模型的大用处", "如何把周末过成两天", "关于睡眠的十个误区",
            "这届年轻人的厨房", "开源这一年发生了什么", "行走的博物馆"]

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", default="/tmp/wetrace-plus-demo.json")
    ap.add_argument("--months", type=int, default=48, help="生成多少个月的数据")
    ap.add_argument("--per-day", type=int, default=150, help="平均每天多少条消息（会随年份递增）")
    ap.add_argument("--seed", type=int, default=20260913)
    args = ap.parse_args()
    random.seed(args.seed)

    now = datetime.now()
    now_ts = int(now.timestamp())
    end = now.replace(hour=0, minute=0, second=0, microsecond=0)
    start = (end - timedelta(days=args.months * 30)).replace(day=1)
    total_days = (end - start).days + 1

    name_of = {p[0]: p[1] for p in PEERS}
    wxid_of = {p[1]: p[0] for p in PEERS}
    voice_rate = {p[0]: p[5] for p in PEERS}
    self_rate = {p[0]: p[6] for p in PEERS}
    weights = {p[0]: p[2] for p in PEERS}
    weights.update({g[0]: g[2] for g in GROUPS})
    group_members = {g[0]: g[3] for g in GROUPS}
    talkers = [p[0] for p in PEERS] + [g[0] for g in GROUPS]
    msgs = {t: [] for t in talkers + [b[0] for b in BIZ]}

    seq = 0
    for d in range(total_days):
        day = start + timedelta(days=d)
        month_idx = (day.year - start.year) * 12 + day.month - start.month
        weekend = day.weekday() >= 5
        ramp = 0.72 + 0.55 * (d / max(1, total_days))          # 逐年增长
        day_total = int(args.per_day * ramp * (1.22 if weekend else 1.0) * random.uniform(0.55, 1.45))
        if random.random() < 0.05:
            day_total = int(day_total * 2.4)                    # 偶尔来个爆发日

        active = []
        for p in PEERS:
            if month_idx < p[3] or (p[4] is not None and month_idx > p[4]):
                continue
            active.append(p[0])
        active += [g[0] for g in GROUPS]
        if not active:
            continue

        for t in random.choices(active, weights=[weights[t] for t in active], k=day_total):
            h = random.choices(range(24), weights=[hour_weight(x) for x in range(24)])[0]
            ts = int((day + timedelta(hours=h, minutes=random.randint(0, 59), seconds=random.randint(0, 59))).timestamp())
            if ts > now_ts:
                continue                                        # 不造「未来」的消息
            seq += 1
            is_group = t.endswith("@chatroom")
            mine = random.random() < (0.34 if is_group else self_rate.get(t, 0.47))
            if is_group and not mine:
                sname = random.choice(group_members[t])
                sender, sender_name = wxid_of[sname], sname
            else:
                sender = ME if mine else t
                sender_name = ME_NAME if mine else name_of.get(t, t)

            roll = random.random()
            vr = voice_rate.get(t, 0.03)
            if roll < 0.06:
                mtype, content = 3, '<msg><img aeskey="demo" md5="demo" /></msg>'
            elif roll < 0.06 + vr:
                mtype, content = 34, voice_xml(sender, random.choice([1800, 2400, 3200, 5600, 9000, 15000, 22000]))
            elif roll < 0.13 + vr:
                # 表情：用文本里的 [微笑] 这类标记，而不是 type 47 的表情包 ——
                # 导入数据没有表情包资源，type 47 在聊天页只会渲染成一个报错占位
                mtype, content = 1, random.choice(EMOJIS)
            elif roll < 0.155 + vr:
                mtype, content = 43, '<msg><videomsg length="120000" playlength="12" /></msg>'
            elif roll < 0.18 + vr:
                mtype, content = 49, link_xml(random.choice(ARTICLES))
            else:
                mtype = 1
                content = random.choice(GROUP_TEXTS if is_group and random.random() < 0.35 else TEXTS)

            msgs[t].append({"id": str(seq), "timestamp": ts, "sender_id": sender, "sender_name": sender_name,
                            "is_self": mine, "type": mtype, "content": content})

        # 通话：两位「电话党」打得最多，其他人偶尔打
        for t, rate in (("wxid_demo_linxiaoman", 0.72), ("wxid_demo_zhoumuyun", 0.34),
                        ("wxid_demo_chenjiusi", 0.12), ("wxid_demo_lubotao", 0.09),
                        ("wxid_demo_fangyuxin", 0.07), ("wxid_demo_xuyaoyao", 0.08)):
            p = next(x for x in PEERS if x[0] == t)
            if month_idx < p[3] or (p[4] is not None and month_idx > p[4]) or random.random() > rate:
                continue
            h = random.choices([20, 21, 22, 23, 0, 12, 13, 19], weights=[6, 8, 8, 5, 3, 2, 2, 4])[0]
            dur = random.choice([0, 0, 45, 120, 360, 900, 1800, 3600, 5400, 7800])
            mine = random.random() < 0.5
            video = random.random() < 0.14
            text = (f"通话时长 {hhmmss(dur)}" if dur else
                    random.choice(["已取消", "对方已拒绝", "对方无应答"] if mine else
                                  ["对方已取消", "已拒绝", "已在其它设备接听"]))
            ts = int((day + timedelta(hours=h, minutes=random.randint(0, 59))).timestamp()) + dur
            if ts > now_ts:
                continue
            seq += 1
            msgs[t].append({"id": str(seq), "timestamp": ts, "sender_id": ME if mine else t,
                            "sender_name": ME_NAME if mine else name_of[t], "is_self": mine,
                            "type": 50, "content": call_xml(text, video)})

        # 公众号：每天推送一两篇，只有收没有发
        for bid, bname in BIZ:
            if random.random() > 0.55:
                continue
            ts = int((day + timedelta(hours=random.choice([7, 8, 12, 18, 21]), minutes=random.randint(0, 59))).timestamp())
            if ts > now_ts:
                continue
            seq += 1
            msgs[bid].append({"id": str(seq), "timestamp": ts, "sender_id": bid, "sender_name": bname,
                              "is_self": False, "type": 49, "content": link_xml(random.choice(ARTICLES))})

    conversations = []
    for p in PEERS:
        if msgs[p[0]]:
            conversations.append({"id": p[0], "name": p[1], "type": "direct", "messages": sorted(msgs[p[0]], key=lambda m: m["timestamp"])})
    for g in GROUPS:
        if msgs[g[0]]:
            conversations.append({"id": g[0], "name": g[1], "type": "group", "messages": sorted(msgs[g[0]], key=lambda m: m["timestamp"])})
    for b in BIZ:
        if msgs[b[0]]:
            conversations.append({"id": b[0], "name": b[1], "type": "direct", "messages": sorted(msgs[b[0]], key=lambda m: m["timestamp"])})

    total = sum(len(c["messages"]) for c in conversations)
    out = os.path.expanduser(args.out)
    with open(out, "w", encoding="utf-8") as f:
        json.dump({"format": "wetrace-plus", "version": 1,
                   "account": {"id": ME, "name": ME_NAME},
                   "conversations": conversations}, f, ensure_ascii=False)
    size = os.path.getsize(out) / (1024 * 1024)
    print(f"✓ {out}")
    print(f"  会话 {len(conversations)} 个 · 消息 {total:,} 条 · {size:.1f} MB")
    print(f"  时间范围 {start:%Y-%m-%d} ~ {end:%Y-%m-%d}")

if __name__ == "__main__":
    main()
