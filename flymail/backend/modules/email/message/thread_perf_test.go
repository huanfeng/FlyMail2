package message_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"flymail/internal/database"
	"flymail/internal/fts"
	"flymail/modules/email/message"
)

// TestThreadPerfRealDB 在真实库的副本上量线程重建与会话列表的耗时（验收「5 万封 P95 < 100ms」用）。
// 设 FLYMAIL_BENCH_DB=<flymail.db 路径> 才跑；库会先拷到临时目录，绝不动原文件。
func TestThreadPerfRealDB(t *testing.T) {
	src := os.Getenv("FLYMAIL_BENCH_DB")
	if src == "" {
		t.Skip("set FLYMAIL_BENCH_DB to run against a real database copy")
	}
	dst := filepath.Join(t.TempDir(), "bench.db")
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	_ = in.Close()
	_ = out.Close()

	db, err := database.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	start := time.Now()
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	t.Logf("migrate (含首次线程重建): %s", time.Since(start))

	var total, threads int64
	db.Raw("SELECT COUNT(*) FROM messages").Scan(&total)
	db.Raw("SELECT COUNT(DISTINCT thread_id) FROM messages").Scan(&threads)
	t.Logf("messages=%d threads=%d", total, threads)

	start = time.Now()
	n, err := message.RebuildThreads(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("rebuild again: %s (threads=%d)", time.Since(start), n)
	var idx []string
	db.Raw("SELECT name FROM sqlite_master WHERE type = 'index' AND tbl_name = 'messages'").Scan(&idx)
	t.Logf("indexes: %v", idx)

	repo := message.NewRepository(db)
	type folderRow struct {
		ID   uint
		N    int64
		Type string
	}
	var biggest folderRow
	db.Raw("SELECT folder_id AS id, COUNT(*) AS n, (SELECT type FROM folders WHERE id = folder_id) AS type FROM messages GROUP BY folder_id ORDER BY n DESC LIMIT 1").Scan(&biggest)
	t.Logf("biggest folder: id=%d n=%d type=%s", biggest.ID, biggest.N, biggest.Type)

	timeIt := func(name string, fn func() error) {
		var durs []time.Duration
		for i := 0; i < 5; i++ {
			s := time.Now()
			if err := fn(); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			durs = append(durs, time.Since(s))
		}
		t.Logf("%-40s %v", name, durs)
	}
	timeIt("folder threads first page", func() error {
		_, err := repo.FolderThreads(biggest.ID, message.Filter{}, nil, "", 50)
		return err
	})
	page, _ := repo.FolderThreads(biggest.ID, message.Filter{}, nil, "", 50)
	if page.NextCursor != nil {
		tm, _ := time.Parse(time.RFC3339Nano, page.NextCursor.BeforeDate)
		timeIt("folder threads page 2", func() error {
			_, err := repo.FolderThreads(biggest.ID, message.Filter{}, &tm, page.NextCursor.BeforeThread, 50)
			return err
		})
	}
	f := false
	timeIt("folder threads unread filter", func() error {
		_, err := repo.FolderThreads(biggest.ID, message.Filter{Seen: &f}, nil, "", 50)
		return err
	})
	for _, view := range []string{"inbox", "unread", "starred"} {
		timeIt("aggregate threads "+view, func() error {
			_, err := repo.AggregateThreads(view, message.Filter{}, nil, "", 50)
			return err
		})
	}
	agg, _ := repo.AggregateThreads("inbox", message.Filter{}, nil, "", 50)
	if agg.NextCursor != nil {
		tm, _ := time.Parse(time.RFC3339Nano, agg.NextCursor.BeforeDate)
		timeIt("aggregate threads inbox page 2", func() error {
			_, err := repo.AggregateThreads("inbox", message.Filter{}, &tm, agg.NextCursor.BeforeThread, 50)
			return err
		})
	}
	for _, q := range []string{"发票", "is:unread", "from:github", "报告"} {
		timeIt("search threads "+q, func() error {
			_, err := repo.SearchThreads(fts.Parse(q), message.Filter{}, nil, "", 50)
			return err
		})
	}
	if len(page.Threads) > 0 {
		// 挑一条最长的会话看展开耗时
		best := page.Threads[0]
		for _, th := range page.Threads {
			if th.Count > best.Count {
				best = th
			}
		}
		t.Logf("longest thread on page: count=%d subject=%q participants=%d", best.Count, best.Subject, len(best.Participants))
		timeIt("thread messages", func() error {
			_, err := repo.ThreadMessages(best.ThreadID, 0)
			return err
		})
	}
}
