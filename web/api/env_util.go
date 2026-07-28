package api

import (
	"fmt"
	"os"
	"strings"
)

// updateEnv 更新 .env 文件中的配置项（平台无关，供各平台的密钥/解密处理器共用）。
func updateEnv(updates map[string]string) error {
	envPath := ".env"
	content, err := os.ReadFile(envPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	lines := strings.Split(string(content), "\n")
	newLines := make([]string, 0, len(lines))
	updated := make(map[string]bool)

	for _, line := range lines {
		trimmedLine := strings.TrimRight(line, "\r")
		keyFound := false
		for k, v := range updates {
			// 匹配 KEY= 开头的行
			if strings.HasPrefix(trimmedLine, k+"=") {
				newLines = append(newLines, fmt.Sprintf("%s=%s", k, v))
				updated[k] = true
				keyFound = true
				break
			}
		}
		if !keyFound {
			newLines = append(newLines, trimmedLine)
		}
	}

	// 添加文件中不存在的新 key
	for k, v := range updates {
		if !updated[k] {
			newLines = append(newLines, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// 重新组合并写回文件
	output := strings.Join(newLines, "\n")
	return os.WriteFile(envPath, []byte(output), 0600)
}
