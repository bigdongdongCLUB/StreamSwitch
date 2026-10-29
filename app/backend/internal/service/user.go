package service

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"livems/internal/config"
	"livems/internal/models"
	"livems/internal/pkg/jwt"
	"livems/internal/pkg/logger"
	"livems/internal/repository"
)

// UserService 用户与 RBAC 服务。
type UserService struct {
	userRepo *repository.UserRepo
	roleRepo *repository.RoleRepo
	permRepo *repository.PermissionRepo
	jwtMgr   *jwt.Manager
	cfg      *config.Config
}

// NewUserService 创建用户服务。
func NewUserService(cfg *config.Config) *UserService {
	return &UserService{
		userRepo: repository.NewUserRepo(),
		roleRepo: repository.NewRoleRepo(),
		permRepo: repository.NewPermissionRepo(),
		jwtMgr:   jwt.New(cfg.JWT.Secret, cfg.JWT.Issuer, cfg.JWT.ExpireHour),
		cfg:      cfg,
	}
}

// Login 管理员登录，返回令牌与用户信息。
func (s *UserService) Login(username, password, clientIP string) (string, int64, *models.User, error) {
	u, err := s.userRepo.GetByUsername(username)
	if err != nil {
		// 统一错误提示，避免暴露用户名是否存在
		return "", 0, nil, fmt.Errorf("用户名或密码错误")
	}
	if u.Status != 1 {
		return "", 0, nil, fmt.Errorf("账号已被禁用")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)); err != nil {
		return "", 0, nil, fmt.Errorf("用户名或密码错误")
	}
	token, expireAt := s.jwtMgr.Generate(u.ID, u.Username, u.IsSuper == 1)
	_ = s.userRepo.UpdateLoginInfo(u.ID, clientIP)
	logger.Info("用户登录成功: %s (IP=%s)", u.Username, clientIP)
	return token, expireAt, u, nil
}

// NeedSetup 系统是否尚未初始化（没有任何用户，需要注册首个超级管理员）。
func (s *UserService) NeedSetup() (bool, error) {
	n, err := s.userRepo.Count()
	return n == 0, err
}

// SetupFirstAdmin 注册首个用户并设为超级管理员，成功后直接返回登录令牌。
//
// 仅在系统中没有任何用户时允许调用；之后再调用会返回错误。
func (s *UserService) SetupFirstAdmin(username, password, nickname, clientIP string) (string, int64, *models.User, error) {
	username = strings.TrimSpace(username)
	nickname = strings.TrimSpace(nickname)
	if len(username) < 3 || len(username) > 32 {
		return "", 0, nil, fmt.Errorf("用户名长度需在 3-32 个字符之间")
	}
	for _, c := range username {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.' || c == '@') {
			return "", 0, nil, fmt.Errorf("用户名只能包含字母、数字及 _ - . @")
		}
	}
	if len(password) < 8 {
		return "", 0, nil, fmt.Errorf("超级管理员密码长度不能少于 8 位")
	}
	if nickname == "" {
		nickname = "超级管理员"
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", 0, nil, fmt.Errorf("密码加密失败: %w", err)
	}
	u := &models.User{Username: username, Password: string(hash), Nickname: nickname}
	created, err := s.userRepo.CreateFirstAdmin(u, "admin")
	if err != nil {
		return "", 0, nil, err
	}
	if !created {
		return "", 0, nil, fmt.Errorf("系统已完成初始化，请直接登录")
	}
	logger.Info("系统初始化：已注册首个超级管理员 %s (IP=%s)", u.Username, clientIP)
	token, expireAt := s.jwtMgr.Generate(u.ID, u.Username, true)
	_ = s.userRepo.UpdateLoginInfo(u.ID, clientIP)
	return token, expireAt, u, nil
}

