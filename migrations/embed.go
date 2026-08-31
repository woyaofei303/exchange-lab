// Package migrations 内嵌 API 和独立迁移命令使用的版本化 PostgreSQL 数据库结构。
package migrations

import "embed"

// CurrentVersion 是当前 API 二进制程序所需的数据库结构版本。
const CurrentVersion int64 = 16

// FS 包含包根目录下的 SQL 迁移文件。
//
//go:embed *.sql
var FS embed.FS
