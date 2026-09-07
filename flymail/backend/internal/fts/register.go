package fts

import (
	"database/sql/driver"
	"fmt"

	sqlite "github.com/glebarez/go-sqlite"
)

// FuncName 是注册到 SQLite 的切分函数名，供触发器/回填 SQL 调用：fts_tokens(text)。
const FuncName = "fts_tokens"

// StripHTMLFuncName 是 HTML 降纯文本函数名：fts_strip_html(html)。
// 纯 HTML 邮件 text_body 为空，索引时退回剥掉标签的 html_body。
const StripHTMLFuncName = "fts_strip_html"

// init 在进程启动时把 Tokenize 注册为 SQLite 标量函数。
//
// 放在 init 而不是某个显式 Setup：驱动的注册只对之后新开的连接生效，
// 而 GORM 连接池是惰性建连的——只要本包在任何连接建立之前被 import 到，就必然先注册。
// message 模块与 database 包都 import 本包，这一点由编译期依赖顺序保证。
//
// 之所以要在 SQL 层提供切分函数：索引由触发器维护，无论 Go 侧哪条路径写 messages /
// message_bodies（upsert、批量删除、UIDVALIDITY 重建……），索引都能自动跟上，
// 不需要每个写入点都记得同步 FTS——那种约定迟早会在某个新路径上被漏掉。
func init() {
	err := sqlite.RegisterDeterministicScalarFunction(FuncName, 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			return Tokenize(argString(args[0])), nil
		})
	if err != nil {
		panic("fts: register " + FuncName + ": " + err.Error())
	}
	err = sqlite.RegisterDeterministicScalarFunction(StripHTMLFuncName, 1,
		func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			return StripHTML(argString(args[0])), nil
		})
	if err != nil {
		panic("fts: register " + StripHTMLFuncName + ": " + err.Error())
	}
}

// argString 把 SQLite 传来的参数统一成字符串（NULL → 空串）。
func argString(v driver.Value) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return fmt.Sprint(x)
	}
}
