package api

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestContactMemoryStorePersistsCompoundKeys(t *testing.T) {
	dataDir := t.TempDir()
	store, err := NewContactMemoryStore(dataDir)
	if err != nil {
		t.Fatalf("创建 store 失败: %v", err)
	}
	fixedNow := time.Date(2026, 9, 9, 13, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	store.now = func() time.Time { return fixedNow }

	for _, memory := range []ContactMemory{
		{AccountID: " account-a ", Talker: " same-wxid ", TargetName: "甲账号联系人", Profile: ContactMemoryProfile{Summary: "来自账号甲"}},
		{AccountID: "account-b", Talker: "same-wxid", TargetName: "乙账号联系人", Profile: ContactMemoryProfile{Summary: "来自账号乙"}},
		{AccountID: "account-a", Talker: "another", TargetName: "另一个联系人", Profile: ContactMemoryProfile{Summary: "另一份记忆"}},
	} {
		if err := store.Put(memory); err != nil {
			t.Fatalf("Put(%q/%q) 失败: %v", memory.AccountID, memory.Talker, err)
		}
	}

	path := filepath.Join(dataDir, contactMemoryFileName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("记忆文件不存在: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("文件权限 = %04o, want 0600", got)
	}

	reloaded, err := NewContactMemoryStore(dataDir)
	if err != nil {
		t.Fatalf("重载 store 失败: %v", err)
	}
	a, ok := reloaded.Get("account-a", "same-wxid")
	if !ok || a.Profile.Summary != "来自账号甲" {
		t.Fatalf("账号甲复合键读取错误: ok=%v memory=%+v", ok, a)
	}
	b, ok := reloaded.Get("account-b", "same-wxid")
	if !ok || b.Profile.Summary != "来自账号乙" {
		t.Fatalf("账号乙复合键读取错误: ok=%v memory=%+v", ok, b)
	}
	if a.SchemaVersion != ContactMemorySchemaVersion || !a.CreatedAt.Equal(fixedNow) || !a.UpdatedAt.Equal(fixedNow) {
		t.Fatalf("store 元数据不正确: %+v", a)
	}

	accountA := reloaded.List("account-a")
	if len(accountA) != 2 || accountA[0].Talker != "another" || accountA[1].Talker != "same-wxid" {
		t.Fatalf("List 应只返回账号甲并按 talker 排序: %+v", accountA)
	}
	all := reloaded.ListAll()
	if len(all) != 3 || all[0].AccountID != "account-a" || all[2].AccountID != "account-b" {
		t.Fatalf("ListAll 排序或数量错误: %+v", all)
	}

	leftovers, err := filepath.Glob(filepath.Join(dataDir, ".ai_contact_memories-*.tmp"))
	if err != nil {
		t.Fatalf("检查临时文件失败: %v", err)
	}
	if len(leftovers) != 0 {
		t.Fatalf("原子保存遗留临时文件: %v", leftovers)
	}
}

func TestContactMemoryStorePutPreservesCreatedAt(t *testing.T) {
	store, err := NewContactMemoryStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	updatedAt := createdAt.Add(time.Hour)
	store.now = func() time.Time { return createdAt }
	if err := store.Put(ContactMemory{AccountID: "acc", Talker: "wxid", Profile: ContactMemoryProfile{Summary: "第一版"}}); err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return updatedAt }
	if err := store.Put(ContactMemory{
		AccountID: "acc",
		Talker:    "wxid",
		CreatedAt: time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC), // caller must not replace it
		Profile:   ContactMemoryProfile{Summary: "第二版"},
	}); err != nil {
		t.Fatal(err)
	}

	got, ok := store.Get("acc", "wxid")
	if !ok {
		t.Fatal("更新后的记忆不存在")
	}
	if !got.CreatedAt.Equal(createdAt) || !got.UpdatedAt.Equal(updatedAt) {
		t.Fatalf("时间戳不正确: created=%v updated=%v", got.CreatedAt, got.UpdatedAt)
	}
}

func TestContactMemoryStorePutFailureDoesNotMutateMemory(t *testing.T) {
	dataDir := t.TempDir()
	store, err := NewContactMemoryStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	firstTime := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return firstTime }
	if err := store.Put(ContactMemory{AccountID: "acc", Talker: "wxid", Profile: ContactMemoryProfile{Summary: "旧记忆"}}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dataDir, contactMemoryFileName)
	beforeDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	store.now = func() time.Time { return firstTime.Add(time.Hour) }
	wantErr := errors.New("simulated disk failure")
	store.persist = func([]byte) error { return wantErr }
	err = store.Put(ContactMemory{AccountID: "acc", Talker: "wxid", Profile: ContactMemoryProfile{Summary: "不应生效"}})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Put 错误 = %v, want wrapped %v", err, wantErr)
	}

	got, ok := store.Get("acc", "wxid")
	if !ok || got.Profile.Summary != "旧记忆" || !got.UpdatedAt.Equal(firstTime) {
		t.Fatalf("失败的 Put 污染了内存: %+v", got)
	}
	afterDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(afterDisk, beforeDisk) {
		t.Fatal("失败的 Put 修改了磁盘快照")
	}
}

