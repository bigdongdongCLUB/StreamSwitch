package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"livems/internal/config"
	"livems/internal/database"
	"livems/internal/models"
	"livems/internal/pkg/jwt"
	"livems/internal/repository"
)

// livePageTicketTTL 观众访问凭证有效期（2 小时）。
const livePageTicketTTL = 2 * time.Hour

// 观众播放令牌有效期（4 小时，覆盖一场直播）。
const livePagePlayTokenSec = 4 * 3600

// LivePageService 直播分享页服务。
type LivePageService struct {
	cfg       *config.Config
	repo      *repository.LivePageRepo
	streamRepo *repository.StreamRepo
	playSvc   *PlayService
	authSvc   *AuthService
}

// NewLivePageService 创建直播分享页服务。
func NewLivePageService(cfg *config.Config, playSvc *PlayService, authSvc *AuthService) *LivePageService {
	return &LivePageService{
		cfg:        cfg,
		repo:       repository.NewLivePageRepo(),
		streamRepo: repository.NewStreamRepo(),
		playSvc:    playSvc,
		authSvc:    authSvc,
	}
}

// codePattern 访问码规则：字母数字中划线，4-32 位。
var codePattern = regexp.MustCompile(`^[A-Za-z0-9-]{4,32}$`)

// LivePageRequest 创建/更新请求。
type LivePageRequest struct {
	Name         string `json:"name" binding:"required"`
	StreamID     uint64 `json:"stream_id" binding:"required"`
	Password     string `json:"password"` // 明文存储（管理端可见）；空=公开
	Announcement string `json:"announcement"`
	Status       int    `json:"status"`
}

// Create 创建直播页。
func (s *LivePageService) Create(req *LivePageRequest, userID uint64) (*models.LivePage, error) {
	st, err := s.streamRepo.GetByID(req.StreamID)
	if err != nil {
		return nil, fmt.Errorf("关联的流不存在")
	}
	p := &models.LivePage{
		Name:         strings.TrimSpace(req.Name),
		Code:         genPageCode(),
		StreamID:     st.ID,
		Announcement: req.Announcement,
		Status:       req.Status,
		CreatedBy:    userID,
	}
	if p.Status == 0 {
		p.Status = 1
	}
	p.Password = req.Password
	if err := s.repo.Create(p); err != nil {
		return nil, err
	}
	return p, nil
}

// Update 更新直播页。
func (s *LivePageService) Update(id uint64, req *LivePageRequest) error {
	p, err := s.repo.GetByID(id)
	if err != nil {
		return err
	}
	if _, err := s.streamRepo.GetByID(req.StreamID); err != nil {
		return fmt.Errorf("关联的流不存在")
	}
	p.Name = strings.TrimSpace(req.Name)
	p.StreamID = req.StreamID
	p.Announcement = req.Announcement
	p.Status = req.Status
	// 密码明文存储（管理端可见），空=公开访问
	p.Password = req.Password
	return s.repo.Update(p)
}

// Delete 删除直播页。
func (s *LivePageService) Delete(id uint64) error {
	return s.repo.Delete(id)
}

// Get 按 ID 查询（管理端）。
func (s *LivePageService) Get(id uint64) (*models.LivePage, error) {
	return s.repo.GetByID(id)
}

// List 分页查询（管理端）。
func (s *LivePageService) List(page, pageSize int, keyword string) ([]models.LivePage, int64, error) {
	list, total, err := s.repo.List(page, pageSize, keyword)
	for i := range list {
		list[i].HasPasswordView = list[i].HasPassword()
	}
	return list, total, err
}

// ---------------------------------------------------------------------------
// 观众侧（免登录）
// ---------------------------------------------------------------------------

// PublicInfo 观众侧的页面信息（不含敏感字段）。
type PublicInfo struct {
	Name         string `json:"name"`
	Code         string `json:"code"`
	NeedPassword bool   `json:"need_password"`
	Announcement string `json:"announcement"`
	Online       bool   `json:"online"`
	StreamName   string `json:"stream_name"`
}

