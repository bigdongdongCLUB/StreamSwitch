// Package database 负责数据库与缓存的初始化、连接管理与通用辅助函数。
package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"

	"livems/internal/config"
	"livems/internal/pkg/logger"
)

// MySQL 全局数据库句柄。
var (
	// DB 是 MySQL 连接池，程序启动时初始化。
	DB *sql.DB
	// RDB 是 Redis 客户端，程序启动时初始化。
	RDB *redis.Client
)

// InitMySQL 初始化 MySQL 连接池并做连通性检查。
func InitMySQL(cfg config.MySQLConfig) error {
	db, err := sql.Open("mysql", cfg.DSN())
	if err != nil {
		return fmt.Errorf("打开数据库连接失败: %w", err)
	}
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("数据库连通性检查失败: %w", err)
	}
	DB = db
	logger.Info("MySQL 连接成功: %s:%d/%s", cfg.Host, cfg.Port, cfg.Database)
	return nil
}

// InitRedis 初始化 Redis 客户端并做连通性检查。
func InitRedis(cfg config.RedisConfig) error {
	rdb := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr(),
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		DialTimeout:  time.Duration(cfg.DialTimeout) * time.Second,
		ReadTimeout:  time.Duration(cfg.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(cfg.WriteTimeout) * time.Second,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return fmt.Errorf("Redis 连通性检查失败: %w", err)
	}
	RDB = rdb
	logger.Info("Redis 连接成功: %s db=%d", cfg.Addr(), cfg.DB)
	return nil
}

// Close 关闭数据库与缓存连接。
func Close() {
	if DB != nil {
		_ = DB.Close()
	}
	if RDB != nil {
		_ = RDB.Close()
	}
}

// RedisKey 按前缀拼接 Redis key。
func RedisKey(prefix string, parts ...string) string {
	out := prefix
	for _, p := range parts {
		out += ":" + p
	}
	return out
}

// IsNotFound 判断 SQL 错误是否为「无记录」。
func IsNotFound(err error) bool {
	return err == sql.ErrNoRows
}
