package repo

import (
	"testing"

	"github.com/DrMaomao12345/Wetrace-Plus/internal/model"
)

func TestPaginateMessagesReverseReturnsNewestFirst(t *testing.T) {
	repo := &Repository{}
	msgs := []*model.Message{
		{Seq: 1},
		{Seq: 2},
		{Seq: 3},
		{Seq: 4},
		{Seq: 5},
	}

	firstPage := repo.paginateMessages(msgs, 2, 0, true)
	assertMessageSeqs(t, firstPage, []int64{5, 4})

	secondPage := repo.paginateMessages(msgs, 2, 2, true)
	assertMessageSeqs(t, secondPage, []int64{3, 2})
}

func TestPaginateMessagesForwardRemainsOldestFirst(t *testing.T) {
	repo := &Repository{}
	msgs := []*model.Message{{Seq: 1}, {Seq: 2}, {Seq: 3}}

	page := repo.paginateMessages(msgs, 2, 1, false)
	assertMessageSeqs(t, page, []int64{2, 3})
}

func assertMessageSeqs(t *testing.T, msgs []*model.Message, want []int64) {
	t.Helper()
	if len(msgs) != len(want) {
		t.Fatalf("消息数量：got %d, want %d", len(msgs), len(want))
	}
	for i := range want {
		if msgs[i].Seq != want[i] {
			t.Fatalf("第 %d 条消息：got %d, want %d", i, msgs[i].Seq, want[i])
		}
	}
}
