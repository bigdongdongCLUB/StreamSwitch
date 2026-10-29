-- =============================================================================
--  StreamSwitch 数据库结构（由后端启动时自动执行，幂等）
--  - CREATE TABLE IF NOT EXISTS：新库建表
--  - 已存在的表：迁移器会按本文件的列定义自动补齐缺失列（ALTER TABLE ADD COLUMN）
--  修改表结构时只需编辑本文件；不要使用 DROP / 带分号的注释。
-- =============================================================================

CREATE TABLE IF NOT EXISTS `hls_encrypt` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `stream_id` bigint unsigned NOT NULL COMMENT '关联的流ID',
  `enabled` tinyint NOT NULL DEFAULT '0' COMMENT '是否开启 HLS AES-128 加密 1=开启',
  `key_hex` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'AES-128 密钥(32位hex=16字节)',
  `key_uri` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '密钥下发路径，如 /hls-key/{stream}.key',
  `key_expire_sec` int NOT NULL DEFAULT '0' COMMENT '密钥有效期(秒)，0=永久有效',
  `iv_strategy` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'seq' COMMENT 'IV 策略 seq=使用媒体序号(符合RFC8216) random=随机',
  `expire_at` bigint NOT NULL DEFAULT '0' COMMENT '密钥过期时间戳，0=不过期',
  `last_rotate_at` bigint NOT NULL DEFAULT '0' COMMENT '上次轮换时间',
  `created_at` bigint NOT NULL DEFAULT '0',
  `updated_at` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_stream` (`stream_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='HLS AES-128 加密配置';

CREATE TABLE IF NOT EXISTS `live_page` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(128) NOT NULL COMMENT '页面名称',
  `code` varchar(32) NOT NULL COMMENT '访问码（URL 标识）',
  `stream_id` bigint unsigned NOT NULL COMMENT '关联流ID',
  `password` varchar(128) NOT NULL DEFAULT '' COMMENT '访问密码（bcrypt 哈希，空=公开）',
  `announcement` varchar(512) NOT NULL DEFAULT '' COMMENT '页面公告/简介',
  `status` tinyint NOT NULL DEFAULT '1' COMMENT '1=启用 0=禁用',
  `view_count` bigint NOT NULL DEFAULT '0' COMMENT '累计访问次数',
  `created_by` bigint unsigned NOT NULL DEFAULT '0',
  `created_at` bigint NOT NULL DEFAULT '0',
  `updated_at` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_code` (`code`),
  KEY `idx_stream` (`stream_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='直播分享页';