// ResetPassword 按用户名重置密码（命令行运维用），同时启用该账号。
func (s *UserService) ResetPasswordByUsername(username, password string) (*models.User, error) {
	if len(password) < 6 {
		return nil, fmt.Errorf("密码长度不能少于 6 位")
	}
	u, err := s.userRepo.GetByUsername(username)
	if err != nil {
		return nil, fmt.Errorf("用户 %s 不存在", username)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	if err := s.userRepo.UpdatePassword(u.ID, string(hash)); err != nil {
		return nil, err
	}
	if u.Status != 1 {
		u.Status = 1
		if err := s.userRepo.Update(u); err != nil {
			return nil, err
		}
	}
	return u, nil
}

// AllUsers 列出全部用户（命令行运维用）。
func (s *UserService) AllUsers() ([]models.User, error) {
	list, _, err := s.userRepo.List(1, 1000, "")
	return list, err
}

// JWTManager 返回令牌管理器（供中间件使用）。
func (s *UserService) JWTManager() *jwt.Manager { return s.jwtMgr }

// CreateUserRequest 新增用户的请求参数。
type CreateUserRequest struct {
	Username string   `json:"username" binding:"required"`
	Password string   `json:"password" binding:"required"`
	Nickname string   `json:"nickname"`
	Email    string   `json:"email"`
	Phone    string   `json:"phone"`
	Status   int      `json:"status"`
	IsSuper  int      `json:"is_super"`
	RoleIDs  []uint64 `json:"role_ids"`
	Remark   string   `json:"remark"`
}

// Create 新增用户。
func (s *UserService) Create(req *CreateUserRequest) (*models.User, error) {
	if len(req.Username) < 3 || len(req.Username) > 32 {
		return nil, fmt.Errorf("用户名长度需在 3-32 个字符之间")
	}
	if len(req.Password) < 6 {
		return nil, fmt.Errorf("密码长度不能少于 6 位")
	}
	if _, err := s.userRepo.GetByUsername(req.Username); err == nil {
		return nil, fmt.Errorf("用户名已存在")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("密码加密失败: %w", err)
	}
	if req.Status == 0 {
		req.Status = 1
	}
	u := &models.User{
		Username: req.Username,
		Password: string(hash),
		Nickname: req.Nickname,
		Email:    req.Email,
		Phone:    req.Phone,
		Status:   req.Status,
		IsSuper:  req.IsSuper,
		Remark:   req.Remark,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	if err := s.userRepo.Create(u); err != nil {
		return nil, err
	}
	if len(req.RoleIDs) > 0 {
		if err := s.userRepo.AssignRoles(u.ID, req.RoleIDs); err != nil {
			return nil, err
		}
	}
	logger.Info("新增用户: %s", u.Username)
	return u, nil
}

// UpdateUserRequest 更新用户的请求参数。
type UpdateUserRequest struct {
	Nickname string   `json:"nickname"`
	Email    string   `json:"email"`
	Phone    string   `json:"phone"`
	Status   *int     `json:"status"`
	IsSuper  *int     `json:"is_super"`
	RoleIDs  []uint64 `json:"role_ids"`
	Remark   string   `json:"remark"`
}

// Update 更新用户。
func (s *UserService) Update(id uint64, req *UpdateUserRequest) error {
	u, err := s.userRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("用户不存在")
	}
	// 保护超级管理员账号：不允许禁用或降级最后一个超管
	if u.IsSuper == 1 {
		if req.Status != nil && *req.Status == 0 {
			if n, _ := s.userRepo.CountAdmins(); n <= 1 {
				return fmt.Errorf("不能禁用唯一的超级管理员")
			}
		}
		if req.IsSuper != nil && *req.IsSuper == 0 {
			if n, _ := s.userRepo.CountAdmins(); n <= 1 {
				return fmt.Errorf("不能取消唯一的超级管理员")
			}
		}
	}
	if req.Nickname != "" {
		u.Nickname = req.Nickname
	}
	if req.Email != "" {
		u.Email = req.Email
	}
	if req.Phone != "" {
		u.Phone = req.Phone
	}
	if req.Status != nil {
		u.Status = *req.Status
	}
	if req.IsSuper != nil {
		u.IsSuper = *req.IsSuper
	}
	if req.Remark != "" {
		u.Remark = req.Remark
	}
	if err := s.userRepo.Update(u); err != nil {
		return err
	}
	if req.RoleIDs != nil {
		if err := s.userRepo.AssignRoles(id, req.RoleIDs); err != nil {
			return err
		}
	}
	return nil
}

// ResetPassword 重置用户密码。
func (s *UserService) ResetPassword(id uint64, newPassword string) error {
	if len(newPassword) < 6 {
		return fmt.Errorf("密码长度不能少于 6 位")
	}
	if _, err := s.userRepo.GetByID(id); err != nil {
		return fmt.Errorf("用户不存在")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.userRepo.UpdatePassword(id, string(hash))
}

// Delete 删除用户。
func (s *UserService) Delete(id uint64) error {
	u, err := s.userRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("用户不存在")
	}
	if u.IsSuper == 1 {
		if n, _ := s.userRepo.CountAdmins(); n <= 1 {
			return fmt.Errorf("不能删除唯一的超级管理员")
		}
	}
	return s.userRepo.Delete(id)
}

// List 分页查询用户。
func (s *UserService) List(page, pageSize int, keyword string) ([]models.User, int64, error) {
	return s.userRepo.List(page, pageSize, keyword)
}

// Get 按ID查询用户（含角色与权限）。
func (s *UserService) Get(id uint64) (*models.User, error) {
	u, err := s.userRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	roles, _ := s.userRepo.Roles(id)
	perms, _ := s.userRepo.Permissions(id)
	u.Roles = roles
	u.Permissions = perms
	return u, nil
}

// Permissions 返回用户的权限标识集合。
func (s *UserService) Permissions(userID uint64) ([]string, error) {
	u, err := s.userRepo.GetByID(userID)
	if err != nil {
		return nil, err
	}
	// 超级管理员拥有全部权限
	if u.IsSuper == 1 {
		perms, err := s.permRepo.List()
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(perms))
		for _, p := range perms {
			out = append(out, p.Code)
		}
		return out, nil
	}
	return s.userRepo.Permissions(userID)
}

