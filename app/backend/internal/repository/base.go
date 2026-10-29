package repository

import (
	"database/sql"

	"livems/internal/database"
)

// exec 执行一条写语句。
func exec(query string, args ...any) (sql.Result, error) {
	return database.DB.Exec(query, args...)
}

// query 执行一条多行查询。
func query(query string, args ...any) (*sql.Rows, error) {
	return database.DB.Query(query, args...)
}

// queryRow 执行单行查询。
func queryRow(query string, args ...any) *sql.Row {
	return database.DB.QueryRow(query, args...)
}

// execTx 是事务句柄的抽象，便于在事务回调中执行语句。
type execTx interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// tx 执行一个事务，fn 返回错误则回滚。
func tx(fn func(execTx) error) error {
	t, err := database.DB.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = t.Rollback()
			panic(p)
		}
	}()
	if err := fn(t); err != nil {
		_ = t.Rollback()
		return err
	}
	return t.Commit()
}
