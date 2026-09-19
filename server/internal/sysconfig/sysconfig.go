package sysconfig

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/EziosWJ/edge-platform/server/internal/audit"
	"github.com/EziosWJ/edge-platform/server/internal/auth"
	platform "github.com/EziosWJ/edge-platform/server/internal/platform/http"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	enabled            = 1
	builtin            = 1
	LogClearEnabledKey = "system.log-clear-enabled"
)

var (
	ErrNotFound = errors.New("数据不存在")
	ErrBuiltin  = errors.New("内置配置项禁止修改")
	ErrKey      = errors.New("配置键已存在")
	ErrInvalid  = errors.New("参数错误")
)

type Config struct {
	ID          int64     `gorm:"primaryKey" json:"id"`
	ConfigName  string    `json:"configName"`
	ConfigKey   string    `json:"configKey"`
	ConfigValue string    `json:"configValue"`
	ConfigType  string    `json:"configType"`
	ValueType   string    `json:"valueType"`
	Status      int       `json:"status"`
	IsBuiltin   int       `json:"isBuiltin"`
	Remark      *string   `json:"remark"`
	CreateTime  time.Time `gorm:"column:create_time;autoCreateTime" json:"createTime"`
	UpdateTime  time.Time `gorm:"column:update_time;autoUpdateTime" json:"updateTime"`
	Deleted     int       `json:"-"`
}

func (Config) TableName() string { return "sys_config" }

type ByKey struct {
	ConfigKey   string `json:"configKey"`
	ConfigValue string `json:"configValue"`
	ValueType   string `json:"valueType"`
	ConfigName  string `json:"configName"`
}
type Input struct {
	ConfigName, ConfigKey, ConfigValue, ConfigType, ValueType string
	Status                                                    *int
	Remark                                                    *string
}
type Query struct {
	Page, PageSize                    int
	ConfigName, ConfigKey, ConfigType string
	Status                            *int
}
type Page struct {
	Records  []Config `json:"records"`
	Total    int64    `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"pageSize"`
}
type Store interface {
	Page(context.Context, Query) (Page, error)
	Find(context.Context, int64) (*Config, error)
	ByKey(context.Context, string) (*ByKey, error)
	KeyExists(context.Context, string, int64) (bool, error)
	Create(context.Context, Config, audit.Event) (Config, error)
	Update(context.Context, Config, audit.Event) error
	Delete(context.Context, int64, audit.Event) error
	DeleteBatch(context.Context, []int64, audit.Event) error
	SetStatus(context.Context, int64, int, audit.Event) error
}
type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db} }
func (r *Repository) Page(ctx context.Context, q Query) (Page, error) {
	var p Page
	d := r.db.WithContext(ctx).Model(&Config{}).Where("deleted=0")
	if q.ConfigName != "" {
		d = d.Where("config_name LIKE ?", "%"+q.ConfigName+"%")
	}
	if q.ConfigKey != "" {
		d = d.Where("config_key LIKE ?", "%"+q.ConfigKey+"%")
	}
	if q.ConfigType != "" {
		d = d.Where("config_type=?", q.ConfigType)
	}
	if q.Status != nil {
		d = d.Where("status=?", *q.Status)
	}
	if e := d.Count(&p.Total).Error; e != nil {
		return p, e
	}
	p.Page, p.PageSize = q.Page, q.PageSize
	e := d.Order("id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&p.Records).Error
	return p, e
}
func (r *Repository) Find(c context.Context, id int64) (*Config, error) {
	var v Config
	e := r.db.WithContext(c).Where("id=? AND deleted=0", id).Take(&v).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &v, e
}
func (r *Repository) ByKey(c context.Context, k string) (*ByKey, error) {
	var v ByKey
	e := r.db.WithContext(c).Table("sys_config").Select("config_key,config_value,value_type,config_name").Where("config_key=? AND deleted=0 AND status=1", k).Take(&v).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &v, e
}
func (r *Repository) KeyExists(c context.Context, k string, x int64) (bool, error) {
	var n int64
	e := r.db.WithContext(c).Model(&Config{}).Where("config_key=? AND deleted=0 AND id<>?", k, x).Count(&n).Error
	return n > 0, e
}
func (r *Repository) Create(c context.Context, v Config, e audit.Event) (Config, error) {
	err := r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&v).Error; err != nil {
			return err
		}
		e.ResourceID = v.ID
		return audit.RecordOn(c, tx, e)
	})
	return v, err
}
func (r *Repository) Update(c context.Context, v Config, e audit.Event) error {
	return r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Config{}).Where("id=?", v.ID).Updates(v).Error; err != nil {
			return err
		}
		return audit.RecordOn(c, tx, e)
	})
}
func (r *Repository) Delete(c context.Context, id int64, e audit.Event) error {
	return r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Config{}).Where("id=?", id).Update("deleted", 1).Error; err != nil {
			return err
		}
		return audit.RecordOn(c, tx, e)
	})
}
func (r *Repository) DeleteBatch(c context.Context, ids []int64, e audit.Event) error {
	return r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Config{}).Where("id IN ?", ids).Update("deleted", 1).Error; err != nil {
			return err
		}
		return audit.RecordOn(c, tx, e)
	})
}
func (r *Repository) SetStatus(c context.Context, id int64, s int, e audit.Event) error {
	return r.db.WithContext(c).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Config{}).Where("id=?", id).Update("status", s).Error; err != nil {
			return err
		}
		return audit.RecordOn(c, tx, e)
	})
}

