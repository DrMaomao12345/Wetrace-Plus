//go:build windows

// Package winkey 在 Windows 上直接读取微信进程内存提取数据库密钥。
//
// 为什么需要它：原本 Windows 走的是 DLL 注入（`wxkey` + `key/pkg/dllloader`），
// 依赖一个叫 `wx_key.dll` 的第三方二进制。那个 DLL 有三个问题：
//
//  1. **从未进过版本库** —— `.gitignore` 从初次提交起就排除 `*.dll`。
//     它早期靠 `//go:embed wx_key.dll` 在编译时烤进 exe（DLL 只存在于开发机本地），
//     v1.5.2 为了让缺少该文件的机器能编译，把 embed 去掉改成运行时磁盘查找 ——
//     从此二进制不再自包含，换台机器就找不到它。
//  2. **上游已消失**：`0xlane/wx_key` 已 404，`afumu/wetrace` 已因法律原因下架。
//  3. 注入前会**强杀并重启微信**，代价高且容易失败。
//
// 本包改用 chatlog 的做法：`OpenProcess` + `PROCESS_VM_READ` 只读扫描微信进程内存，
// 不注入、不写目标进程、**不需要重启微信**，也不依赖任何外部二进制。
//
// 与 macOS 侧的 `internal/cl/mackey` 是对称关系：同样先找在线进程，
// 提取后用数据目录里的加密库校验一次，再返回。
//
// 运行时前提：
//   - 微信正在运行且已登录（未登录时密钥不在内存里）
//   - 当前用户有权限读取该进程内存（同用户进程通常无需管理员）
package winkey

import (
	"context"
	"encoding/hex"
	"fmt"

	"github.com/afumu/wetrace/internal/cl/wechat/decrypt"
	"github.com/afumu/wetrace/internal/cl/wechat/key"
	"github.com/afumu/wetrace/internal/cl/wechat/model"
	"github.com/afumu/wetrace/internal/cl/wechat/process"
)

// Result 与 mackey.Result 保持一致的形状，便于两端调用方对称处理。
type Result struct {
	DataKey  string // 数据库解密密钥（hex）
	ImageKey string // 图片密钥（本实现不提取，保留字段以对齐 macOS 侧）
	DataDir  string // 微信账号数据目录，正好是 decrypt.RunTask 需要的 srcDir
	Version  int    // 微信大版本（3 / 4）
	PID      uint32
}

// ExtractWeChatKey 检测已登录的微信进程并从其内存中提取数据库密钥。
func ExtractWeChatKey(ctx context.Context) (*Result, error) {
	proc, err := detectOnlineWeChat()
	if err != nil {
		return nil, err
	}

	extractor, err := key.NewExtractor(model.PlatformWindows, proc.Version)
	if err != nil {
		return nil, fmt.Errorf("不支持的微信版本 v%d: %w", proc.Version, err)
	}

	// 先装校验器：提取器内部用它判定候选 key 是否真的能解开数据库，
	// 没有校验器就只能靠特征猜，误报率高。
	if v, verr := decrypt.NewValidator(model.PlatformWindows, proc.Version, proc.DataDir); verr == nil {
		extractor.SetValidate(v)
	}

	dataKey, imgKey, err := extractor.Extract(ctx, proc)
	if err != nil {
		return nil, fmt.Errorf("从微信进程提取密钥失败: %w", err)
	}
	if dataKey == "" {
		return nil, fmt.Errorf("未能在微信进程内存中找到数据库密钥；请确认微信已登录后重试")
	}

	// 再校验一次，挡掉万一的误命中。校验器建不起来（比如数据目录还没生成）时跳过。
	if v, verr := decrypt.NewValidator(model.PlatformWindows, proc.Version, proc.DataDir); verr == nil {
		if kb, derr := hex.DecodeString(dataKey); derr == nil && !v.Validate(kb) {
			return nil, fmt.Errorf("提取到的密钥未通过校验，请重启微信后重试")
		}
	}

	return &Result{
		DataKey:  dataKey,
		ImageKey: imgKey,
		DataDir:  proc.DataDir,
		Version:  proc.Version,
		PID:      proc.PID,
	}, nil
}

// detectOnlineWeChat 找一个在线且已定位到数据目录的微信进程。
func detectOnlineWeChat() (*model.Process, error) {
	det := process.NewDetector(model.PlatformWindows)
	procs, err := det.FindProcesses()
	if err != nil {
		return nil, fmt.Errorf("检测微信进程失败: %w", err)
	}
	for _, p := range procs {
		if p != nil && p.Status == model.StatusOnline && p.DataDir != "" {
			return p, nil
		}
	}
	return nil, fmt.Errorf("未找到已登录的微信进程；请确认微信正在运行并已登录")
}
