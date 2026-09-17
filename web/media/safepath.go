package media

import (
	"errors"
	neturl "net/url"
	"path/filepath"
	"strings"
)

var errOutsideRoot = errors.New("路径超出媒体目录")

// SafeJoin 把一个「来自消息内容或请求参数」的相对路径拼到 root 下，
// 并保证结果仍在 root 之内。
//
// 媒体路径是不可信输入：它来自导入文件里的消息 XML，也可以直接由 ?path= 传进来。
// 以前图片分支直接 filepath.Join(root, path)，于是
// `/api/v1/media/image/x?path=../../../../etc/hosts` 能读出机器上任意文件。
//
// 除了词法上的 `..`，也检查符号链接：root 里的一个链接指向外面同样算越界。
func SafeJoin(root, rel string) (string, error) {
	if root == "" {
		return "", errOutsideRoot
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	// 绝对路径和盘符一律按相对处理：去掉开头的分隔符，交给 Join 规整
	rel = strings.TrimLeft(filepath.FromSlash(rel), `/\`)
	if vol := filepath.VolumeName(rel); vol != "" {
		return "", errOutsideRoot
	}
	p := filepath.Join(absRoot, rel)
	if !within(absRoot, p) {
		return "", errOutsideRoot
	}
	// 文件存在时再按真实路径核一遍（符号链接）
	if real, err := filepath.EvalSymlinks(p); err == nil {
		realRoot, err := filepath.EvalSymlinks(absRoot)
		if err != nil {
			realRoot = absRoot
		}
		if !within(realRoot, real) {
			return "", errOutsideRoot
		}
	}
	return p, nil
}

func within(root, p string) bool {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	return r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) && !filepath.IsAbs(r)
}

func parseURL(raw string) (*neturl.URL, error) { return neturl.Parse(raw) }
