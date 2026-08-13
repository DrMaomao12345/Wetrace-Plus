package model

import "unicode/utf8"

// ── 微信自带的语音转文字 ────────────────────────────────────────
//
// 用户在微信里点过「转文字」的语音，识别结果会随消息一起存进
// `packed_info_data`（protobuf）。结构实测如下：
//
//	field 1 (varint)  = 4
//	field 2 (varint)  = 5
//	field 5 (message) ┐
//	    field 1 (varint) = 2
//	    field 2 (string) = 转写文本（UTF-8）
//
// 现成的 wxproto.PackedInfo 只定义到 field 4，为了不动生成代码，
// 这里用一个最小的字段遍历器把 field 5 里的字符串取出来。
//
// 覆盖率有限：只有手动点过「转文字」的语音才有（实测约 4.5%），
// 其余仍需本地 Whisper 补齐。

const (
	protoWireVarint  = 0
	protoWireBytes   = 2
	protoWireFixed64 = 1
	protoWireFixed32 = 5
)

// ParseVoiceTranscript 从 packed_info_data 里取出微信自带的语音转写文本。
// 没有则返回空字符串。
func ParseVoiceTranscript(packed []byte) string {
	var text string
	walkProtoFields(packed, func(fieldNo, wire int, payload []byte) bool {
		if fieldNo != 5 || wire != protoWireBytes {
			return true
		}
		walkProtoFields(payload, func(sub, subWire int, subPayload []byte) bool {
			if sub == 2 && subWire == protoWireBytes && utf8.Valid(subPayload) && len(subPayload) > 0 {
				text = string(subPayload)
				return false
			}
			return true
		})
		return text == ""
	})
	return text
}

// walkProtoFields 逐个回调 protobuf 顶层字段（fn 返回 false 表示提前结束）。
// 只处理够用的几种 wire type，遇到无法解析的内容就停下 ——
// 这里只做尽力而为的提取，不追求完整的 protobuf 实现。
func walkProtoFields(b []byte, fn func(fieldNo, wire int, payload []byte) bool) {
	i := 0
	for i < len(b) {
		key, n := readProtoVarint(b, i)
		if n < 0 {
			return
		}
		i = n
		fieldNo, wire := int(key>>3), int(key&7)

		switch wire {
		case protoWireVarint:
			_, n := readProtoVarint(b, i)
			if n < 0 {
				return
			}
			i = n
		case protoWireBytes:
			length, n := readProtoVarint(b, i)
			if n < 0 || length > uint64(len(b)-n) {
				return
			}
			i = n
			end := i + int(length)
			if !fn(fieldNo, wire, b[i:end]) {
				return
			}
			i = end
		case protoWireFixed32:
			i += 4
		case protoWireFixed64:
			i += 8
		default:
			return
		}
	}
}

func readProtoVarint(b []byte, i int) (uint64, int) {
	var result uint64
	var shift uint
	for i < len(b) {
		c := b[i]
		i++
		result |= uint64(c&0x7f) << shift
		if c&0x80 == 0 {
			return result, i
		}
		shift += 7
		if shift > 63 {
			return 0, -1
		}
	}
	return 0, -1
}
