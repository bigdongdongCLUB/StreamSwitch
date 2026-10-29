-- 由迁移器执行：按 ';' + 换行 切分语句；全部为 INSERT IGNORE，可重复执行。

-- 仅新库初始化时执行一次：内置运营/只读角色的默认权限（之后由管理员自行调整，不再覆盖）

INSERT IGNORE INTO `sys_role_permission` (`role_id`, `permission_id`)
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

INSERT IGNORE INTO `sys_role_permission` (`role_id`, `permission_id`)
SELECT 3, id FROM `sys_permission`
WHERE code IN ('stream', 'stream:list', 'proxy', 'proxy:list', 'forward', 'forward:list',
               'domain', 'domain:list', 'format', 'format:list', 'encrypt', 'encrypt:list',
               'token', 'token:list', 'log', 'log:list',
               'studio', 'studio:list', 'livepage', 'livepage:list');
