package repository

import (
	"context"
	"fmt"
	"livems/internal/database"
	"database/sql"
	"strings"
	"time"

	"livems/internal/models"
)

// UserRepo 用户与权限仓储。
type UserRepo struct{}

// NewUserRepo 创建用户仓储。
func NewUserRepo() *UserRepo { return &UserRepo{} }

const userColumns = `id, username, password, nickname, email, phone, avatar, status, is_super,
	last_login_at, last_login_ip, remark, created_at, updated_at`

// Create 新增用户。
func (r *UserRepo) Create(u *models.User) error {
	res, err := exec(`
		INSERT INTO sys_user (username, password, nickname, email, phone, avatar, status, is_super,
			remark, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		u.Username, u.Password, u.Nickname, u.Email, u.Phone, u.Avatar, u.Status, u.IsSuper,
		u.Remark, time.Now().Unix(), time.Now().Unix())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	u.ID = uint64(id)
	return nil
}

// Update 更新用户资料。
func (r *UserRepo) Update(u *models.User) error {
	_, err := exec(`
		UPDATE sys_user SET nickname=?, email=?, phone=?, avatar=?, status=?, is_super=?, remark=?, updated_at=?
		WHERE id=?`,
		u.Nickname, u.Email, u.Phone, u.Avatar, u.Status, u.IsSuper, u.Remark, time.Now().Unix(), u.ID)
	return err
}

// UpdatePassword 更新密码。
func (r *UserRepo) UpdatePassword(id uint64, password string) error {
	_, err := exec(`UPDATE sys_user SET password=?, updated_at=? WHERE id=?`,
		password, time.Now().Unix(), id)
	return err
}

// UpdateLoginInfo 记录登录信息。
func (r *UserRepo) UpdateLoginInfo(id uint64, ip string) error {
	_, err := exec(`UPDATE sys_user SET last_login_at=?, last_login_ip=? WHERE id=?`,
		time.Now().Unix(), ip, id)
	return err
}

// Delete 删除用户。
func (r *UserRepo) Delete(id uint64) error {
	return tx(func(t execTx) error {
		if _, err := t.Exec(`DELETE FROM sys_user_role WHERE user_id=?`, id); err != nil {
			return err
		}
		_, err := t.Exec(`DELETE FROM sys_user WHERE id=?`, id)
		return err
	})
}

// GetByID 按ID查询用户。
func (r *UserRepo) GetByID(id uint64) (*models.User, error) {
	row := queryRow(`SELECT `+userColumns+` FROM sys_user WHERE id=?`, id)
	u, err := scanUser(row)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// GetByUsername 按用户名查询用户。
func (r *UserRepo) GetByUsername(username string) (*models.User, error) {
	row := queryRow(`SELECT `+userColumns+` FROM sys_user WHERE username=?`, username)
	u, err := scanUser(row)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// List 分页查询用户。
func (r *UserRepo) List(page, pageSize int, keyword string) ([]models.User, int64, error) {
	where := []string{"1=1"}
	args := []any{}
	if keyword != "" {
		where = append(where, "(username LIKE ? OR nickname LIKE ?)")
		like := "%" + keyword + "%"
		args = append(args, like, like)
	}
	cond := strings.Join(where, " AND ")

	var total int64
	if err := queryRow(`SELECT COUNT(*) FROM sys_user WHERE `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := query(`SELECT `+userColumns+` FROM sys_user WHERE `+cond+` ORDER BY id ASC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	list := make([]models.User, 0, pageSize)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, *u)
	}
	return list, total, rows.Err()
}

// Roles 获取用户拥有的角色。
func (r *UserRepo) Roles(userID uint64) ([]models.Role, error) {
	rows, err := query(`
		SELECT r.id, r.name, r.code, r.remark, r.status, r.created_at, r.updated_at
		FROM sys_role r
		INNER JOIN sys_user_role ur ON ur.role_id = r.id
		WHERE ur.user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]models.Role, 0)
	for rows.Next() {
		var role models.Role
		if err := rows.Scan(&role.ID, &role.Name, &role.Code, &role.Remark, &role.Status,
			&role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, role)
	}
	return list, rows.Err()
}

// Permissions 获取用户通过角色拥有的权限标识。
func (r *UserRepo) Permissions(userID uint64) ([]string, error) {
	rows, err := query(`
		SELECT DISTINCT p.code
		FROM sys_permission p
		INNER JOIN sys_role_permission rp ON rp.permission_id = p.id
		INNER JOIN sys_user_role ur ON ur.role_id = rp.role_id
		WHERE ur.user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]string, 0)
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		list = append(list, code)
	}
	return list, rows.Err()
}

