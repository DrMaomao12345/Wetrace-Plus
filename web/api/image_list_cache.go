package api

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/store/repo"
)

// imageListCache 缓存图库的图片清单。
//
// 枚举一次要把 3 万多条图片消息的正文解压出来抠 md5，约 1.7 秒；
// 而这份清单只在消息库变化时才会变，翻页、切时间范围都可以复用。
// 按数据指纹作键，库一变旧条目自然失效。
type imageListCache struct {
	mu      sync.Mutex
	version string
	entries map[string][]*repo.ImageRef
}

func newImageListCache() *imageListCache {
	return &imageListCache{entries: map[string][]*repo.ImageRef{}}
}

func (c *imageListCache) get(version, key string) []*repo.ImageRef {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.version != version {
		return nil
	}
	return c.entries[key]
}

func (c *imageListCache) put(version, key string, refs []*repo.ImageRef) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.version != version {
		c.version = version
		c.entries = map[string][]*repo.ImageRef{}
	}
	c.entries[key] = refs
}

// listImagesCached 取图片清单，命中缓存直接返回。
func (a *API) listImagesCached(ctx context.Context, talker string, start, end time.Time) []*repo.ImageRef {
	if a.ImageList == nil {
		return a.Store.ListImageMessages(ctx, talker, start, end)
	}
	// 只用消息库指纹：图片清单与语音转写无关，不必跟着转写数变动
	version := a.Store.GetDataVersion()
	key := fmt.Sprintf("%s|%d|%d", talker, start.Unix(), end.Unix())

	if refs := a.ImageList.get(version, key); refs != nil {
		return refs
	}
	refs := a.Store.ListImageMessages(ctx, talker, start, end)
	a.ImageList.put(version, key, refs)
	return refs
}