// GetPublicInfo 获取页面公开信息（不泄露流地址与密码）。
func (s *LivePageService) GetPublicInfo(code string) (*PublicInfo, error) {
	p, err := s.repo.GetByCode(code)
	if err != nil {
		return nil, err
	}
	if p.Status != 1 {
		return nil, fmt.Errorf("该直播页已停用")
	}
	st, err := s.streamRepo.GetByID(p.StreamID)
	if err != nil {
		return nil, fmt.Errorf("关联的流不存在")
	}
	return &PublicInfo{
		Name:         p.Name,
		Code:         p.Code,
		NeedPassword: p.HasPassword(),
		Announcement: p.Announcement,
		Online:       st.Online == 1,
		StreamName:   st.Name,
	}, nil
}

// ticketKey 观众凭证 Redis 键。
func (s *LivePageService) ticketKey(ticket string) string {
	return database.RedisKey(s.cfg.Redis.Prefix, "livepage", ticket)
}

// VerifyPassword 校验访问密码，成功返回访问凭证（2 小时有效）。
func (s *LivePageService) VerifyPassword(code, password string) (string, error) {
	p, err := s.repo.GetByCode(code)
	if err != nil {
		return "", err
	}
	if p.Status != 1 {
		return "", fmt.Errorf("该直播页已停用")
	}
	if !p.HasPassword() {
		return "", fmt.Errorf("该直播页无需密码")
	}
	// 兼容历史 bcrypt 哈希数据（$2a$/$2b$ 前缀）：此类记录需管理员重置密码
	if strings.HasPrefix(p.Password, "$2") {
		return "", fmt.Errorf("该直播页密码为旧版加密存储，请联系管理员在后台重置")
	}
	if p.Password != password {
		return "", fmt.Errorf("访问密码错误")
	}
	ticket := jwt.RandomHex(16)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := database.RDB.Set(ctx, s.ticketKey(ticket), p.ID, livePageTicketTTL).Err(); err != nil {
		return "", fmt.Errorf("凭证签发失败: %w", err)
	}
	s.repo.IncrView(p.ID)
	return ticket, nil
}

// checkTicket 校验观众凭证。
func (s *LivePageService) checkTicket(p *models.LivePage, ticket string) error {
	if !p.HasPassword() {
		return nil // 公开页无需凭证
	}
	if ticket == "" {
		return fmt.Errorf("请先输入访问密码")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	pid, err := database.RDB.Get(ctx, s.ticketKey(ticket)).Uint64()
	if err != nil || pid != p.ID {
		return fmt.Errorf("访问凭证已失效，请重新输入密码")
	}
	return nil
}

// PlayData 观众播放数据。
type PlayData struct {
	Name    string            `json:"name"`
	Online  bool              `json:"online"`
	App     string            `json:"app"`
	Stream  string            `json:"stream"`
	URLs    map[string]string `json:"urls"` // 各格式播放地址（同源相对路径已含令牌）
}

// GetPlayData 校验凭证后下发播放地址（自动签发绑定该流的播放令牌）。
func (s *LivePageService) GetPlayData(code, ticket string) (*PlayData, error) {
	p, err := s.repo.GetByCode(code)
	if err != nil {
		return nil, err
	}
	if p.Status != 1 {
		return nil, fmt.Errorf("该直播页已停用")
	}
	if err := s.checkTicket(p, ticket); err != nil {
		return nil, err
	}
	st, err := s.streamRepo.GetByID(p.StreamID)
	if err != nil {
		return nil, fmt.Errorf("关联的流不存在")
	}
	// 签发绑定该流的播放令牌（4 小时）
	tk, err := s.authSvc.CreateToken(st.ID, livePagePlayTokenSec, "", "")
	if err != nil {
		return nil, fmt.Errorf("播放令牌签发失败: %w", err)
	}
	// 生成播放地址（默认域名）
	urls, err := s.playSvc.GenerateURLs(context.Background(), st, 0, tk.Token, tk.ExpireAt)
	if err != nil {
		return nil, err
	}
	return &PlayData{
		Name:   p.Name,
		Online: st.Online == 1,
		App:    st.App,
		Stream: st.Stream,
		URLs:   urls.URLs,
	}, nil
}

// genPageCode 生成直播页访问码（8 位随机，短小好分享）。
func genPageCode() string {
	return strings.ToLower(jwt.RandomHex(4))
}
