-- 由迁移器执行：按 ';' + 换行 切分语句；全部为 INSERT IGNORE，可重复执行。

-- 每次启动执行：补齐内置角色/权限，超管角色始终拥有全部权限，补齐默认播放格式与全局配置

INSERT IGNORE INTO `sys_role` (`id`, `name`, `code`, `remark`, `status`, `created_at`, `updated_at`) VALUES
(1, '超级管理员', 'admin', '拥有全部权限', 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(2, '运营人员', 'operator', '日常流管理操作', 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
(3, '只读观察员', 'viewer', '仅查看', 1, UNIX_TIMESTAMP(), UNIX_TIMESTAMP());

INSERT IGNORE INTO `sys_permission` (`id`, `name`, `code`, `type`, `parent_id`, `sort`, `created_at`, `updated_at`) VALUES
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

INSERT IGNORE INTO `sys_role_permission` (`role_id`, `permission_id`)
SELECT 1, id FROM `sys_permission`;

INSERT IGNORE INTO `play_format` (`format`, `name`, `enabled`, `path_prefix`, `sort`, `remark`, `created_at`, `updated_at`) VALUES
('rtmp',     'RTMP',     1, '/live', 1, 'RTMP 播放地址', UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
('http-flv', 'HTTP-FLV', 1, '/live', 2, 'HTTP-FLV 播放地址', UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
('ws-flv',   'WS-FLV',   1, '/live', 3, 'WebSocket-FLV 播放地址', UNIX_TIMESTAMP(), UNIX_TIMESTAMP()),
('hls',      'HLS',      1, '/live', 4, 'HLS(m3u8) 播放地址', UNIX_TIMESTAMP(), UNIX_TIMESTAMP());

INSERT IGNORE INTO `sys_config` (`config_key`, `config_val`, `remark`, `updated_at`) VALUES
('auth', '{"enabled":true,"token_expire":3600,"ip_bind_enabled":false,"param_name":"token","allow_empty":false}', '播放鉴权策略', UNIX_TIMESTAMP()),
('hls_encrypt', '{"enabled":false,"key_expire":0,"iv_strategy":"seq"}', 'HLS 加密全局默认策略', UNIX_TIMESTAMP()),
('zlm', '{"app":"live","vhost":"__defaultVhost__","sync_interval":30}', 'ZLMediaKit 同步参数', UNIX_TIMESTAMP());
