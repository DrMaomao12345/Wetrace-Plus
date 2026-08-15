// Package envfile 按字面值读取 .env。
//
// viper 读 .env 用的是 gotenv，它会对未加引号的值做 shell 风格的 `$` 变量展开。
// 对配置来说这是个陷阱：任何含 `$` 的值都会被悄悄改写。最典型的是 bcrypt 密码
// 哈希（形如 `$2a$10$...`）—— 写进文件的和读出来的不是同一个值，比对必然失败，
// 而且不报任何错。API Key 之类含 `$` 的值同样会中招。
//
// 这里自己解析一遍文件，只做「去引号」，不做任何展开，供调用方覆盖回 viper。
package envfile

import (
	"bufio"
	"os"
	"strings"
)

// Load 按字面值解析 .env，返回键值对。文件不存在时返回空 map。
func Load(path string) map[string]string {
	out := map[string]string{}

	f, err := os.Open(path)
	if err != nil {
		return out
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // 有些值（如整段 JSON）会很长
	for sc.Scan() {
		key, val, ok := parseLine(sc.Text())
		if ok {
			out[key] = val
		}
	}
	return out
}

// parseLine 解析一行 `KEY=VALUE`；注释、空行、无等号的行返回 ok=false。
func parseLine(line string) (key, value string, ok bool) {
	s := strings.TrimSpace(line)
	if s == "" || strings.HasPrefix(s, "#") {
		return "", "", false
	}
	s = strings.TrimPrefix(s, "export ")

	i := strings.Index(s, "=")
	if i <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(s[:i])
	value = strings.TrimSpace(s[i+1:])

	// 去掉成对的引号；引号内的内容原样保留，不做展开也不处理转义
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return key, value, true
}