// HasPermission 判断用户是否拥有指定权限。
func (s *UserService) HasPermission(userID uint64, code string) bool {
	perms, err := s.Permissions(userID)
	if err != nil {
		return false
	}
	for _, p := range perms {
		if p == code {
			return true
		}
	}
	return false
}

// ListRoles 查询全部角色。
func (s *UserService) ListRoles() ([]models.Role, error) {
	return s.roleRepo.List()
}

// ListPermissions 查询全部权限。
func (s *UserService) ListPermissions() ([]models.Permission, error) {
	return s.permRepo.List()
}

// CreateRoleRequest 新增角色的请求参数。
type CreateRoleRequest struct {
	Name   string   `json:"name" binding:"required"`
	Code   string   `json:"code" binding:"required"`
	Remark string   `json:"remark"`
	Status int      `json:"status"`
	PermIDs []uint64 `json:"perm_ids"`
}

// CreateRole 新增角色。
func (s *UserService) CreateRole(req *CreateRoleRequest) (*models.Role, error) {
	if strings.TrimSpace(req.Code) == "" {
		return nil, fmt.Errorf("角色标识不能为空")
	}
	if req.Status == 0 {
		req.Status = 1
	}
	role := &models.Role{
		Name:   req.Name,
		Code:   req.Code,
		Remark: req.Remark,
		Status: req.Status,
	}
	if err := s.roleRepo.Create(role); err != nil {
		return nil, err
	}
	if len(req.PermIDs) > 0 {
		if err := s.roleRepo.AssignPermissions(role.ID, req.PermIDs); err != nil {
			return nil, err
		}
	}
	return role, nil
}

// UpdateRole 更新角色。
func (s *UserService) UpdateRole(id uint64, req *CreateRoleRequest) error {
	role, err := s.roleRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("角色不存在")
	}
	if role.Code == "admin" {
		return fmt.Errorf("内置管理员角色不允许修改")
	}
	if req.Name != "" {
		role.Name = req.Name
	}
	if req.Code != "" {
		role.Code = req.Code
	}
	if req.Remark != "" {
		role.Remark = req.Remark
	}
	if req.Status > 0 {
		role.Status = req.Status
	}
	if err := s.roleRepo.Update(role); err != nil {
		return err
	}
	if req.PermIDs != nil {
		return s.roleRepo.AssignPermissions(id, req.PermIDs)
	}
	return nil
}

// DeleteRole 删除角色。
func (s *UserService) DeleteRole(id uint64) error {
	role, err := s.roleRepo.GetByID(id)
	if err != nil {
		return fmt.Errorf("角色不存在")
	}
	if role.Code == "admin" {
		return fmt.Errorf("内置管理员角色不允许删除")
	}
	return s.roleRepo.Delete(id)
}

// RolePermissionIDs 返回角色已分配的权限ID。
func (s *UserService) RolePermissionIDs(roleID uint64) ([]uint64, error) {
	return s.roleRepo.PermissionIDs(roleID)
}
