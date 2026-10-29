-- =============================================================================
--  流媒体管理系统 (Stream Management System) 数据库初始化脚本
--  MySQL 8.0+ / 宝塔面板环境
--
--  用法：
--    mysql -u<user> -p<pwd> streamswitch < sql/schema.sql
--
--  说明：
--    1. 所有表使用 utf8mb4 字符集，兼容中文/emoji
--    2. 不含点播(VOD)、聊天、文件上传相关表
--    3. 时间字段统一使用 BIGINT (Unix 时间戳)，便于与 ZLMediaKit 交互
-- =============================================================================

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- -----------------------------------------------------------------------------
-- 1. 管理员用户表
--    RBAC 权限体系的用户主体，密码使用 bcrypt 哈希存储
-- -----------------------------------------------------------------------------
DROP TABLE IF EXISTS `sys_user`;
CREATE TABLE `sys_user` (
    `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `username`      VARCHAR(64)     NOT NULL                COMMENT '登录用户名',
    `password`      VARCHAR(255)    NOT NULL                COMMENT 'bcrypt 密码哈希',
    `nickname`      VARCHAR(64)     NOT NULL DEFAULT ''     COMMENT '昵称',
    `email`         VARCHAR(128)    NOT NULL DEFAULT ''     COMMENT '邮箱',
    `phone`         VARCHAR(32)     NOT NULL DEFAULT ''     COMMENT '手机号',
    `avatar`        VARCHAR(255)    NOT NULL DEFAULT ''     COMMENT '头像地址',
    `status`        TINYINT         NOT NULL DEFAULT 1      COMMENT '状态 1=启用 0=禁用',
    `is_super`      TINYINT         NOT NULL DEFAULT 0      COMMENT '是否超级管理员 1=是（拥有全部权限）',
    `last_login_at` BIGINT          NOT NULL DEFAULT 0      COMMENT '最后登录时间',
    `last_login_ip` VARCHAR(64)     NOT NULL DEFAULT ''     COMMENT '最后登录 IP',
    `remark`        VARCHAR(255)    NOT NULL DEFAULT ''     COMMENT '备注',
    `created_at`    BIGINT          NOT NULL DEFAULT 0      COMMENT '创建时间',
    `updated_at`    BIGINT          NOT NULL DEFAULT 0      COMMENT '更新时间',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_username` (`username`),
    KEY `idx_status` (`status`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '管理员用户';

-- -----------------------------------------------------------------------------
-- 2. 角色表
-- -----------------------------------------------------------------------------
DROP TABLE IF EXISTS `sys_role`;
CREATE TABLE `sys_role` (
    `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `name`        VARCHAR(64)     NOT NULL                COMMENT '角色名称',
    `code`        VARCHAR(64)     NOT NULL                COMMENT '角色标识，如 admin/operator/viewer',
    `remark`      VARCHAR(255)    NOT NULL DEFAULT ''     COMMENT '备注',
    `status`      TINYINT         NOT NULL DEFAULT 1      COMMENT '状态 1=启用 0=禁用',
    `created_at`  BIGINT          NOT NULL DEFAULT 0,
    `updated_at`  BIGINT          NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_code` (`code`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '角色';

-- -----------------------------------------------------------------------------
-- 3. 权限表（菜单/按钮/接口粒度）
-- -----------------------------------------------------------------------------
DROP TABLE IF EXISTS `sys_permission`;
CREATE TABLE `sys_permission` (
    `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `name`       VARCHAR(64)     NOT NULL                COMMENT '权限名称',
    `code`       VARCHAR(128)    NOT NULL                COMMENT '权限标识，如 stream:create',
    `type`       TINYINT         NOT NULL DEFAULT 1      COMMENT '类型 1=菜单 2=按钮/接口',
    `parent_id`  BIGINT UNSIGNED NOT NULL DEFAULT 0      COMMENT '父级ID，0为顶级',
    `sort`       INT             NOT NULL DEFAULT 0      COMMENT '排序',
    `remark`     VARCHAR(255)    NOT NULL DEFAULT ''     COMMENT '备注',
    `created_at` BIGINT          NOT NULL DEFAULT 0,
    `updated_at` BIGINT          NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_code` (`code`),
    KEY `idx_parent` (`parent_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '权限';

-- -----------------------------------------------------------------------------
-- 4. 用户-角色关联
-- -----------------------------------------------------------------------------
DROP TABLE IF EXISTS `sys_user_role`;
CREATE TABLE `sys_user_role` (
    `id`      BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `user_id` BIGINT UNSIGNED NOT NULL,
    `role_id` BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_user_role` (`user_id`, `role_id`),
    KEY `idx_role` (`role_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '用户角色关联';

-- -----------------------------------------------------------------------------
-- 5. 角色-权限关联
-- -----------------------------------------------------------------------------
DROP TABLE IF EXISTS `sys_role_permission`;
CREATE TABLE `sys_role_permission` (
    `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `role_id`       BIGINT UNSIGNED NOT NULL,
    `permission_id` BIGINT UNSIGNED NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_role_permission` (`role_id`, `permission_id`),
    KEY `idx_permission` (`permission_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '角色权限关联';

-- -----------------------------------------------------------------------------
-- 6. 推流管理（流表）
--    对应 ZLMediaKit 中的一条 MediaSource，app 固定为 live
-- -----------------------------------------------------------------------------
DROP TABLE IF EXISTS `stream`;
CREATE TABLE `stream` (
    `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `name`          VARCHAR(128)    NOT NULL                COMMENT '流名称（展示用）',
    `app`           VARCHAR(64)     NOT NULL DEFAULT 'live' COMMENT 'ZLMediaKit app',
    `stream`        VARCHAR(128)    NOT NULL                COMMENT 'ZLMediaKit 流ID（自定义流Key）',
    `vhost`         VARCHAR(64)     NOT NULL DEFAULT '__defaultVhost__' COMMENT '虚拟主机',
    `push_key`      VARCHAR(128)    NOT NULL DEFAULT ''     COMMENT '推流鉴权Key（自定义流Key）',
    `status`        TINYINT         NOT NULL DEFAULT 1      COMMENT '状态 1=启用 0=禁用',
    `remark`        VARCHAR(255)    NOT NULL DEFAULT ''     COMMENT '备注',
    `online`        TINYINT         NOT NULL DEFAULT 0      COMMENT '在线状态 1=在线 0=离线（由 on_publish/on_play 更新）',
    `publish_ip`    VARCHAR(64)     NOT NULL DEFAULT ''     COMMENT '推流端IP',
    `publish_time`  BIGINT          NOT NULL DEFAULT 0      COMMENT '上线时间',
    `offline_time`  BIGINT          NOT NULL DEFAULT 0      COMMENT '最后下线时间',
    `video_codec`   VARCHAR(32)     NOT NULL DEFAULT ''     COMMENT '视频编码 如 H264/H265',
    `audio_codec`   VARCHAR(32)     NOT NULL DEFAULT ''     COMMENT '音频编码 如 AAC/G711',
    `width`         INT             NOT NULL DEFAULT 0      COMMENT '宽',
    `height`        INT             NOT NULL DEFAULT 0      COMMENT '高',
    `fps`           INT             NOT NULL DEFAULT 0      COMMENT '帧率',
    `bitrate`       INT             NOT NULL DEFAULT 0      COMMENT '总码率 kbps',
    `video_bitrate` INT             NOT NULL DEFAULT 0      COMMENT '视频码率 kbps',
    `audio_bitrate` INT             NOT NULL DEFAULT 0      COMMENT '音频码率 kbps',
    `sample_rate`   INT             NOT NULL DEFAULT 0      COMMENT '音频采样率',
    `reader_count`  INT             NOT NULL DEFAULT 0      COMMENT '当前观看人数',
    `total_reader`  INT             NOT NULL DEFAULT 0      COMMENT '累计观看人次',
    `total_bytes`   BIGINT          NOT NULL DEFAULT 0      COMMENT '累计流量(字节)',
    `created_by`    BIGINT UNSIGNED NOT NULL DEFAULT 0      COMMENT '创建人ID',
    `created_at`    BIGINT          NOT NULL DEFAULT 0,
    `updated_at`    BIGINT          NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_app_stream` (`app`, `stream`),
    KEY `idx_status` (`status`),
    KEY `idx_online` (`online`),
    KEY `idx_publish_time` (`publish_time`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '推流管理';

-- -----------------------------------------------------------------------------
-- 7. 拉流代理任务
--    后台自动从远端源(rtmp/http-flv/hls)拉流并推到本地 ZLMediaKit
-- -----------------------------------------------------------------------------
DROP TABLE IF EXISTS `stream_proxy`;
CREATE TABLE `stream_proxy` (
    `id`             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `name`           VARCHAR(128)    NOT NULL                COMMENT '任务名称',
    `src_url`        VARCHAR(1024)   NOT NULL                COMMENT '远端源地址 rtmp/http-flv/hls',
    `app`            VARCHAR(64)     NOT NULL DEFAULT 'live' COMMENT '本地 app',
    `stream`         VARCHAR(128)    NOT NULL                COMMENT '本地流ID',
    `vhost`          VARCHAR(64)     NOT NULL DEFAULT '__defaultVhost__',
    `status`         TINYINT         NOT NULL DEFAULT 1      COMMENT '状态 1=启用 0=禁用',
    `retry_count`    INT             NOT NULL DEFAULT -1     COMMENT '失败重试次数，-1=无限重试',
    `retry_interval` INT             NOT NULL DEFAULT 10     COMMENT '重试间隔(秒)',
    `timeout_sec`    INT             NOT NULL DEFAULT 10     COMMENT '拉流握手超时(秒)',
    `enable_rtsp`    TINYINT         NOT NULL DEFAULT 1      COMMENT '是否开启 rtsp 转换',
    `enable_rtmp`    TINYINT         NOT NULL DEFAULT 1      COMMENT '是否开启 rtmp/flv 转换',
    `enable_ts`      TINYINT         NOT NULL DEFAULT 0      COMMENT '是否开启 http-ts 转换',
    `enable_fmp4`    TINYINT         NOT NULL DEFAULT 0      COMMENT '是否开启 http-fmp4 转换',
    `enable_hls`     TINYINT         NOT NULL DEFAULT 1      COMMENT '是否开启 hls 转换',
    `zlm_key`        VARCHAR(128)    NOT NULL DEFAULT ''     COMMENT 'ZLMediaKit 代理任务 key',
    `run_status`     VARCHAR(16)     NOT NULL DEFAULT 'idle' COMMENT '运行状态 idle/running/failed/stopped',
    `last_error`     VARCHAR(1024)   NOT NULL DEFAULT ''     COMMENT '最近一次错误',
    `retry_times`    INT             NOT NULL DEFAULT 0      COMMENT '已重试次数',
    `last_active_at` BIGINT          NOT NULL DEFAULT 0      COMMENT '最后活跃时间',
    `created_by`     BIGINT UNSIGNED NOT NULL DEFAULT 0,
    `created_at`     BIGINT          NOT NULL DEFAULT 0,
    `updated_at`     BIGINT          NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_app_stream` (`app`, `stream`),
    KEY `idx_status` (`status`),
    KEY `idx_run_status` (`run_status`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '拉流代理任务';

-- -----------------------------------------------------------------------------
-- 8. 转推流任务
--    一路源流转推到多个 RTMP 目标地址
-- -----------------------------------------------------------------------------
DROP TABLE IF EXISTS `stream_forward`;
CREATE TABLE `stream_forward` (
    `id`            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `name`          VARCHAR(128)    NOT NULL                COMMENT '任务名称',
    `src_app`       VARCHAR(64)     NOT NULL DEFAULT 'live' COMMENT '源 app',
    `src_stream`    VARCHAR(128)    NOT NULL                COMMENT '源流ID',
    `src_vhost`     VARCHAR(64)     NOT NULL DEFAULT '__defaultVhost__',
    `dst_url`       VARCHAR(1024)   NOT NULL                COMMENT 'RTMP 目标地址',
    `schema`        VARCHAR(16)     NOT NULL DEFAULT 'rtmp'  COMMENT '推流协议 rtmp/rtmps',
    `status`        TINYINT         NOT NULL DEFAULT 1      COMMENT '状态 1=启用 0=禁用',
    `run_status`    VARCHAR(16)     NOT NULL DEFAULT 'idle'  COMMENT '运行状态 idle/running/failed/stopped',
    `retry_count`   INT             NOT NULL DEFAULT -1     COMMENT '失败重试次数，-1=无限',
    `timeout_sec`   INT             NOT NULL DEFAULT 10     COMMENT '连接超时(秒)',
    `zlm_key`       VARCHAR(128)    NOT NULL DEFAULT ''     COMMENT 'ZLMediaKit 任务 key',
    `retry_times`   INT             NOT NULL DEFAULT 0      COMMENT '已重试次数',
    `last_error`    VARCHAR(1024)   NOT NULL DEFAULT ''     COMMENT '最近一次错误',
    `last_active_at` BIGINT         NOT NULL DEFAULT 0      COMMENT '最后活跃时间',
    `created_by`    BIGINT UNSIGNED NOT NULL DEFAULT 0,
    `created_at`    BIGINT          NOT NULL DEFAULT 0,
    `updated_at`    BIGINT          NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_dst` (`dst_url`(255), `src_stream`),
    KEY `idx_src` (`src_app`, `src_stream`),
    KEY `idx_status` (`status`),
    KEY `idx_run_status` (`run_status`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '转推流任务';

-- -----------------------------------------------------------------------------
-- 9. 播放域名管理
--    可在后台新增/启用/禁用多个播放域名，设置默认域名
--    配置存库，改配置无需重启服务
-- -----------------------------------------------------------------------------
DROP TABLE IF EXISTS `play_domain`;
CREATE TABLE `play_domain` (
    `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `name`        VARCHAR(128)    NOT NULL                COMMENT '域名名称',
    `schema`      VARCHAR(16)     NOT NULL DEFAULT 'http'  COMMENT '协议 http/https',
    `host`        VARCHAR(255)    NOT NULL                COMMENT '主机名，如 live.example.com',
    `port`        INT             NOT NULL DEFAULT 80      COMMENT '端口，https 默认 443',
    `is_default`  TINYINT         NOT NULL DEFAULT 0      COMMENT '是否默认域名 1=是',
    `status`      TINYINT         NOT NULL DEFAULT 1      COMMENT '状态 1=启用 0=禁用',
    `sort`        INT             NOT NULL DEFAULT 0      COMMENT '排序',
    `remark`      VARCHAR(255)    NOT NULL DEFAULT ''     COMMENT '备注',
    `created_at`  BIGINT          NOT NULL DEFAULT 0,
    `updated_at`  BIGINT          NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_host` (`host`),
    KEY `idx_status` (`status`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '播放域名';

-- -----------------------------------------------------------------------------
-- 10. 播放格式开关（全局）
--     后台可单独开关每种格式是否对外分发
-- -----------------------------------------------------------------------------
DROP TABLE IF EXISTS `play_format`;
CREATE TABLE `play_format` (
    `id`          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `format`      VARCHAR(16)     NOT NULL                COMMENT '格式 rtmp/http-flv/ws-flv/hls',
    `name`        VARCHAR(64)     NOT NULL                COMMENT '显示名称',
    `enabled`     TINYINT         NOT NULL DEFAULT 1      COMMENT '是否对外分发 1=开启 0=关闭',
    `path_prefix` VARCHAR(128)    NOT NULL DEFAULT ''     COMMENT 'URL 路径前缀，如 /live',
    `sort`        INT             NOT NULL DEFAULT 0,
    `remark`      VARCHAR(255)    NOT NULL DEFAULT ''     COMMENT '备注',
    `created_at`  BIGINT          NOT NULL DEFAULT 0,
    `updated_at`  BIGINT          NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_format` (`format`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '播放格式开关';

-- -----------------------------------------------------------------------------
-- 11. 全局配置（键值对）
--     存放鉴权策略、加密策略等运行时可改配置
-- -----------------------------------------------------------------------------
DROP TABLE IF EXISTS `sys_config`;
CREATE TABLE `sys_config` (
    `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `config_key` VARCHAR(128)    NOT NULL,
    `config_val` TEXT                     DEFAULT NULL COMMENT '配置值（JSON 或文本）',
    `remark`     VARCHAR(255)    NOT NULL DEFAULT '',
    `updated_at` BIGINT          NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_key` (`config_key`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '全局配置';

-- -----------------------------------------------------------------------------
-- 12. HLS 加密配置（每流独立）
--     支持密钥过期策略
-- -----------------------------------------------------------------------------
DROP TABLE IF EXISTS `hls_encrypt`;
CREATE TABLE `hls_encrypt` (
    `id`             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `stream_id`      BIGINT UNSIGNED NOT NULL                COMMENT '关联的流ID',
    `enabled`        TINYINT         NOT NULL DEFAULT 0      COMMENT '是否开启 HLS AES-128 加密 1=开启',
    `key_hex`        VARCHAR(64)     NOT NULL DEFAULT ''     COMMENT 'AES-128 密钥(32位hex=16字节)',
    `key_uri`        VARCHAR(255)    NOT NULL DEFAULT ''     COMMENT '密钥下发路径，如 /hls-key/{stream}.key',
    `key_expire_sec` INT             NOT NULL DEFAULT 0      COMMENT '密钥有效期(秒)，0=永久有效',
    `iv_strategy`    VARCHAR(16)     NOT NULL DEFAULT 'seq'  COMMENT 'IV 策略 seq=使用媒体序号(符合RFC8216) random=随机',
    `expire_at`      BIGINT          NOT NULL DEFAULT 0      COMMENT '密钥过期时间戳，0=不过期',
    `last_rotate_at` BIGINT          NOT NULL DEFAULT 0      COMMENT '上次轮换时间',
    `created_at`     BIGINT          NOT NULL DEFAULT 0,
    `updated_at`     BIGINT          NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_stream` (`stream_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = 'HLS AES-128 加密配置';

-- -----------------------------------------------------------------------------
-- 12.1 流量采样统计
--     随 ZLM 同步周期（默认 30s）采样入/出流量速率，供仪表盘趋势图使用
-- -----------------------------------------------------------------------------
DROP TABLE IF EXISTS `traffic_stat`;
CREATE TABLE `traffic_stat` (
    `id`             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT COMMENT '主键',
    `ts`             BIGINT          NOT NULL                COMMENT '采样时间戳（秒）',
    `recv_bps`       BIGINT          NOT NULL DEFAULT 0      COMMENT '入流量速率（字节/秒，推流接收方向）',
    `send_bps`       BIGINT          NOT NULL DEFAULT 0      COMMENT '出流量速率（字节/秒，播放分发方向，按 码率×观看数 估算）',
    `online_streams` INT             NOT NULL DEFAULT 0      COMMENT '在线流数',
    `readers`        INT             NOT NULL DEFAULT 0      COMMENT '观看人数',
    `created_at`     BIGINT          NOT NULL                COMMENT '记录时间（Unix秒）',
    PRIMARY KEY (`id`),
    KEY `idx_ts` (`ts`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '流量采样统计表';

-- -----------------------------------------------------------------------------
-- 12.2 制播素材库 / 制播任务
--     本地视频轮播播出、拉流压制（滚动字幕 / Logo 水印叠加）
-- -----------------------------------------------------------------------------
DROP TABLE IF EXISTS `studio_material`;
CREATE TABLE `studio_material` (
    `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `name`       VARCHAR(128)    NOT NULL,
    `type`       VARCHAR(16)     NOT NULL DEFAULT 'video'  COMMENT 'video=视频素材 logo=Logo图片',
    `file_path`  VARCHAR(255)    NOT NULL                  COMMENT '存储相对路径',
    `size`       BIGINT          NOT NULL DEFAULT 0        COMMENT '文件大小（字节）',
    `duration`   INT             NOT NULL DEFAULT 0        COMMENT '视频时长（秒）',
    `width`      INT             NOT NULL DEFAULT 0,
    `height`     INT             NOT NULL DEFAULT 0,
    `created_by` BIGINT UNSIGNED NOT NULL DEFAULT 0,
    `created_at` BIGINT          NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`),
    KEY `idx_type` (`type`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '制播素材库';

DROP TABLE IF EXISTS `studio_task`;
CREATE TABLE `studio_task` (
    `id`               BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `name`             VARCHAR(128)    NOT NULL,
    `mode`             VARCHAR(16)     NOT NULL DEFAULT 'loop' COMMENT 'loop=视频轮播 relay=拉流压制',
    `material_id`      BIGINT UNSIGNED NOT NULL DEFAULT 0      COMMENT '轮播素材ID（loop模式）',
    `src_url`          VARCHAR(512)    NOT NULL DEFAULT ''     COMMENT '源流地址（relay模式）',
    `app`              VARCHAR(64)     NOT NULL DEFAULT 'live',
    `stream`           VARCHAR(128)    NOT NULL                  COMMENT '输出流ID',
    `caption_enabled`  TINYINT         NOT NULL DEFAULT 0        COMMENT '滚动字幕开关',
    `caption_text`     VARCHAR(512)    NOT NULL DEFAULT ''       COMMENT '字幕内容',
    `caption_size`     INT             NOT NULL DEFAULT 28       COMMENT '字号',
    `caption_color`    VARCHAR(16)     NOT NULL DEFAULT 'white'  COMMENT '字体颜色',
    `caption_position` VARCHAR(8)      NOT NULL DEFAULT 'bottom' COMMENT 'top/bottom',
    `caption_speed`    INT             NOT NULL DEFAULT 80       COMMENT '滚动速度 px/秒',
    `logo_material_id` BIGINT UNSIGNED NOT NULL DEFAULT 0        COMMENT 'Logo素材ID',
    `logo_position`    VARCHAR(8)      NOT NULL DEFAULT 'tr'     COMMENT 'tl/tr/bl/br 四角',
    `logo_opacity`     INT             NOT NULL DEFAULT 100      COMMENT '不透明度百分比',
    `logo_width_pct`   INT             NOT NULL DEFAULT 0        COMMENT 'Logo宽度占画面百分比，0=原始尺寸',
    `vbitrate`         INT             NOT NULL DEFAULT 1500     COMMENT '压制视频码率 kbps（CRF 模式下作为最大码率上限）',
    `quality_mode`     VARCHAR(16)     NOT NULL DEFAULT 'bitrate' COMMENT '编码模式 bitrate=固定码率 crf=画质优先',
    `crf`              INT             NOT NULL DEFAULT 21       COMMENT 'CRF 质量因子(0-51，越小越清晰)',
    `status`           TINYINT         NOT NULL DEFAULT 1        COMMENT '1=启用（自动运行）',
    `run_status`       VARCHAR(16)     NOT NULL DEFAULT 'idle'   COMMENT 'idle/running/failed/stopped',
    `pid`              INT             NOT NULL DEFAULT 0,
    `last_error`       VARCHAR(1024)   NOT NULL DEFAULT '',
    `retry_count`      INT             NOT NULL DEFAULT -1       COMMENT '最大重启次数 -1=无限',
    `retry_times`      INT             NOT NULL DEFAULT 0,
    `last_active_at`   BIGINT          NOT NULL DEFAULT 0,
    `created_by`       BIGINT UNSIGNED NOT NULL DEFAULT 0,
    `created_at`       BIGINT          NOT NULL DEFAULT 0,
    `updated_at`       BIGINT          NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`),
    KEY `idx_stream` (`app`, `stream`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '制播任务';

-- -----------------------------------------------------------------------------
-- 12.3 直播分享页
--     观众免登录通过 /p/<code> 访问观看直播，可选访问密码（bcrypt 存储）
-- -----------------------------------------------------------------------------
DROP TABLE IF EXISTS `live_page`;
CREATE TABLE `live_page` (
    `id`           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `name`         VARCHAR(128)    NOT NULL,
    `code`         VARCHAR(32)     NOT NULL                COMMENT '访问码（URL 标识）',
    `stream_id`    BIGINT UNSIGNED NOT NULL                COMMENT '关联流ID',
    `password`     VARCHAR(128)    NOT NULL DEFAULT ''     COMMENT '访问密码（明文存储，管理端可见，空=公开）',
    `announcement` VARCHAR(512)    NOT NULL DEFAULT ''     COMMENT '页面公告/简介',
    `status`       TINYINT         NOT NULL DEFAULT 1      COMMENT '1=启用 0=禁用',
    `view_count`   BIGINT          NOT NULL DEFAULT 0      COMMENT '累计访问次数',
    `created_by`   BIGINT UNSIGNED NOT NULL DEFAULT 0,
    `created_at`   BIGINT          NOT NULL DEFAULT 0,
    `updated_at`   BIGINT          NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_code` (`code`),
    KEY `idx_stream` (`stream_id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '直播分享页';

-- -----------------------------------------------------------------------------
-- 13. 播放 Token
--     Redis 为主存储(带TTL)，MySQL 留档用于审计
-- -----------------------------------------------------------------------------
DROP TABLE IF EXISTS `play_token`;
CREATE TABLE `play_token` (
    `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `token`      VARCHAR(128)    NOT NULL                COMMENT 'Token 明文(建议只存哈希，保留明文便于排查)',
    `stream_id`  BIGINT UNSIGNED NOT NULL DEFAULT 0      COMMENT '关联流ID，0=不限流',
    `app`        VARCHAR(64)     NOT NULL DEFAULT ''     COMMENT 'app',
    `stream`     VARCHAR(128)    NOT NULL DEFAULT ''     COMMENT '流ID',
    `expire_at`  BIGINT          NOT NULL DEFAULT 0      COMMENT '过期时间戳',
    `ip_bind`    VARCHAR(255)    NOT NULL DEFAULT ''     COMMENT 'IP绑定，空=不限制；多个用逗号分隔',
    `format`     VARCHAR(16)     NOT NULL DEFAULT ''     COMMENT '限定格式，空=全部格式共用',
    `created_by` BIGINT UNSIGNED NOT NULL DEFAULT 0,
    `created_at` BIGINT          NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_token` (`token`),
    KEY `idx_stream` (`stream_id`),
    KEY `idx_expire` (`expire_at`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '播放Token';

-- -----------------------------------------------------------------------------
-- 14. 操作日志审计
-- -----------------------------------------------------------------------------
DROP TABLE IF EXISTS `sys_log`;
CREATE TABLE `sys_log` (
    `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `user_id`    BIGINT UNSIGNED NOT NULL DEFAULT 0,
    `username`   VARCHAR(64)     NOT NULL DEFAULT ''     COMMENT '操作人',
    `module`     VARCHAR(64)     NOT NULL DEFAULT ''     COMMENT '模块',
    `action`     VARCHAR(64)     NOT NULL DEFAULT ''     COMMENT '动作',
    `method`     VARCHAR(16)     NOT NULL DEFAULT ''     COMMENT 'HTTP 方法',
    `path`       VARCHAR(255)    NOT NULL DEFAULT ''     COMMENT '请求路径',
    `ip`         VARCHAR(64)     NOT NULL DEFAULT ''     COMMENT '操作 IP',
    `params`     TEXT                     DEFAULT NULL COMMENT '请求参数',
    `result`     TEXT                     DEFAULT NULL COMMENT '返回结果摘要',
    `status`     TINYINT         NOT NULL DEFAULT 1      COMMENT '1=成功 0=失败',
    `error_msg`  VARCHAR(1024)   NOT NULL DEFAULT ''     COMMENT '错误信息',
    `duration`   INT             NOT NULL DEFAULT 0      COMMENT '耗时(ms)',
    `created_at` BIGINT          NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`),
    KEY `idx_user` (`user_id`),
    KEY `idx_module` (`module`),
    KEY `idx_created_at` (`created_at`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '操作日志审计';

-- -----------------------------------------------------------------------------
-- 15. 推流/拉流/转推 错误日志
-- -----------------------------------------------------------------------------
DROP TABLE IF EXISTS `task_error_log`;
CREATE TABLE `task_error_log` (
    `id`         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    `task_type`  VARCHAR(16)     NOT NULL                COMMENT '类型 proxy/forward',
    `task_id`    BIGINT UNSIGNED NOT NULL DEFAULT 0      COMMENT '任务ID',
    `task_name`  VARCHAR(128)    NOT NULL DEFAULT ''     COMMENT '任务名称',
    `level`      VARCHAR(16)     NOT NULL DEFAULT 'error' COMMENT '级别 error/warn/info',
    `message`    VARCHAR(2048)   NOT NULL DEFAULT ''     COMMENT '错误信息',
    `created_at` BIGINT          NOT NULL DEFAULT 0,
    PRIMARY KEY (`id`),
    KEY `idx_task` (`task_type`, `task_id`),
    KEY `idx_created_at` (`created_at`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_unicode_ci COMMENT = '任务错误日志';

-- =============================================================================
--  初始化数据
-- =============================================================================

-- 角色
INSERT INTO `sys_role` (`id`, `name`, `code`, `remark`, `status`, `created_at`, `updated_at`) VALUES
(1, '超级管理员', 'admin', '拥有全部权限', 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(2, '运营人员', 'operator', '日常流管理操作', 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(3, '只读观察员', 'viewer', '仅查看', 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP());

-- 权限
INSERT INTO `sys_permission` (`id`, `name`, `code`, `type`, `parent_id`, `sort`, `created_at`, `updated_at`) VALUES
(1,  '流管理',   'stream',            1, 0, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(2,  '查看流',   'stream:list',       2, 1, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(3,  '新增流',   'stream:create',     2, 1, 2, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(4,  '编辑流',   'stream:update',     2, 1, 3, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(5,  '删除流',   'stream:delete',     2, 1, 4, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(6,  '启停流',   'stream:toggle',     2, 1, 5, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(10, '拉流代理', 'proxy',             1, 0, 2, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(11, '查看代理', 'proxy:list',        2, 10, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(12, '新增代理', 'proxy:create',      2, 10, 2, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(13, '编辑代理', 'proxy:update',      2, 10, 3, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(14, '删除代理', 'proxy:delete',      2, 10, 4, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(15, '启停代理', 'proxy:toggle',      2, 10, 5, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(20, '转推流',   'forward',           1, 0, 3, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(21, '查看转推', 'forward:list',      2, 20, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(22, '新增转推', 'forward:create',    2, 20, 2, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(23, '编辑转推', 'forward:update',    2, 20, 3, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(24, '删除转推', 'forward:delete',    2, 20, 4, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(25, '启停转推', 'forward:toggle',    2, 20, 5, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(30, '域名管理', 'domain',            1, 0, 4, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(31, '查看域名', 'domain:list',       2, 30, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(32, '新增域名', 'domain:create',     2, 30, 2, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(33, '编辑域名', 'domain:update',     2, 30, 3, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(34, '删除域名', 'domain:delete',     2, 30, 4, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(40, '格式开关', 'format',            1, 0, 5, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(41, '查看格式', 'format:list',       2, 40, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(42, '编辑格式', 'format:update',     2, 40, 2, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(50, '加密管理', 'encrypt',           1, 0, 6, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(51, '查看加密', 'encrypt:list',      2, 50, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(52, '编辑加密', 'encrypt:update',    2, 50, 2, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(60, '令牌管理', 'token',             1, 0, 7, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(61, '查看令牌', 'token:list',        2, 60, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(62, '生成令牌', 'token:create',      2, 60, 2, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(63, '删除令牌', 'token:delete',      2, 60, 3, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(70, '用户管理', 'user',              1, 0, 8, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(71, '查看用户', 'user:list',         2, 70, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(72, '新增用户', 'user:create',       2, 70, 2, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(73, '编辑用户', 'user:update',       2, 70, 3, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(74, '删除用户', 'user:delete',       2, 70, 4, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(75, '重置密码', 'user:resetpwd',     2, 70, 5, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(80, '角色权限', 'role',              1, 0, 9, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(81, '查看角色', 'role:list',         2, 80, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(82, '新增角色', 'role:create',       2, 80, 2, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(83, '编辑角色', 'role:update',       2, 80, 3, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(84, '删除角色', 'role:delete',       2, 80, 4, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(85, '分配权限', 'role:assign',       2, 80, 5, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(90, '日志审计', 'log',               1, 0, 10, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(91, '查看日志', 'log:list',          2, 90, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(95, '制播管理', 'studio',            1, 0, 3, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(96, '查看制播', 'studio:list',       2, 95, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(97, '新建制播', 'studio:create',     2, 95, 2, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(98, '编辑制播', 'studio:update',     2, 95, 3, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(99, '删除制播', 'studio:delete',     2, 95, 4, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(100, '启停制播', 'studio:toggle',    2, 95, 5, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(101, '直播管理', 'livepage',           1, 0, 2, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(102, '查看直播页', 'livepage:list',    2, 101, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(103, '新建直播页', 'livepage:create',  2, 101, 2, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(104, '编辑直播页', 'livepage:update',  2, 101, 3, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(105, '删除直播页', 'livepage:delete',  2, 101, 4, UNIX_TIMESTAMP(), UNIX_TIMESTAMP());

-- 角色-权限：超管全部
INSERT INTO `sys_role_permission` (`role_id`, `permission_id`)
SELECT 1, id FROM `sys_permission`;

-- 角色-权限：运营人员（流/代理/转推/域名/格式/加密/令牌 可操作，日志只读）
INSERT INTO `sys_role_permission` (`role_id`, `permission_id`)
SELECT 2, id FROM `sys_permission`
WHERE code IN ('stream', 'stream:list', 'stream:create', 'stream:update', 'stream:delete', 'stream:toggle',
               'proxy', 'proxy:list', 'proxy:create', 'proxy:update', 'proxy:delete', 'proxy:toggle',
               'forward', 'forward:list', 'forward:create', 'forward:update', 'forward:delete', 'forward:toggle',
               'domain', 'domain:list', 'domain:create', 'domain:update', 'domain:delete',
               'format', 'format:list', 'format:update',
               'encrypt', 'encrypt:list', 'encrypt:update',
               'token', 'token:list', 'token:create', 'token:delete',
               'log', 'log:list',
               'studio', 'studio:list', 'studio:create', 'studio:update', 'studio:delete', 'studio:toggle',
               'livepage', 'livepage:list', 'livepage:create', 'livepage:update', 'livepage:delete');

-- 角色-权限：只读观察员
INSERT INTO `sys_role_permission` (`role_id`, `permission_id`)
SELECT 3, id FROM `sys_permission`
WHERE code IN ('stream', 'stream:list', 'proxy', 'proxy:list', 'forward', 'forward:list',
               'domain', 'domain:list', 'format', 'format:list', 'encrypt', 'encrypt:list',
               'token', 'token:list', 'log', 'log:list',
               'studio', 'studio:list', 'livepage', 'livepage:list');

-- 播放格式（默认全部开启）
INSERT INTO `play_format` (`format`, `name`, `enabled`, `path_prefix`, `sort`, `remark`, `created_at`, `updated_at`) VALUES
('rtmp',     'RTMP',     1, '/live', 1, 'RTMP 播放地址', UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
('http-flv', 'HTTP-FLV', 1, '/live', 2, 'HTTP-FLV 播放地址', UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
('ws-flv',   'WS-FLV',   1, '/live', 3, 'WebSocket-FLV 播放地址', UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
('hls',      'HLS',      1, '/live', 4, 'HLS(m3u8) 播放地址', UNIX_TIMESTAMP(), UNIX_TIMESTAMP());

-- 全局配置
INSERT INTO `sys_config` (`config_key`, `config_val`, `remark`, `updated_at`) VALUES
('auth', '{"enabled":true,"token_expire":3600,"ip_bind_enabled":false,"param_name":"token","allow_empty":false}', '播放鉴权策略', UNIX_TIMESTAMP()),
('hls_encrypt', '{"enabled":false,"key_expire":0,"iv_strategy":"seq"}', 'HLS 加密全局默认策略', UNIX_TIMESTAMP()),
('zlm', '{"app":"live","vhost":"__defaultVhost__","sync_interval":30}', 'ZLMediaKit 同步参数', UNIX_TIMESTAMP());

SET FOREIGN_KEY_CHECKS = 1;

-- 默认管理员账号
--   用户名：admin   密码：admin123
--   （首次登录后请立即在后台修改密码）
--   哈希由 `go run ./cmd/server -init-admin` 生成
-- =============================================================================
INSERT INTO `sys_user` (`id`, `username`, `password`, `nickname`, `status`, `is_super`, `created_at`, `updated_at`) VALUES
(1, 'admin', '$2a$10$X7WkvCiMIaDSNWhVMcT7veaJKnXenrFl5BSJbgBEeRf9v3Phm3DU6', '超级管理员', 1, 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP());

INSERT INTO `sys_user_role` (`user_id`, `role_id`) VALUES (1, 1);
