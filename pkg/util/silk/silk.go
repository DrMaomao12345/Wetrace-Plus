// Package silk 把微信的 .silk 语音解码成可用的音频。
//
// **为什么有 WAV 这条路**：微信的 silk 里实际有内容到 ~7.8 kHz，
// 而原本唯一的出口是 16 kbps 的 MP3 —— 实测那个 MP3 把 4.4 kHz 以上全削平了，
// 5~8 kHz 掉 48~66 dB（等于归零）。那一段恰好是齿音（s/sh/f/x/z）和说话人
// 身份特征最集中的频段，音色克隆和语音识别都要靠它。
//
// 更荒唐的是这个转码不省空间：同一条 3.7 秒语音，原始 silk 6,781 字节，
// 转出来的 MP3 反而是 7,085 字节。纯亏。
//
// 而 PCM 本来就在手里 —— SilkDecode 返回的就是它，以前直接喂给 LAME 然后丢掉。
// 现在保留一条无损出口给转写和音色克隆用；播放仍可以走 MP3，但码率提到了
// 能听清的水平。
package silk

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/sjzar/go-lame"
	"github.com/sjzar/go-silk"
)

// SampleRate 是微信语音解出来的采样率。go-silk 默认按 24 kHz 解码，
// 下面所有封装都按这个值写头，改动前先确认解码器行为。
const SampleRate = 24000

// Silk2PCM 把 .silk 解成 16 位小端单声道 PCM。
//
// 这是唯一真正的解码步骤，WAV 和 MP3 都是在它之上再封一层。
func Silk2PCM(data []byte) ([]byte, error) {
	sd := silk.SilkInit()
	defer sd.Close()

	pcm := sd.Decode(data)
	if len(pcm) == 0 {
		return nil, fmt.Errorf("silk decode failed")
	}
	return pcm, nil
}

// Silk2WAV 解成无损 WAV（16 位单声道 24 kHz）。
//
// 音色克隆和语音识别都该用这个：主流的 TTS / voice-clone 模型要 22~24 kHz
// 的输入，而 whisper 本来就吃 WAV。体积约是 MP3 的 25 倍，但克隆每人只要
// 几条样本，转写是本机临时文件，都不在乎这点空间。
func Silk2WAV(data []byte) ([]byte, error) {
	pcm, err := Silk2PCM(data)
	if err != nil {
		return nil, err
	}
	return wrapWAV(pcm, SampleRate, 1, 16), nil
}

// Silk2MP3 解成 MP3，给浏览器播放用。
//
// 码率从 16 kbps 提到 64 kbps：16 kbps 连 4.4 kHz 以上都保不住，
// 而且文件比原始 silk 还大。64 kbps 下 3.7 秒约 30 KB，仍然很小，
// 但能把 silk 里真实存在的高频带出来。
func Silk2MP3(data []byte) ([]byte, error) {
	pcm, err := Silk2PCM(data)
	if err != nil {
		return nil, err
	}

	le := lame.Init()
	defer le.Close()

	le.SetInSamplerate(SampleRate)
	le.SetOutSamplerate(SampleRate)
	le.SetNumChannels(1)
	le.SetBitrate(64)
	// IMPORTANT!
	le.InitParams()

	mp3 := le.Encode(pcm)
	if len(mp3) == 0 {
		return nil, fmt.Errorf("mp3 encode failed")
	}
	return mp3, nil
}

// wrapWAV 给裸 PCM 套一个标准 RIFF/WAVE 头。
func wrapWAV(pcm []byte, sampleRate, channels, bitsPerSample int) []byte {
	byteRate := sampleRate * channels * bitsPerSample / 8
	blockAlign := channels * bitsPerSample / 8

	var buf bytes.Buffer
	buf.Grow(44 + len(pcm))
	w16 := func(v int) { _ = binary.Write(&buf, binary.LittleEndian, uint16(v)) }
	w32 := func(v int) { _ = binary.Write(&buf, binary.LittleEndian, uint32(v)) }

	buf.WriteString("RIFF")
	w32(36 + len(pcm)) // 整个文件长度减 8
	buf.WriteString("WAVE")

	buf.WriteString("fmt ")
	w32(16) // PCM 的 fmt 块固定 16 字节
	w16(1)  // 1 = PCM，未压缩
	w16(channels)
	w32(sampleRate)
	w32(byteRate)
	w16(blockAlign)
	w16(bitsPerSample)

	buf.WriteString("data")
	w32(len(pcm))
	buf.Write(pcm)

	return buf.Bytes()
}