// AssignRoles 为用户分配角色。
func (r *UserRepo) AssignRoles(userID uint64, roleIDs []uint64) error {
	return tx(func(t execTx) error {
		if _, err := t.Exec(`DELETE FROM sys_user_role WHERE user_id=?`, userID); err != nil {
			return err
		}
		for _, rid := range roleIDs {
			if _, err := t.Exec(`INSERT INTO sys_user_role (user_id, role_id) VALUES (?,?)`, userID, rid); err != nil {
				return err
			}
		}
		return nil
	})
}

// Count 统计用户总数（为 0 表示系统尚未初始化，需要注册首个超级管理员）。
func (r *UserRepo) Count() (int, error) {
	var n int
	err := queryRow(`SELECT COUNT(*) FROM sys_user`).Scan(&n)
	return n, err
}

// CreateFirstAdmin 在「系统中没有任何用户」的前提下创建首个超级管理员并赋予超管角色。
//
// 使用 MySQL 命名锁保证并发注册时只有一个请求成功；已存在用户时返回 created=false。
func (r *UserRepo) CreateFirstAdmin(u *models.User, adminRoleCode string) (created bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := database.DB.Conn(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	var got sql.NullInt64
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK('streamswitch_first_admin', 10)`).Scan(&got); err != nil {
		return false, err
	}
	if !got.Valid || got.Int64 != 1 {
		return false, fmt.Errorf("系统繁忙，请稍后重试")
	}
	defer conn.ExecContext(context.Background(), `SELECT RELEASE_LOCK('streamswitch_first_admin')`)

	var n int
	if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM sys_user`).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	now := time.Now().Unix()
	res, err := conn.ExecContext(ctx, `
		INSERT INTO sys_user (username, password, nickname, email, phone, avatar, status, is_super,
			remark, created_at, updated_at) VALUES (?,?,?,'','','',1,1,'首个注册用户（系统初始化）',?,?)`,
		u.Username, u.Password, u.Nickname, now, now)
	if err != nil {
		return false, err
	}
	id, _ := res.LastInsertId()
	u.ID, u.Status, u.IsSuper, u.CreatedAt, u.UpdatedAt = uint64(id), 1, 1, now, now
	// 赋予内置超管角色（不存在时忽略：is_super=1 已拥有全部权限）
	_, _ = conn.ExecContext(ctx, `INSERT IGNORE INTO sys_user_role (user_id, role_id)
		SELECT ?, id FROM sys_role WHERE code = ?`, u.ID, adminRoleCode)
	return true, nil
}

// CountAdmins 统计超级管理员数量（防止删光导致无法登录）。
func (r *UserRepo) CountAdmins() (int, error) {
	var n int
	err := queryRow(`SELECT COUNT(*) FROM sys_user WHERE is_super=1 AND status=1`).Scan(&n)
	return n, err
}