CREATE TABLE IF NOT EXISTS `play_domain` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '域名名称',
  `schema` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'http' COMMENT '协议 http/https',
  `host` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '主机名，如 live.example.com',
  `port` int NOT NULL DEFAULT '80' COMMENT '端口，https 默认 443',
  `is_default` tinyint NOT NULL DEFAULT '0' COMMENT '是否默认域名 1=是',
  `status` tinyint NOT NULL DEFAULT '1' COMMENT '状态 1=启用 0=禁用',
  `sort` int NOT NULL DEFAULT '0' COMMENT '排序',
  `remark` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '备注',
  `created_at` bigint NOT NULL DEFAULT '0',
  `updated_at` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_host` (`host`),
  KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='播放域名';

CREATE TABLE IF NOT EXISTS `play_format` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `format` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '格式 rtmp/http-flv/ws-flv/hls',
  `name` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '显示名称',
  `enabled` tinyint NOT NULL DEFAULT '1' COMMENT '是否对外分发 1=开启 0=关闭',
  `path_prefix` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'URL 路径前缀，如 /live',
  `sort` int NOT NULL DEFAULT '0',
  `remark` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '备注',
  `created_at` bigint NOT NULL DEFAULT '0',
  `updated_at` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_format` (`format`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='播放格式开关';

CREATE TABLE IF NOT EXISTS `play_token` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `token` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'Token 明文(建议只存哈希，保留明文便于排查)',
  `stream_id` bigint unsigned NOT NULL DEFAULT '0' COMMENT '关联流ID，0=不限流',
  `app` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'app',
  `stream` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '流ID',
  `expire_at` bigint NOT NULL DEFAULT '0' COMMENT '过期时间戳',
  `ip_bind` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'IP绑定，空=不限制；多个用逗号分隔',
  `format` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '限定格式，空=全部格式共用',
  `created_by` bigint unsigned NOT NULL DEFAULT '0',
  `created_at` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_token` (`token`),
  KEY `idx_stream` (`stream_id`),
  KEY `idx_expire` (`expire_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='播放Token';

CREATE TABLE IF NOT EXISTS `stream` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '流名称（展示用）',
  `app` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'live' COMMENT 'ZLMediaKit app',
  `stream` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'ZLMediaKit 流ID（自定义流Key）',
  `vhost` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '__defaultVhost__' COMMENT '虚拟主机',
  `push_key` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '推流鉴权Key（自定义流Key）',
  `status` tinyint NOT NULL DEFAULT '1' COMMENT '状态 1=启用 0=禁用',
  `remark` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '备注',
  `online` tinyint NOT NULL DEFAULT '0' COMMENT '在线状态 1=在线 0=离线（由 on_publish/on_play 更新）',
  `publish_ip` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '推流端IP',
  `publish_time` bigint NOT NULL DEFAULT '0' COMMENT '上线时间',
  `offline_time` bigint NOT NULL DEFAULT '0' COMMENT '最后下线时间',
  `video_codec` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '视频编码 如 H264/H265',
  `audio_codec` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '音频编码 如 AAC/G711',
  `width` int NOT NULL DEFAULT '0' COMMENT '宽',
  `height` int NOT NULL DEFAULT '0' COMMENT '高',
  `fps` int NOT NULL DEFAULT '0' COMMENT '帧率',
  `bitrate` int NOT NULL DEFAULT '0' COMMENT '总码率 kbps',
  `video_bitrate` int NOT NULL DEFAULT '0' COMMENT '视频码率 kbps',
  `audio_bitrate` int NOT NULL DEFAULT '0' COMMENT '音频码率 kbps',
  `sample_rate` int NOT NULL DEFAULT '0' COMMENT '音频采样率',
  `reader_count` int NOT NULL DEFAULT '0' COMMENT '当前观看人数',
  `total_reader` int NOT NULL DEFAULT '0' COMMENT '累计观看人次',
  `total_bytes` bigint NOT NULL DEFAULT '0' COMMENT '累计流量(字节)',
  `created_by` bigint unsigned NOT NULL DEFAULT '0' COMMENT '创建人ID',
  `created_at` bigint NOT NULL DEFAULT '0',
  `updated_at` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_app_stream` (`app`,`stream`),
  KEY `idx_status` (`status`),
  KEY `idx_online` (`online`),
  KEY `idx_publish_time` (`publish_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='推流管理';

CREATE TABLE IF NOT EXISTS `stream_forward` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '任务名称',
  `src_app` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'live' COMMENT '源 app',
  `src_stream` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '源流ID',
  `src_vhost` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '__defaultVhost__',
  `dst_url` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'RTMP 目标地址',
  `schema` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'rtmp' COMMENT '推流协议 rtmp/rtmps',
  `status` tinyint NOT NULL DEFAULT '1' COMMENT '状态 1=启用 0=禁用',
  `run_status` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'idle' COMMENT '运行状态 idle/running/failed/stopped',
  `retry_count` int NOT NULL DEFAULT '-1' COMMENT '失败重试次数，-1=无限',
  `timeout_sec` int NOT NULL DEFAULT '10' COMMENT '连接超时(秒)',
  `zlm_key` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'ZLMediaKit 任务 key',
  `retry_times` int NOT NULL DEFAULT '0' COMMENT '已重试次数',
  `last_error` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '最近一次错误',
  `last_active_at` bigint NOT NULL DEFAULT '0' COMMENT '最后活跃时间',
  `created_by` bigint unsigned NOT NULL DEFAULT '0',
  `created_at` bigint NOT NULL DEFAULT '0',
  `updated_at` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_dst` (`dst_url`(255),`src_stream`),
  KEY `idx_src` (`src_app`,`src_stream`),
  KEY `idx_status` (`status`),
  KEY `idx_run_status` (`run_status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='转推流任务';

CREATE TABLE IF NOT EXISTS `stream_proxy` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '任务名称',
  `src_url` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '远端源地址 rtmp/http-flv/hls',
  `app` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'live' COMMENT '本地 app',
  `stream` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '本地流ID',
  `vhost` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '__defaultVhost__',
  `status` tinyint NOT NULL DEFAULT '1' COMMENT '状态 1=启用 0=禁用',
  `retry_count` int NOT NULL DEFAULT '-1' COMMENT '失败重试次数，-1=无限重试',
  `retry_interval` int NOT NULL DEFAULT '10' COMMENT '重试间隔(秒)',
  `timeout_sec` int NOT NULL DEFAULT '10' COMMENT '拉流握手超时(秒)',
  `enable_rtsp` tinyint NOT NULL DEFAULT '1' COMMENT '是否开启 rtsp 转换',
  `enable_rtmp` tinyint NOT NULL DEFAULT '1' COMMENT '是否开启 rtmp/flv 转换',
  `enable_ts` tinyint NOT NULL DEFAULT '0' COMMENT '是否开启 http-ts 转换',
  `enable_fmp4` tinyint NOT NULL DEFAULT '0' COMMENT '是否开启 http-fmp4 转换',
  `enable_hls` tinyint NOT NULL DEFAULT '1' COMMENT '是否开启 hls 转换',
  `zlm_key` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'ZLMediaKit 代理任务 key',
  `run_status` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'idle' COMMENT '运行状态 idle/running/failed/stopped',
  `last_error` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '最近一次错误',
  `retry_times` int NOT NULL DEFAULT '0' COMMENT '已重试次数',
  `last_active_at` bigint NOT NULL DEFAULT '0' COMMENT '最后活跃时间',
  `created_by` bigint unsigned NOT NULL DEFAULT '0',
  `created_at` bigint NOT NULL DEFAULT '0',
  `updated_at` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_app_stream` (`app`,`stream`),
  KEY `idx_status` (`status`),
  KEY `idx_run_status` (`run_status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='拉流代理任务';

CREATE TABLE IF NOT EXISTS `studio_material` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(128) NOT NULL COMMENT '素材名称',
  `type` varchar(16) NOT NULL DEFAULT 'video' COMMENT 'video=视频素材 logo=Logo图片',
  `file_path` varchar(255) NOT NULL COMMENT '存储相对路径',
  `size` bigint NOT NULL DEFAULT '0' COMMENT '文件大小（字节）',
  `duration` int NOT NULL DEFAULT '0' COMMENT '视频时长（秒）',
  `width` int NOT NULL DEFAULT '0',
  `height` int NOT NULL DEFAULT '0',
  `created_by` bigint unsigned NOT NULL DEFAULT '0',
  `created_at` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  KEY `idx_type` (`type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='制播素材库';

CREATE TABLE IF NOT EXISTS `studio_task` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(128) NOT NULL,
  `mode` varchar(16) NOT NULL DEFAULT 'loop' COMMENT 'loop=视频轮播 relay=拉流压制',
  `material_id` bigint unsigned NOT NULL DEFAULT '0' COMMENT '轮播素材ID（loop模式）',
  `src_url` varchar(512) NOT NULL DEFAULT '' COMMENT '源流地址（relay模式）',
  `app` varchar(64) NOT NULL DEFAULT 'live',
  `stream` varchar(128) NOT NULL COMMENT '输出流ID',
  `caption_enabled` tinyint NOT NULL DEFAULT '0' COMMENT '滚动字幕开关',
  `caption_text` varchar(512) NOT NULL DEFAULT '' COMMENT '字幕内容',
  `caption_size` int NOT NULL DEFAULT '28' COMMENT '字号',
  `caption_color` varchar(16) NOT NULL DEFAULT 'white' COMMENT '字体颜色',
  `caption_position` varchar(8) NOT NULL DEFAULT 'bottom' COMMENT 'top/bottom',
  `caption_speed` int NOT NULL DEFAULT '80' COMMENT '滚动速度 px/秒',
  `logo_material_id` bigint unsigned NOT NULL DEFAULT '0' COMMENT 'Logo素材ID',
  `logo_position` varchar(8) NOT NULL DEFAULT 'tr' COMMENT 'tl/tr/bl/br 四角',
  `logo_opacity` int NOT NULL DEFAULT '100' COMMENT '不透明度百分比',
  `logo_width_pct` int NOT NULL DEFAULT '0' COMMENT 'Logo宽度占画面百分比，0=原始尺寸',
  `vbitrate` int NOT NULL DEFAULT '1500' COMMENT '压制视频码率 kbps',
  `quality_mode` varchar(16) NOT NULL DEFAULT 'bitrate' COMMENT '编码模式 bitrate=固定码率 crf=画质优先',
  `crf` int NOT NULL DEFAULT '21' COMMENT 'CRF 质量因子(0-51，越小越清晰)',
  `status` tinyint NOT NULL DEFAULT '1' COMMENT '1=启用（自动运行）',
  `run_status` varchar(16) NOT NULL DEFAULT 'idle' COMMENT 'idle/running/failed/stopped',
  `pid` int NOT NULL DEFAULT '0',
  `last_error` varchar(1024) NOT NULL DEFAULT '',
  `retry_count` int NOT NULL DEFAULT '-1' COMMENT '最大重启次数 -1=无限',
  `retry_times` int NOT NULL DEFAULT '0',
  `last_active_at` bigint NOT NULL DEFAULT '0',
  `created_by` bigint unsigned NOT NULL DEFAULT '0',
  `created_at` bigint NOT NULL DEFAULT '0',
  `updated_at` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  KEY `idx_stream` (`app`,`stream`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='制播任务';

CREATE TABLE IF NOT EXISTS `sys_config` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `config_key` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL,
  `config_val` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci COMMENT '配置值（JSON 或文本）',
  `remark` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '',
  `updated_at` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_key` (`config_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='全局配置';

CREATE TABLE IF NOT EXISTS `sys_log` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` bigint unsigned NOT NULL DEFAULT '0',
  `username` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '操作人',
  `module` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '模块',
  `action` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '动作',
  `method` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT 'HTTP 方法',
  `path` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '请求路径',
  `ip` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '操作 IP',
  `params` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci COMMENT '请求参数',
  `result` text CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci COMMENT '返回结果摘要',
  `status` tinyint NOT NULL DEFAULT '1' COMMENT '1=成功 0=失败',
  `error_msg` varchar(1024) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '错误信息',
  `duration` int NOT NULL DEFAULT '0' COMMENT '耗时(ms)',
  `created_at` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  KEY `idx_user` (`user_id`),
  KEY `idx_module` (`module`),
  KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='操作日志审计';

CREATE TABLE IF NOT EXISTS `sys_permission` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '权限名称',
  `code` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '权限标识，如 stream:create',
  `type` tinyint NOT NULL DEFAULT '1' COMMENT '类型 1=菜单 2=按钮/接口',
  `parent_id` bigint unsigned NOT NULL DEFAULT '0' COMMENT '父级ID，0为顶级',
  `sort` int NOT NULL DEFAULT '0' COMMENT '排序',
  `remark` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '备注',
  `created_at` bigint NOT NULL DEFAULT '0',
  `updated_at` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_code` (`code`),
  KEY `idx_parent` (`parent_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='权限';

CREATE TABLE IF NOT EXISTS `sys_role` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '角色名称',
  `code` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '角色标识，如 admin/operator/viewer',
  `remark` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '备注',
  `status` tinyint NOT NULL DEFAULT '1' COMMENT '状态 1=启用 0=禁用',
  `created_at` bigint NOT NULL DEFAULT '0',
  `updated_at` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='角色';

CREATE TABLE IF NOT EXISTS `sys_role_permission` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `role_id` bigint unsigned NOT NULL,
  `permission_id` bigint unsigned NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_role_permission` (`role_id`,`permission_id`),
  KEY `idx_permission` (`permission_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='角色权限关联';

CREATE TABLE IF NOT EXISTS `sys_user` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `username` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '登录用户名',
  `password` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT 'bcrypt 密码哈希',
  `nickname` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '昵称',
  `email` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '邮箱',
  `phone` varchar(32) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '手机号',
  `avatar` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '头像地址',
  `status` tinyint NOT NULL DEFAULT '1' COMMENT '状态 1=启用 0=禁用',
  `is_super` tinyint NOT NULL DEFAULT '0' COMMENT '是否超级管理员 1=是（拥有全部权限）',
  `last_login_at` bigint NOT NULL DEFAULT '0' COMMENT '最后登录时间',
  `last_login_ip` varchar(64) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '最后登录 IP',
  `remark` varchar(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '备注',
  `created_at` bigint NOT NULL DEFAULT '0' COMMENT '创建时间',
  `updated_at` bigint NOT NULL DEFAULT '0' COMMENT '更新时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_username` (`username`),
  KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='管理员用户';

CREATE TABLE IF NOT EXISTS `sys_user_role` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` bigint unsigned NOT NULL,
  `role_id` bigint unsigned NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_role` (`user_id`,`role_id`),
  KEY `idx_role` (`role_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='用户角色关联';

CREATE TABLE IF NOT EXISTS `task_error_log` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `task_type` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL COMMENT '类型 proxy/forward',
  `task_id` bigint unsigned NOT NULL DEFAULT '0' COMMENT '任务ID',
  `task_name` varchar(128) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '任务名称',
  `level` varchar(16) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT 'error' COMMENT '级别 error/warn/info',
  `message` varchar(2048) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci NOT NULL DEFAULT '' COMMENT '错误信息',
  `created_at` bigint NOT NULL DEFAULT '0',
  PRIMARY KEY (`id`),
  KEY `idx_task` (`task_type`,`task_id`),
  KEY `idx_created_at` (`created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='任务错误日志';

CREATE TABLE IF NOT EXISTS `traffic_stat` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键',
  `ts` bigint NOT NULL COMMENT '采样时间戳（秒）',
  `recv_bps` bigint NOT NULL DEFAULT '0' COMMENT '入流量速率（字节/秒）',
  `send_bps` bigint NOT NULL DEFAULT '0' COMMENT '出流量速率（字节/秒）',
  `online_streams` int NOT NULL DEFAULT '0' COMMENT '在线流数',
  `readers` int NOT NULL DEFAULT '0' COMMENT '观看人数',
  `created_at` bigint NOT NULL COMMENT '记录时间（Unix秒）',
  PRIMARY KEY (`id`),
  KEY `idx_ts` (`ts`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='流量采样统计表';
