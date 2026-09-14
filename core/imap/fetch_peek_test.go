package imap

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"strings"
	"testing"
)

// TestBodySectionsAlwaysPeek 守的是一整类缺陷，不是已经修好的那两行。
//
// ── 背景 ─────────────────────────────────────────────────────────────────
//
// IMAP 的 `FETCH BODY[...]` 会让服务端给邮件置上 \Seen；`BODY.PEEK[...]` 不会。
// 而 FlyMail 的后台正文预取会自动抓取正文——少一个 PEEK，**光是同步就会把
// 用户邮箱里抓过的未读邮件全部标成已读**，并且是在服务端：手机、网页版、
// 其它客户端一起跟着变。未读状态丢了就找不回来（没有"本来哪些是未读"的记录）。
//
// 这个缺陷真的发生过，而且成因很说明问题：threadHeaderSection 上早就写着
// 「PEEK 避免把邮件标成已读」，写它的人显然知道规则——但抓整封正文的**另外
// 两处**各自漏了。规则记在一个人脑子里、写在一处注释上，是守不住的。
//
// ── 为什么扫源码而不是断言那两个值 ───────────────────────────────────────
//
// 断言 `newFullBodySection().Peek == true` 只能证明现有构造函数是对的，
// 挡不住下一个人在别处再 `&imapv2.FetchItemBodySection{}` 写一次——
// 那正是这次的出错方式。所以判据是：本包里**任何一处** FetchItemBodySection
// 的字面量构造都必须显式带上 Peek。
func TestBodySectionsAlwaysPeek(t *testing.T) {
	fset := token.NewFileSet()
	files, err := parseGoFiles(fset)
	if err != nil {
		t.Fatalf("解析源码: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("一个源文件都没扫到——这条守卫等于没跑")
	}

	var bare []string
	var seen int
	for name, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			sel, ok := lit.Type.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "FetchItemBodySection" {
				return true
			}
			seen++
			for _, el := range lit.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "Peek" {
					if v, ok := kv.Value.(*ast.Ident); ok && v.Name == "true" {
						return true // 这一处没问题
					}
				}
			}
			bare = append(bare, name+":"+fset.Position(lit.Pos()).String())
			return true
		})
	}

	if seen == 0 {
		t.Fatal("没扫到任何 FetchItemBodySection 构造——选择器可能改名了，守卫已失效")
	}
	if len(bare) > 0 {
		t.Errorf("以下 FetchItemBodySection 没有 Peek: true，抓取会把邮件在服务端标成已读：\n  %s",
			strings.Join(bare, "\n  "))
	}
}

// parseGoFiles 解析本包的非测试源文件。
func parseGoFiles(fset *token.FileSet) (map[string]*ast.File, error) {
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		return nil, err
	}
	out := map[string]*ast.File{}
	for _, p := range pkgs {
		for name, f := range p.Files {
			out[name] = f
		}
	}
	return out, nil
}
