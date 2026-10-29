// Command server 是流媒体管理系统的后端服务入口。
//
// 用法：
//
//	server                     使用默认配置路径启动
//	server -c /path/config.yaml  指定配置文件
//	server -init-admin          重置管理员密码并退出
//	server -version             打印版本号
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"livems/internal/config"
	"livems/internal/database"
	"livems/internal/pkg/logger"
	"livems/internal/repository"
	"livems/internal/server"
)

// Version 服务版本号。
const Version = "1.0.0"

func main() {
	var (
		configPath = flag.String("c", "configs/config.yaml", "配置文件路径")
		initAdmin  = flag.Bool("init-admin", false, "重置 admin 账号密码并退出")
		newPass    = flag.String("password", "admin123", "-init-admin 时设置的新密码")
		showVer    = flag.Bool("version", false, "打印版本号")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("流媒体管理系统 v%s\n", Version)
		return
	}

	// 加载配置
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}
	config.Set(cfg)

	// 初始化日志
	log := logger.New(logger.Options{
		Level:      logger.ParseLevel(cfg.Log.Level),
		Format:     cfg.Log.Format,
		OutputFile: cfg.Log.OutputFile,
	})
	logger.SetDefault(log)
	defer log.Close()

	logger.Info("流媒体管理系统 v%s 启动中...", Version)

	// 初始化数据库与缓存
	if err := database.InitMySQL(cfg.MySQL); err != nil {
		logger.Error("初始化 MySQL 失败: %v", err)
		os.Exit(1)
	}
	if err := database.InitRedis(cfg.Redis); err != nil {
		logger.Error("初始化 Redis 失败: %v", err)
		os.Exit(1)
	}

	// 管理员初始化模式
	if *initAdmin {
		if err := resetAdminPassword(*newPass); err != nil {
			logger.Error("重置管理员密码失败: %v", err)
			os.Exit(1)
		}
		logger.Info("管理员密码已重置，用户名: admin")
		return
	}

	// 恢复后台任务（代理/转推/加密配置）
	server.RestoreTasks(cfg)

	// 启动 HTTP 服务
	srv := server.New(cfg)
	if err := srv.Run(); err != nil {
		logger.Error("服务运行异常: %v", err)
		os.Exit(1)
	}
}

// resetAdminPassword 重置 admin 账号的密码。
func resetAdminPassword(password string) error {
	if len(password) < 6 {
		return fmt.Errorf("密码长度不能少于 6 位")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	repo := repository.NewUserRepo()
	user, err := repo.GetByUsername("admin")
	if err != nil {
		return fmt.Errorf("未找到 admin 账号，请先导入 sql/schema.sql: %w", err)
	}
	if err := repo.UpdatePassword(user.ID, string(hash)); err != nil {
		return err
	}
	return nil
}

// ensure filepath 与 strings 被引用。
var (
	_ = filepath.Join
	_ = strings.TrimSpace
)