func TestContactMemoryStoreReturnsDeepCopies(t *testing.T) {
	store, err := NewContactMemoryStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	observedAt := time.Date(2026, 7, 1, 1, 2, 3, 0, time.UTC)
	input := ContactMemory{
		AccountID: "acc",
		Talker:    "wxid",
		Source:    ContactMemorySource{VoiceTranscriptIDs: []string{"voice-1"}},
		Profile: ContactMemoryProfile{
			Traits: []string{"直接"},
			SpeakingStyle: ContactMemorySpeakingStyle{
				Catchphrases: []ContactMemoryCatchphrase{{Text: "行", EvidenceSeqs: []int64{11}}},
			},
			Facts: []ContactMemoryFact{{Content: "喜欢茶", ObservedAt: &observedAt, EvidenceSeqs: []int64{12}, Confidence: "high"}},
		},
		UserOverrides: ContactMemoryUserOverrides{PinnedFacts: []string{"住在上海"}},
	}
	if err := store.Put(input); err != nil {
		t.Fatal(err)
	}

	input.Profile.Traits[0] = "被外部修改"
	input.Source.VoiceTranscriptIDs[0] = "被外部修改"
	input.Profile.SpeakingStyle.Catchphrases[0].EvidenceSeqs[0] = 999
	input.Profile.Facts[0].ObservedAt = nil
	input.UserOverrides.PinnedFacts[0] = "被外部修改"
	first, _ := store.Get("acc", "wxid")
	first.Source.VoiceTranscriptIDs[0] = "修改返回值"
	first.Profile.Traits[0] = "修改返回值"
	first.Profile.SpeakingStyle.Catchphrases[0].EvidenceSeqs[0] = 888
	*first.Profile.Facts[0].ObservedAt = time.Time{}
	first.UserOverrides.PinnedFacts[0] = "修改返回值"

	got, _ := store.Get("acc", "wxid")
	if got.Source.VoiceTranscriptIDs[0] != "voice-1" {
		t.Fatalf("source 没有深拷贝: %+v", got.Source)
	}
	if got.Profile.Traits[0] != "直接" || got.Profile.SpeakingStyle.Catchphrases[0].EvidenceSeqs[0] != 11 {
		t.Fatalf("profile 没有深拷贝: %+v", got.Profile)
	}
	if got.Profile.Facts[0].ObservedAt == nil || !got.Profile.Facts[0].ObservedAt.Equal(observedAt) {
		t.Fatalf("ObservedAt 没有深拷贝: %+v", got.Profile.Facts[0])
	}
	if got.UserOverrides.PinnedFacts[0] != "住在上海" {
		t.Fatalf("user_overrides 没有深拷贝: %+v", got.UserOverrides)
	}
}

func TestContactMemoryStoreConcurrentAccess(t *testing.T) {
	store, err := NewContactMemoryStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// This test exercises locking rather than the filesystem. Persistence remains
	// serialized under the same write lock in production.
	store.persist = func([]byte) error { return nil }

	const count = 40
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			talker := "wxid-" + time.Unix(int64(i), 0).UTC().Format("150405")
			if err := store.Put(ContactMemory{AccountID: "acc", Talker: talker}); err != nil {
				t.Errorf("并发 Put 失败: %v", err)
				return
			}
			if _, ok := store.Get("acc", talker); !ok {
				t.Errorf("并发 Get 未找到 %q", talker)
			}
		}()
	}
	wg.Wait()
	if got := len(store.List("acc")); got != count {
		t.Fatalf("并发写入后数量 = %d, want %d", got, count)
	}
}

func TestNewContactMemoryStoreRejectsMalformedFile(t *testing.T) {
	dataDir := t.TempDir()
	path := filepath.Join(dataDir, contactMemoryFileName)
	if err := os.WriteFile(path, []byte(`{"version":1,"memories":[`), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewContactMemoryStore(dataDir); err == nil {
		t.Fatal("损坏的记忆文件应返回错误")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("即使解析失败也应收紧权限: got %04o", got)
	}
}

func TestContactMemoryStoreRejectsEmptyTalker(t *testing.T) {
	store, err := NewContactMemoryStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(ContactMemory{AccountID: "acc", Talker: "  "}); !errors.Is(err, ErrInvalidContactMemoryKey) {
		t.Fatalf("空 talker 错误 = %v", err)
	}
	if err := store.Delete("acc", ""); !errors.Is(err, ErrInvalidContactMemoryKey) {
		t.Fatalf("空 talker 删除错误 = %v", err)
	}
}
