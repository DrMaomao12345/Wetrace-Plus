package tts

// Transcriber 语音转文字通用接口
type Transcriber interface {
	Transcribe(audioData []byte, filename string) (string, error)
}
