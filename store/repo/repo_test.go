package repo

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/afumu/wetrace/store/bind"
	"github.com/afumu/wetrace/store/core"
	"github.com/afumu/wetrace/store/strategy"
	"github.com/afumu/wetrace/store/types"

	_ "github.com/mattn/go-sqlite3"
)

func TestRepo_GetContacts(t *testing.T) {
	tmpDir := t.TempDir()
	pool := core.NewConnectionPool(tmpDir)
	defer pool.CloseAll()
	strat := strategy.NewV4()

	// 创建 contact.db
	createContactDB(t, filepath.Join(tmpDir, "contact.db"))

	// 初始化 Router
	router := bind.NewTimelineRouter(tmpDir, pool, strat)

	repo := New(router, pool)

	// 测试
	contacts, err := repo.GetContacts(context.Background(), types.ContactQuery{})
	if err != nil {
		t.Fatalf("GetContacts 失败: %v", err)
	}
	// 必须 Fatalf：Errorf 不中断，下一行 contacts[0] 会 panic，
	// 而 panic 会终止整个包的测试进程，掩盖后面所有用例
	if len(contacts) != 1 {
		t.Fatalf("期望 1 个联系人, 实际得到 %d", len(contacts))
	}
	if contacts[0].UserName != "user1" {
		t.Errorf("期望 user1, 实际得到 %s", contacts[0].UserName)
	}
}

func TestRepo_GetMessages(t *testing.T) {
	tmpDir := t.TempDir()
	pool := core.NewConnectionPool(tmpDir)
	defer pool.CloseAll()
	strat := strategy.NewV4()

	// 创建 message_0.db
	t1 := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	createMessageDB(t, filepath.Join(tmpDir, "message_0.db"), t1, "alice", 1)

	// 初始化 Router
	router := bind.NewTimelineRouter(tmpDir, pool, strat)
	if err := router.RebuildIndex(context.Background()); err != nil {
		t.Fatalf("RebuildIndex 失败: %v", err)
	}

	repo := New(router, pool)

	// 测试
	msgs, err := repo.GetMessages(context.Background(), types.MessageQuery{
		StartTime: t1,
		EndTime:   t1.Add(time.Hour),
		Talker:    "alice",
	})
	if err != nil {
		t.Fatalf("GetMessages 失败: %v", err)
	}
	// 同上，必须 Fatalf
	if len(msgs) != 1 {
		t.Fatalf("期望 1 条消息, 实际得到 %d", len(msgs))
	}
	if msgs[0].Content != "hello" {
		t.Errorf("期望 'hello', 实际得到 '%s'", msgs[0].Content)
	}
}

func createContactDB(t *testing.T, path string) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// 列必须与生产查询（repo/contact.go）对齐 —— 少一列整条查询就报
	// "no such column"，而 COALESCE 只处理 NULL，**列不存在照样报错**，兜不住。
	_, err = db.Exec(`CREATE TABLE contact (
		username TEXT, local_type INTEGER, alias TEXT, remark TEXT, nick_name TEXT,
		small_head_url TEXT, big_head_url TEXT
	)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("INSERT INTO contact VALUES (?, ?, ?, ?, ?, ?, ?)",
		"user1", 0, "alias1", "remark1", "nick1", "", "")
	if err != nil {
		t.Fatal(err)
	}
}

func createMessageDB(t *testing.T, path string, startTime time.Time, user string, id int) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// V4 Metadata
	db.Exec("CREATE TABLE Timestamp (timestamp INTEGER)")
	db.Exec("INSERT INTO Timestamp VALUES (?)", startTime.Unix())

	// Msg_{md5} table
	hash := md5.Sum([]byte(user))
	tableName := "Msg_" + hex.EncodeToString(hash[:])

	// 列必须与生产查询（repo/message.go）对齐，含 compress_content —— 缺了它
	// 查询会失败，而 message.go 的失败处理是「跳过整个分片只打一条 warn」，
	// 对上层表现为「这个时间段没有消息」，很难查。
	sqlStmt := fmt.Sprintf(`
	CREATE TABLE %s (
		sort_seq INTEGER, server_id INTEGER, local_type INTEGER, 
		real_sender_id INTEGER, create_time INTEGER, 
		message_content TEXT, compress_content BLOB, packed_info_data BLOB, status INTEGER
	)`, tableName)
	_, err = db.Exec(sqlStmt)
	if err != nil {
		t.Fatalf("创建 %s 失败: %v", tableName, err)
	}

	// Name2Id table for sender resolution
	db.Exec("CREATE TABLE Name2Id (user_name TEXT)")
	db.Exec("INSERT INTO Name2Id VALUES (?)", user)

	// 插入消息
	_, err = db.Exec(fmt.Sprintf(`INSERT INTO %s VALUES (
		?, 1, 1, 1, ?, 'hello', NULL, NULL, 0
	)`, tableName), startTime.Unix()*1000, startTime.Unix())
	if err != nil {
		t.Fatalf("插入消息失败: %v", err)
	}
}