type Service struct {
	store Store
}

func NewService(s Store) *Service {
	return &Service{s}
}
func (s *Service) Page(c context.Context, q Query) (Page, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 10
	}
	if q.PageSize > 500 {
		q.PageSize = 500
	}
	return s.store.Page(c, q)
}
func (s *Service) Detail(c context.Context, id int64) (*Config, error)  { return s.store.Find(c, id) }
func (s *Service) GetByKey(c context.Context, k string) (*ByKey, error) { return s.store.ByKey(c, k) }
func (s *Service) Create(c context.Context, m audit.Metadata, in Input) error {
	if e := valid(in); e != nil {
		return e
	}
	x, e := s.store.KeyExists(c, in.ConfigKey, 0)
	if e != nil {
		return e
	}
	if x {
		return ErrKey
	}
	_, e = s.store.Create(c, makeConfig(in, 0), event(m, "config.create", "config", 0, "创建配置"))
	return e
}
func (s *Service) Update(c context.Context, m audit.Metadata, id int64, in Input) error {
	old, e := s.store.Find(c, id)
	if e != nil {
		return e
	}
	if old.IsBuiltin == builtin {
		return s.updateBuiltin(c, m, *old, in)
	}
	if e = valid(in); e != nil {
		return e
	}
	x, e := s.store.KeyExists(c, in.ConfigKey, id)
	if e != nil {
		return e
	}
	if x {
		return ErrKey
	}
	return s.store.Update(c, makeConfig(in, id), event(m, "config.update", "config", id, "更新配置"))
}

func (s *Service) updateBuiltin(c context.Context, m audit.Metadata, old Config, in Input) error {
	if old.ConfigKey != LogClearEnabledKey {
		return ErrBuiltin
	}
	if old.ValueType != "BOOLEAN" {
		return ErrInvalid
	}
	value, ok := normalizeBooleanValue(in.ConfigValue)
	if !ok {
		return ErrInvalid
	}
	old.ConfigValue = value
	return s.store.Update(c, old, event(m, "config.update", "config", old.ID, "更新配置"))
}

