package importer

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DrMaomao12345/Wetrace-Plus/store"
	"github.com/DrMaomao12345/Wetrace-Plus/store/types"
	_ "github.com/mattn/go-sqlite3"
)

func TestImportChatlogJSONAndDeduplicate(t *testing.T) {
	dataDir := t.TempDir()
	inputDir := t.TempDir()
	inputPath := filepath.Join(inputDir, "chatlog.json")
	messages := []map[string]any{
		{
			"seq": 1704067200001, "time": "2024-01-01T08:00:00+08:00",
			"talker": "wxid_friend", "talkerName": "小明", "isChatRoom": false,
			"sender": "wxid_friend", "senderName": "小明", "isSelf": false,
			"type": 1, "subType": 0, "content": "新年快乐",
		},
		{
			"seq": 1704067260001, "time": "2024-01-01T08:01:00+08:00",
			"talker": "wxid_friend", "talkerName": "小明", "isChatRoom": false,
			"sender": "wxid_me", "senderName": "我", "isSelf": true,
			"type": 3, "subType": 0, "content": "[图片]",
		},
	}
	payload, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inputPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}

	service := New(dataDir)
	result, err := service.ImportFiles(context.Background(), []Upload{{Name: "chatlog.json", Path: inputPath, Size: int64(len(payload))}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 2 || result.Duplicates != 0 || result.Conversations != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}

	result, err = service.ImportFiles(context.Background(), []Upload{{Name: "chatlog.json", Path: inputPath, Size: int64(len(payload))}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 0 || result.Duplicates != 2 {
		t.Fatalf("unexpected duplicate result: %+v", result)
	}

	assertImportedStore(t, dataDir)
}

func TestImportChatlogKeeperWeChatJSON(t *testing.T) {
	dataDir := t.TempDir()
	inputPath := filepath.Join("testdata", "chatlog-keeper-wechat.json")
	payload, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatal(err)
	}

	result, err := New(dataDir).ImportFiles(context.Background(), []Upload{{Name: "renamed-export.json", Path: inputPath, Size: int64(len(payload))}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 4 || result.Conversations != 3 || len(result.Warnings) != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(result.Sources) != 1 || result.Sources[0].Format != FormatChatlogKeeperJSON {
		t.Fatalf("wrong adapter: %+v", result.Sources)
	}

	db, err := sql.Open("sqlite3", filepath.Join(dataDir, "message", "message_0.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query(`SELECT source_message_id, status, create_time, message_content FROM ` + messageTable("wxid_account::room@chatroom") + ` ORDER BY create_time`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type stored struct {
		id      string
		status  int
		created int64
		content string
	}
	var got []stored
	for rows.Next() {
		var item stored
		if err := rows.Scan(&item.id, &item.status, &item.created, &item.content); err != nil {
			t.Fatal(err)
		}
		got = append(got, item)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d stored messages", len(got))
	}
	if got[0].id != "server-message-1" || got[0].status != 4 || got[0].created != 1789008000 || got[0].content != "wxid_sender:\nhello" {
		t.Fatalf("incoming message was not preserved: %+v", got[0])
	}
	if got[1].id != "wechat_export:second-message-hash" || got[1].status != 2 || got[1].created != 1789008060 || got[1].content != "reply" {
		t.Fatalf("outgoing message was not preserved: %+v", got[1])
	}
	var secondAccountCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + messageTable("wxid_second_account::room@chatroom")).Scan(&secondAccountCount); err != nil {
		t.Fatal(err)
	}
	if secondAccountCount != 1 {
		t.Fatalf("multi-account conversation was merged: got %d messages in second account", secondAccountCount)
	}
	sessionDB, err := sql.Open("sqlite3", filepath.Join(dataDir, "session", "session.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sessionDB.Close()
	var sessionCount int
	if err := sessionDB.QueryRow(`SELECT COUNT(*) FROM SessionTable WHERE username IN (?, ?)`, "wxid_account::room@chatroom", "wxid_second_account::room@chatroom").Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 2 {
		t.Fatalf("account-aware thread IDs were not preserved: got %d sessions", sessionCount)
	}
	var directType, directStatus int
	var directContent string
	if err := db.QueryRow(`SELECT local_type, status, message_content FROM `+messageTable("wxid_account::wxid_friend")).Scan(&directType, &directStatus, &directContent); err != nil {
		t.Fatal(err)
	}
	if directType != 3 || directStatus != 4 || directContent != "[图片]" {
		t.Fatalf("direct media placeholder was not preserved: type=%d status=%d content=%q", directType, directStatus, directContent)
	}
}

func TestImportMemoTraceCSVAliasAndZIP(t *testing.T) {
	dataDir := t.TempDir()
	inputDir := t.TempDir()
	zipPath := filepath.Join(inputDir, "memotrace.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(f)
	entry, err := archive.Create("聊天记录/小红(wxid_red)/小红.csv")
	if err != nil {
		t.Fatal(err)
	}
	_, err = entry.Write([]byte("\ufeff消息ID,类型,发送人,时间,内容,备注,昵称,更多信息\n1,文本,小红,2024-02-01 10:00:00,在吗,小红,小红,more\n2,文本,我,2024-02-01 10:01:00,在,我,我,more\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(zipPath)

	result, err := New(dataDir).ImportFiles(context.Background(), []Upload{{Name: "memotrace.zip", Path: zipPath, Size: info.Size()}}, Options{SelfName: "我"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 2 || len(result.Sources) != 1 || result.Sources[0].Format != FormatMemoTraceCSV {
		t.Fatalf("unexpected result: %+v", result)
	}

	assertImportedStore(t, dataDir)
}

func TestImportWetracePlusArchive(t *testing.T) {
	dataDir := t.TempDir()
	inputDir := t.TempDir()
	inputPath := filepath.Join(inputDir, "archive.json")
	payload := []byte(`{
		"format":"wetrace-plus","version":1,
		"account":{"id":"wxid_me","name":"我"},
		"conversations":[{
			"id":"123@chatroom","name":"测试群","type":"group",
			"messages":[
				{"id":"a","time":"2024-03-01T10:00:00+08:00","sender_id":"wxid_a","sender_name":"小 A","direction":"incoming","type":1,"content":"早"},
				{"id":"b","time":1709268060,"sender_id":"wxid_me","sender_name":"我","direction":"outgoing","type":1,"content":"早上好"}
			]
		}]
	}`)
	if err := os.WriteFile(inputPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := New(dataDir).ImportFiles(context.Background(), []Upload{{Name: "archive.json", Path: inputPath, Size: int64(len(payload))}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 2 || result.Sources[0].Format != FormatWetracePlusJSON {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func assertImportedStore(t *testing.T, dataDir string) {
	t.Helper()
	s, err := store.NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sessions, err := s.GetSessions(context.Background(), types.SessionQuery{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions", len(sessions))
	}
	messages, err := s.GetMessages(context.Background(), types.MessageQuery{
		Talker:    sessions[0].UserName,
		StartTime: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC),
		Limit:     100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("got %d messages", len(messages))
	}
	if messages[0].Content == "" || messages[1].Content == "" {
		t.Fatalf("message content was not preserved: %+v", messages)
	}

	db, err := sql.Open("sqlite3", filepath.Join(dataDir, "message", "message_0.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM wetrace_import_sources`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatal("missing import history")
	}
}
