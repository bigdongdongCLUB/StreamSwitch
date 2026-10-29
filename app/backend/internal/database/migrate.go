package database

import (
	_ "embed"
	"fmt"
	"regexp"
	"strings"

	"livems/internal/pkg/logger"
)

// 内置数据库结构与初始化数据（随二进制发布，安装/升级无需手动导入 SQL）。
var (
	//go:embed schema.sql
	schemaSQL string
	//go:embed seed_always.sql
	seedAlwaysSQL string
	//go:embed seed_init.sql
	seedInitSQL string
)

var (
	createTableRe = regexp.MustCompile("(?s)CREATE TABLE IF NOT EXISTS `([a-z_0-9]+)` \\((.*)\\) ENGINE=")
	// 列定义行：以反引号开头的行（主键/索引行以 PRIMARY/UNIQUE/KEY 开头，不匹配）
	columnLineRe = regexp.MustCompile("^\\s*`([a-z_0-9]+)` (.+?),?\\s*$")
)

// Migrate 自动建表并补齐缺失列，然后写入内置角色/权限等初始化数据。
//
// 幂等：每次启动都可执行。
//   - 新库：建全部表 + 写入全部初始化数据（不创建任何用户，首个用户由后台注册页创建为超级管理员）；
//   - 旧库升级：缺表则建表，已有表按 schema.sql 补齐缺失列；新增的内置权限自动授予超管角色。
func Migrate() error {
	stmts := splitSQL(schemaSQL)
	fresh := false
	if n, err := countRows("sys_role"); err != nil || n == 0 {
		fresh = true
	}
	created, added := 0, 0
	for _, stmt := range stmts {
		m := createTableRe.FindStringSubmatch(stmt)
		if m == nil {
			continue
		}
		table := m[1]
		exists, err := tableExists(table)
		if err != nil {
			return fmt.Errorf("检查表 %s 失败: %w", table, err)
		}
		if !exists {
			if _, err := DB.Exec(stmt); err != nil {
				return fmt.Errorf("创建表 %s 失败: %w", table, err)
			}
			created++
			continue
		}
		cols, err := existingColumns(table)
		if err != nil {
			return fmt.Errorf("读取表 %s 列信息失败: %w", table, err)
		}
		for _, line := range strings.Split(m[2], "\n") {
			cm := columnLineRe.FindStringSubmatch(line)
			if cm == nil || cols[cm[1]] {
				continue
			}
			alter := fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `%s` %s", table, cm[1], cm[2])
			if _, err := DB.Exec(alter); err != nil {
				return fmt.Errorf("为表 %s 补充列 %s 失败: %w", table, cm[1], err)
			}
			logger.Info("数据库升级: %s 新增列 %s", table, cm[1])
			added++
		}
	}

	seeds := splitSQL(seedAlwaysSQL)
	if fresh {
		seeds = append(seeds, splitSQL(seedInitSQL)...)
	}
	for _, s := range seeds {
		if _, err := DB.Exec(s); err != nil {
			return fmt.Errorf("写入初始化数据失败: %w\n%s", err, firstLine(s))
		}
	}
	switch {
	case fresh:
		logger.Info("数据库初始化完成：创建 %d 张表，已写入内置角色与权限", created)
	case created > 0 || added > 0:
		logger.Info("数据库升级完成：新建 %d 张表，补充 %d 列", created, added)
	}
	return nil
}

// splitSQL 按「分号 + 换行」切分语句，并去掉注释行与空语句。
func splitSQL(src string) []string {
	var out []string
	for _, part := range strings.Split(src, ";\n") {
		var lines []string
		for _, l := range strings.Split(part, "\n") {
			if t := strings.TrimSpace(l); t == "" || strings.HasPrefix(t, "--") {
				continue
			}
			lines = append(lines, l)
		}
		if s := strings.TrimSpace(strings.Join(lines, "\n")); s != "" {
			out = append(out, strings.TrimSuffix(s, ";"))
		}
	}
	return out
}

func tableExists(table string) (bool, error) {
	var n int
	err := DB.QueryRow(`SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, table).Scan(&n)
	return n > 0, err
}

func existingColumns(table string) (map[string]bool, error) {
	rows, err := DB.Query(`SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		cols[strings.ToLower(c)] = true
	}
	return cols, rows.Err()
}

func countRows(table string) (int, error) {
	var n int
	if ok, err := tableExists(table); err != nil || !ok {
		return 0, err
	}
	err := DB.QueryRow("SELECT COUNT(*) FROM `" + table + "`").Scan(&n)
	return n, err
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	return s
}
