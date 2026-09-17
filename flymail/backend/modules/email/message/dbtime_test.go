package message

import (
	"testing"
	"time"
)

// 驱动落库的时间文本要能解析回来。
//
// ── 缘起（用户报的：列表里的时间显示成 1901 年） ─────────────────────────────
//
// 2026-09-16 的「邮件日期统一存 UTC」迁移（EnsureUTCDates 用
// strftime('%Y-%m-%dT%H:%M:%SZ', date)）把全库日期写成了 Z 结尾，而 dbTimeLayouts
// 里的偏移量写的是 -07:00——Go 的 -07:00 只认字面的 ±hh:mm，遇到 Z 直接失败。
// 于是 parseDBTime 对**每一行**都返回零值。
//
// ⚠ 后果有两层，第二层更隐蔽：
//  1. 会话列表的日期显示成 0001-01-01（界面上就是那个怪年份）
//  2. LastDate 同时是游标键——零值让下一页立刻为空，会话视图的分页悄悄失效
func TestParseDBTimeAcceptsUTCAndOffset(t *testing.T) {
	cases := []struct {
		in   string
		why  string
		want time.Time
	}{
		{
			in:   "2026-09-16T11:09:44Z",
			why:  "UTC 迁移写成的规范形式——正是它让整个列表变成了零值",
			want: time.Date(2026, 9, 16, 11, 9, 44, 0, time.UTC),
		},
		{
			in:   "2026-09-16T11:09:44.123456789Z",
			why:  "带纳秒的 Z 形式",
			want: time.Date(2026, 9, 16, 11, 9, 44, 123456789, time.UTC),
		},
		{
			in:   "2026-09-16 11:09:44+08:00",
			why:  "驱动写本地时间时的形式，不能因为支持 Z 就把它弄丢",
			want: time.Date(2026, 9, 16, 11, 9, 44, 0, time.FixedZone("", 8*3600)),
		},
		{
			in:   "2026-09-16 11:09:44",
			why:  "没有偏移量的裸形式（老库）",
			want: time.Date(2026, 9, 16, 11, 9, 44, 0, time.UTC),
		},
	}
	for _, tc := range cases {
		got := parseDBTime(tc.in)
		if got.IsZero() {
			t.Errorf("%q 解析成了零值 —— %s", tc.in, tc.why)
			continue
		}
		if !got.Equal(tc.want) {
			t.Errorf("%q 解析成 %v，想要 %v（%s）", tc.in, got, tc.want, tc.why)
		}
	}
}

// 解析不了的输入仍然返回零值（调用方靠它判断），不能 panic。
func TestParseDBTimeRejectsGarbage(t *testing.T) {
	for _, s := range []string{"", "not a time", "2026/09/16"} {
		if got := parseDBTime(s); !got.IsZero() {
			t.Errorf("%q 应当解析失败，却得到 %v", s, got)
		}
	}
}
