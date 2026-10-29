// Command server 是流媒体管理系统的后端服务入口。
//
// 用法：
//
//	server                                   使用默认配置路径启动（启动时自动建表/升级数据库）
//	server -c /path/config.yaml              指定配置文件
//	server -migrate                          仅执行数据库建表/升级后退出
//	server -list-users                       列出全部后台用户后退出
//	server -reset-password -user <用户名>     重置指定用户的密码后退出（密码取 -password 或环境变量 LIVEMS_NEW_PASSWORD）
//	server -version                          打印版本号
package main

import (
	"flag"
	"fmt"
	"os"

	"livems/internal/config"
	"livems/internal/database"
	"livems/internal/pkg/logger"
	"livems/internal/server"
	"livems/internal/service"
)

// Version 服务版本号（发布构建时通过 -ldflags "-X main.Version=x.y.z" 注入）。
var Version = "dev"

func main() {
	var (
		configPath = flag.String("c", "configs/config.yaml", "配置文件路径")
		migrate    = flag.Bool("migrate", false, "仅执行数据库建表/升级后退出")
		listUsers  = flag.Bool("list-users", false, "列出全部后台用户后退出")
		resetPwd   = flag.Bool("reset-password", false, "重置指定用户的密码后退出")
		initAdmin  = flag.Bool("init-admin", false, "（兼容旧版）重置 admin 用户的密码后退出")
		user       = flag.String("user", "", "-reset-password 的目标用户名")
		newPass    = flag.String("password", "", "新密码（也可用环境变量 LIVEMS_NEW_PASSWORD 传入，避免出现在进程列表中）")
		showVer    = flag.Bool("version", false, "打印版本号")
	)
	flag.Parse()

	if *showVer {
		fmt.Printf("StreamSwitch 流媒体管理系统 v%s\n", Version)
		return
	}
	server.Version = Version

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}
	config.Set(cfg)

	cliMode := *migrate || *listUsers || *resetPwd || *initAdmin
	logOpts := logger.Options{
		Level:      logger.ParseLevel(cfg.Log.Level),
		Format:     cfg.Log.Format,
		OutputFile: cfg.Log.OutputFile,
	}
	if cliMode {
		logOpts.Level = logger.ParseLevel("warn") // 命令行模式只输出结果
		logOpts.OutputFile = ""
	}
	log := logger.New(logOpts)
	logger.SetDefault(log)
	defer log.Close()

	if !cliMode {
		logger.Info("StreamSwitch 流媒体管理系统 v%s 启动中...", Version)
	}

	if err := database.InitMySQL(cfg.MySQL); err != nil {
		fail("初始化 MySQL 失败: %v", err)
	}
	if err := database.Migrate(); err != nil {
		fail("数据库建表/升级失败: %v", err)
	}
	if *migrate {
		fmt.Println("数据库建表/升级完成")
		return
	}
	if *listUsers {
		runListUsers(cfg)
		return
	}
	if *resetPwd || *initAdmin {
		target := *user
		if *initAdmin && target == "" {
			target = "admin"
		}
		pwd := *newPass
		if pwd == "" {
			pwd = os.Getenv("LIVEMS_NEW_PASSWORD")
		}
		runResetPassword(cfg, target, pwd)
		return
	}

	if err := database.InitRedis(cfg.Redis); err != nil {
		fail("初始化 Redis 失败: %v", err)
	}

	// 恢复后台任务（代理/转推/加密配置/制播）
	server.RestoreTasks(cfg)

	srv := server.New(cfg)
	if err := srv.Run(); err != nil {
		fail("服务运行异常: %v", err)
	}
}

// runListUsers 以制表符分隔输出：ID 用户名 昵称 超管 状态（供部署脚本解析）。
func runListUsers(cfg *config.Config) {
	users, err := service.NewUserService(cfg).AllUsers()
	if err != nil {
		fail("查询用户失败: %v", err)
	}
	for _, u := range users {
		super, status := "否", "启用"
		if u.IsSuper == 1 {
			super = "是"
		}
		if u.Status != 1 {
			status = "禁用"
		}
		fmt.Printf("%d\t%s\t%s\t%s\t%s\n", u.ID, u.Username, u.Nickname, super, status)
	}
}

func runResetPassword(cfg *config.Config, username, password string) {
	if username == "" {
		fail("请用 -user 指定要重置密码的用户名")
	}
	if password == "" {
		fail("请用 -password 或环境变量 LIVEMS_NEW_PASSWORD 提供新密码")
	}
	u, err := service.NewUserService(cfg).ResetPasswordByUsername(username, password)
	if err != nil {
		fail("重置密码失败: %v", err)
	}
	fmt.Printf("已重置用户 %s 的密码（账号状态：启用）\n", u.Username)
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