// scanUser 扫描一行用户记录。
func scanUser(row scanner) (*models.User, error) {
	var u models.User
	err := row.Scan(&u.ID, &u.Username, &u.Password, &u.Nickname, &u.Email, &u.Phone, &u.Avatar,
		&u.Status, &u.IsSuper, &u.LastLoginAt, &u.LastLoginIP, &u.Remark, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// RoleRepo 角色仓储。
type RoleRepo struct{}

// NewRoleRepo 创建角色仓储。
func NewRoleRepo() *RoleRepo { return &RoleRepo{} }

// List 查询全部角色。
func (r *RoleRepo) List() ([]models.Role, error) {
	rows, err := query(`SELECT id, name, code, remark, status, created_at, updated_at FROM sys_role ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]models.Role, 0)
	for rows.Next() {
		var role models.Role
		if err := rows.Scan(&role.ID, &role.Name, &role.Code, &role.Remark, &role.Status,
			&role.CreatedAt, &role.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, role)
	}
	return list, rows.Err()
}

// GetByID 按ID查询角色。
func (r *RoleRepo) GetByID(id uint64) (*models.Role, error) {
	row := queryRow(`SELECT id, name, code, remark, status, created_at, updated_at FROM sys_role WHERE id=?`, id)
	var role models.Role
	err := row.Scan(&role.ID, &role.Name, &role.Code, &role.Remark, &role.Status,
		&role.CreatedAt, &role.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &role, nil
}

// Create 新增角色。
func (r *RoleRepo) Create(role *models.Role) error {
	res, err := exec(`INSERT INTO sys_role (name, code, remark, status, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
		role.Name, role.Code, role.Remark, role.Status, time.Now().Unix(), time.Now().Unix())
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	role.ID = uint64(id)
	return nil
}

// Update 更新角色。
func (r *RoleRepo) Update(role *models.Role) error {
	_, err := exec(`UPDATE sys_role SET name=?, code=?, remark=?, status=?, updated_at=? WHERE id=?`,
		role.Name, role.Code, role.Remark, role.Status, time.Now().Unix(), role.ID)
	return err
}

// Delete 删除角色。
func (r *RoleRepo) Delete(id uint64) error {
	return tx(func(t execTx) error {
		if _, err := t.Exec(`DELETE FROM sys_role_permission WHERE role_id=?`, id); err != nil {
			return err
		}
		if _, err := t.Exec(`DELETE FROM sys_user_role WHERE role_id=?`, id); err != nil {
			return err
		}
		_, err := t.Exec(`DELETE FROM sys_role WHERE id=?`, id)
		return err
	})
}

// AssignPermissions 为角色分配权限。
func (r *RoleRepo) AssignPermissions(roleID uint64, permIDs []uint64) error {
	return tx(func(t execTx) error {
		if _, err := t.Exec(`DELETE FROM sys_role_permission WHERE role_id=?`, roleID); err != nil {
			return err
		}
		for _, pid := range permIDs {
			if _, err := t.Exec(`INSERT INTO sys_role_permission (role_id, permission_id) VALUES (?,?)`, roleID, pid); err != nil {
				return err
			}
		}
		return nil
	})
}

// PermissionIDs 获取角色已分配的权限ID列表。
func (r *RoleRepo) PermissionIDs(roleID uint64) ([]uint64, error) {
	rows, err := query(`SELECT permission_id FROM sys_role_permission WHERE role_id=?`, roleID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]uint64, 0)
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		list = append(list, id)
	}
	return list, rows.Err()
}

// PermissionRepo 权限仓储。
type PermissionRepo struct{}

// NewPermissionRepo 创建权限仓储。
func NewPermissionRepo() *PermissionRepo { return &PermissionRepo{} }

// List 查询全部权限。
func (r *PermissionRepo) List() ([]models.Permission, error) {
	rows, err := query(`SELECT id, name, code, type, parent_id, sort, remark, created_at, updated_at
		FROM sys_permission ORDER BY parent_id ASC, sort ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]models.Permission, 0)
	for rows.Next() {
		var p models.Permission
		if err := rows.Scan(&p.ID, &p.Name, &p.Code, &p.Type, &p.ParentID, &p.Sort, &p.Remark,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

// GetByID 按ID查询权限。
func (r *PermissionRepo) GetByID(id uint64) (*models.Permission, error) {
	row := queryRow(`SELECT id, name, code, type, parent_id, sort, remark, created_at, updated_at
		FROM sys_permission WHERE id=?`, id)
	var p models.Permission
	err := row.Scan(&p.ID, &p.Name, &p.Code, &p.Type, &p.ParentID, &p.Sort, &p.Remark,
		&p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListByIDs 按ID集合查询权限。
func (r *PermissionRepo) ListByIDs(ids []uint64) ([]models.Permission, error) {
	if len(ids) == 0 {
		return []models.Permission{}, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := query(`SELECT id, name, code, type, parent_id, sort, remark, created_at, updated_at
		FROM sys_permission WHERE id IN (`+placeholders+`) ORDER BY parent_id ASC, sort ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	list := make([]models.Permission, 0, len(ids))
	for rows.Next() {
		var p models.Permission
		if err := rows.Scan(&p.ID, &p.Name, &p.Code, &p.Type, &p.ParentID, &p.Sort, &p.Remark,
			&p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

// 确保 sql 包被引用。
var _ = sql.ErrNoRows