func normalizeBooleanValue(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true":
		return "true", true
	case "false":
		return "false", true
	default:
		return "", false
	}
}
func (s *Service) Delete(c context.Context, m audit.Metadata, id int64) error {
	v, e := s.store.Find(c, id)
	if e != nil {
		return e
	}
	if v.IsBuiltin == builtin {
		return ErrBuiltin
	}
	return s.store.Delete(c, id, event(m, "config.delete", "config", id, "删除配置"))
}
func (s *Service) DeleteBatch(c context.Context, m audit.Metadata, ids []int64) error {
	clean := unique(ids)
	for _, id := range clean {
		v, e := s.store.Find(c, id)
		if e != nil {
			return e
		}
		if v.IsBuiltin == builtin {
			return ErrBuiltin
		}
	}
	return s.store.DeleteBatch(c, clean, event(m, "config.batch-delete", "config", 0, "批量删除配置"))
}
func (s *Service) Status(c context.Context, m audit.Metadata, id, st int64) error {
	if st < 0 || st > 1 {
		return ErrInvalid
	}
	if _, e := s.store.Find(c, id); e != nil {
		return e
	}
	return s.store.SetStatus(c, id, int(st), event(m, "config.status", "config", id, "更新配置状态"))
}
func valid(v Input) error {
	if strings.TrimSpace(v.ConfigName) == "" || len(v.ConfigName) > 100 || strings.TrimSpace(v.ConfigKey) == "" || len(v.ConfigKey) > 100 || len(v.ConfigValue) > 500 || (v.Status != nil && (*v.Status < 0 || *v.Status > 1)) {
		return ErrInvalid
	}
	return nil
}
func event(m audit.Metadata, a, r string, id int64, sum string) audit.Event {
	return audit.Event{Action: a, Resource: r, ResourceID: id, Summary: sum, Metadata: m}
}
func unique(ids []int64) []int64 {
	seen := map[int64]bool{}
	out := []int64{}
	for _, id := range ids {
		if id > 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
func makeConfig(v Input, id int64) Config {
	if v.ConfigType == "" {
		v.ConfigType = "SYSTEM"
	}
	if v.ValueType == "" {
		v.ValueType = "TEXT"
	}
	status := enabled
	if v.Status != nil {
		status = *v.Status
	}
	return Config{ID: id, ConfigName: v.ConfigName, ConfigKey: v.ConfigKey, ConfigValue: v.ConfigValue, ConfigType: v.ConfigType, ValueType: v.ValueType, Status: status, Remark: v.Remark}
}

type Handler struct{ svc *Service }

// ApiEnvelope is the Swagger representation of the legacy response envelope.
type ApiEnvelope struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func NewHandler(s *Service) *Handler { return &Handler{s} }
func (h *Handler) Register(r gin.IRouter) {
	g := r.Group("/config")
	g.GET("/page", h.page)
	g.GET("/key/:key", h.key)
	g.GET("/:id", h.detail)
	g.POST("", h.create)
	g.PUT("/:id", h.update)
	g.DELETE("/:id", h.delete)
	g.POST("/batch-delete", h.batch)
	g.PATCH("/:id/status", h.status)
}

// @Summary 配置分页
// @Tags 配置管理
// @Security BearerAuth
// @Param page query int false "页码"
// @Param pageSize query int false "每页条数"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/config/page [get]
func (h *Handler) page(c *gin.Context) {
	q := Query{Page: queryInt(c, "page", 1), PageSize: queryInt(c, "pageSize", 10), ConfigName: c.Query("configName"), ConfigKey: c.Query("configKey"), ConfigType: c.Query("configType")}
	if x, ok := queryStatus(c, "status"); ok {
		q.Status = x
	} else {
		badConfig(c)
		return
	}
	if q.Page < 1 || q.PageSize < 1 || q.PageSize > 500 {
		badConfig(c)
		return
	}
	p, e := h.svc.Page(c.Request.Context(), q)
	if e != nil {
		if platform.IsTemporaryUnavailable(e) {
			platform.TemporaryUnavailable(c)
			return
		}
		platform.WriteError(c, 500, 500, "系统错误", nil)
		return
	}
	platform.OK(c, p)
}

// @Summary 配置详情
// @Tags 配置管理
// @Security BearerAuth
// @Param id path int true "配置 ID"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/config/{id} [get]
func (h *Handler) detail(c *gin.Context) {
	id, ok := configID(c)
	if !ok {
		return
	}
	v, e := h.svc.Detail(c, id)
	configError(c, e, v)
}

// @Summary 按配置键查询
// @Tags 配置管理
// @Security BearerAuth
// @Param key path string true "配置键"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/config/key/{key} [get]
func (h *Handler) key(c *gin.Context) {
	v, e := h.svc.GetByKey(c.Request.Context(), c.Param("key"))
	if e != nil {
		configError(c, e, nil)
		return
	}
	platform.OK(c, v)
}

type configRequest struct {
	ConfigName  string  `json:"configName"`
	ConfigKey   string  `json:"configKey"`
	ConfigValue string  `json:"configValue"`
	ConfigType  string  `json:"configType"`
	ValueType   string  `json:"valueType"`
	Status      *int    `json:"status"`
	Remark      *string `json:"remark"`
}
type idsRequest struct {
	IDs []int64 `json:"ids"`
}
type statusRequest struct {
	Status int64 `json:"status"`
}

// @Summary 新增配置
// @Tags 配置管理
// @Security BearerAuth
// @Param request body configRequest true "配置"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/config [post]
func (h *Handler) create(c *gin.Context) {
	var x configRequest
	if c.ShouldBindJSON(&x) != nil {
		badConfig(c)
		return
	}
	configError(c, h.svc.Create(c, configMeta(c), toInput(x)), nil)
}

// @Summary 修改配置
// @Tags 配置管理
// @Security BearerAuth
// @Param id path int true "配置 ID"
// @Param request body configRequest true "配置"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/config/{id} [put]
func (h *Handler) update(c *gin.Context) {
	id, ok := configID(c)
	if !ok {
		return
	}
	var x configRequest
	if c.ShouldBindJSON(&x) != nil {
		badConfig(c)
		return
	}
	configError(c, h.svc.Update(c, configMeta(c), id, toInput(x)), nil)
}

// @Summary 删除配置
// @Tags 配置管理
// @Security BearerAuth
// @Param id path int true "配置 ID"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/config/{id} [delete]
func (h *Handler) delete(c *gin.Context) {
	id, ok := configID(c)
	if ok {
		configError(c, h.svc.Delete(c, configMeta(c), id), nil)
	}
}

// @Summary 批量删除配置
// @Tags 配置管理
// @Security BearerAuth
// @Param request body idsRequest true "配置 ID 列表"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/config/batch-delete [post]
func (h *Handler) batch(c *gin.Context) {
	var x idsRequest
	if c.ShouldBindJSON(&x) != nil || len(x.IDs) == 0 {
		badConfig(c)
		return
	}
	configError(c, h.svc.DeleteBatch(c, configMeta(c), x.IDs), nil)
}

// @Summary 修改配置状态
// @Tags 配置管理
// @Security BearerAuth
// @Param id path int true "配置 ID"
// @Param request body statusRequest true "状态"
// @Success 200 {object} ApiEnvelope
// @Router /api/system/config/{id}/status [patch]
func (h *Handler) status(c *gin.Context) {
	id, ok := configID(c)
	if !ok {
		return
	}
	var x statusRequest
	if c.ShouldBindJSON(&x) != nil || x.Status < 0 || x.Status > 1 {
		badConfig(c)
		return
	}
	configError(c, h.svc.Status(c, configMeta(c), id, x.Status), nil)
}
func toInput(x configRequest) Input {
	return Input(x)
}
func configID(c *gin.Context) (int64, bool) {
	id, e := strconv.ParseInt(c.Param("id"), 10, 64)
	if e != nil || id < 1 {
		badConfig(c)
		return 0, false
	}
	return id, true
}
func configMeta(c *gin.Context) audit.Metadata {
	p, _ := auth.PrincipalFromContext(c.Request.Context())
	m, _ := platform.RequestMetaFromContext(c.Request.Context())
	return audit.Metadata{ActorID: p.UserID, RequestID: m.RequestID, ClientIP: m.ClientIP, UserAgent: m.UserAgent, RequestMethod: c.Request.Method, RequestURL: c.Request.URL.RequestURI()}
}
func badConfig(c *gin.Context) {
	platform.WriteError(c, http.StatusBadRequest, 400, "参数错误", nil)
}
func configError(c *gin.Context, e error, v any) {
	if e == nil {
		platform.OK(c, v)
		return
	}
	if platform.IsTemporaryUnavailable(e) {
		platform.TemporaryUnavailable(c)
	} else if errors.Is(e, ErrNotFound) {
		platform.WriteError(c, 200, 404, e.Error(), nil)
	} else if errors.Is(e, ErrBuiltin) || errors.Is(e, ErrKey) || errors.Is(e, ErrInvalid) {
		platform.WriteError(c, 200, 400, e.Error(), nil)
	} else {
		platform.WriteError(c, 500, 500, "系统错误", nil)
	}
}
func queryInt(c *gin.Context, k string, d int) int {
	v := c.Query(k)
	if v == "" {
		return d
	}
	n, e := strconv.Atoi(v)
	if e != nil {
		return 0
	}
	return n
}
func queryStatus(c *gin.Context, k string) (*int, bool) {
	v := c.Query(k)
	if v == "" {
		return nil, true
	}
	n, e := strconv.Atoi(v)
	if e != nil || n < 0 || n > 1 {
		return nil, false
	}
	return &n, true
}
