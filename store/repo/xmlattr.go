package repo

import (
	"strconv"
	"strings"
)

// 语音 / 通话这些消息体是 XML，但只需要取一两个属性，
// 走完整 XML 解析太重（几十万条消息要过一遍），这里按属性名直接抠。

// attrString 取 XML 里 name="value" 的 value
func attrString(content, name string) string {
	key := name + "=\""
	i := strings.Index(content, key)
	if i < 0 {
		return ""
	}
	rest := content[i+len(key):]
	j := strings.IndexByte(rest, '"')
	if j < 0 {
		return ""
	}
	return rest[:j]
}

// attrInt 取 XML 属性并转成整数，取不到返回 0
func attrInt(content, name string) int {
	v := attrString(content, name)
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}
