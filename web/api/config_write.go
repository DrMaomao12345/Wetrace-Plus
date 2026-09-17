package api

import (
	"strings"

	"github.com/spf13/viper"
)

// saveConfig 是写 .env 的唯一入口。
//
// viper 的 dotenv 编码器就是 fmt.Sprintf("%v=%v\n", key, value)，不做任何转义。
// 设置接口的字段（AI 模型名、语音模型路径、备份路径……）只要带一个换行，
// 就能在 .env 里凭空多出一整行配置 —— 比如 LISTEN_ADDR=0.0.0.0:5200，
// 下次启动时整个服务就暴露到局域网了。这里在落盘前把所有字符串值里的换行去掉。
func saveConfig() error {
	sanitizeConfigValues()
	return viper.WriteConfig()
}

var lineBreaks = strings.NewReplacer("\r", "", "\n", "", " ", "", " ", "")

func sanitizeConfigValues() {
	for _, k := range viper.AllKeys() {
		if s, ok := viper.Get(k).(string); ok && strings.ContainsAny(s, "\r\n  ") {
			viper.Set(k, lineBreaks.Replace(s))
		}
	}
}
