// Package app wires infrastructure dependencies into the HTTP application.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/EziosWJ/edge-platform/server/internal/auth"
	"github.com/EziosWJ/edge-platform/server/internal/config"
	"github.com/EziosWJ/edge-platform/server/internal/dept"
	"github.com/EziosWJ/edge-platform/server/internal/dictionary"
	"github.com/EziosWJ/edge-platform/server/internal/filemgmt"
	"github.com/EziosWJ/edge-platform/server/internal/logmgmt"
	"github.com/EziosWJ/edge-platform/server/internal/notification"
	platformhttp "github.com/EziosWJ/edge-platform/server/internal/platform/http"
	"github.com/EziosWJ/edge-platform/server/internal/rbac"
	"github.com/EziosWJ/edge-platform/server/internal/sysconfig"
	"github.com/EziosWJ/edge-platform/server/internal/usermgmt"
)

// Dependencies holds the named business services the HTTP application assembles.
// Core management services are required; notification routes are enabled when
// the optional Notification service is supplied.
type Dependencies struct {
	Auth         *auth.Service
	RBAC         *rbac.Service
	Department   *dept.Service
	User         *usermgmt.Service
	Dictionary   *dictionary.Service
	SysConfig    *sysconfig.Service
	File         *filemgmt.Service
	Log          *logmgmt.Service
	Notification *notification.Service
	MQTT         platformhttp.MQTTRuntime
}

// Application is the assembled HTTP application and its process logger.
type Application struct {
	Router *gin.Engine
	Logger *slog.Logger
	mqtt   platformhttp.MQTTRuntime
}

// New assembles the HTTP router. Database readiness is supplied by the caller
// so that the application layer does not depend on a concrete database driver.
func New(cfg config.Config, readiness platformhttp.ReadinessChecker, deps Dependencies) (*Application, error) {
	if err := deps.validate(); err != nil {
		return nil, err
	}

	logger, err := newLogger(cfg)
	if err != nil {
		return nil, err
	}
	mqttRuntime, err := newMQTTRuntime(cfg.MQTT, deps.MQTT)
	if err != nil {
		return nil, err
	}

	router := gin.New()
	if err := router.SetTrustedProxies(cfg.HTTP.TrustedProxies); err != nil {
		return nil, fmt.Errorf("configure trusted proxies: %w", err)
	}
	router.Use(
		platformhttp.RequestMetadata(),
		platformhttp.RequestLogger(logger),
		platformhttp.Recovery(logger),
		platformhttp.CORS(platformhttp.CORSConfig{
			AllowedOrigins: cfg.CORS.AllowedOrigins,
			AllowedMethods: cfg.CORS.AllowedMethods,
			AllowedHeaders: cfg.CORS.AllowedHeaders,
			ExposedHeaders: cfg.CORS.ExposedHeaders,
			MaxAge:         cfg.CORS.MaxAge,
		}),
	)
	router.NoRoute(platformhttp.NotFoundHandler)

	platformhttp.RegisterSystemRoutes(router, platformhttp.SystemRoutes{
		Readiness: combinedReadiness{database: readiness, mqtt: mqttRuntime, mqttEnabled: cfg.MQTT.Enabled},
	})
	authHandler, err := auth.NewHandler(deps.Auth, deps.Auth)
	if err != nil {
		return nil, fmt.Errorf("create authentication handler: %w", err)
	}
	auth.RegisterRoutes(router, authHandler)

	system := router.Group("/api/system")
	system.Use(
		auth.BearerMiddleware(deps.Auth),
		platformhttp.MultipartProtection(platformhttp.MultipartProtectionConfig{
			Policies: map[string]platformhttp.MultipartPolicy{
				"/api/system/file/upload":       {MaxBodyBytes: filemgmt.MaxSingleBodySize},
				"/api/system/file/upload-batch": {MaxBodyBytes: filemgmt.MaxBatchBodySize},
			},
			MaxConcurrent: 8,
			Logger:        logger,
		}),
	)
	platformhttp.RegisterMQTTStatusRoute(system, mqttRuntime)

	rbacHandler, err := rbac.NewHandler(deps.RBAC)
	if err != nil {
		return nil, fmt.Errorf("create RBAC handler: %w", err)
	}
	rbac.RegisterRoutes(system, rbacHandler)

	deptHandler, err := dept.NewHandler(deps.Department)
	if err != nil {
		return nil, fmt.Errorf("create department handler: %w", err)
	}
	dept.RegisterRoutes(system, deptHandler)

	userHandler, err := usermgmt.NewHandler(deps.User)
	if err != nil {
		return nil, fmt.Errorf("create user handler: %w", err)
	}
	usermgmt.RegisterRoutes(system, userHandler)

	dictionaryHandler, err := dictionary.NewHandler(deps.Dictionary)
	if err != nil {
		return nil, fmt.Errorf("create dictionary handler: %w", err)
	}
	dictionary.RegisterRoutes(system, dictionaryHandler)

	sysconfig.NewHandler(deps.SysConfig).Register(system)

	fileHandler, err := filemgmt.NewHandler(deps.File)
	if err != nil {
		return nil, fmt.Errorf("create file handler: %w", err)
	}
	filemgmt.RegisterRoutes(system, fileHandler)

	logHandler, err := logmgmt.NewHandler(deps.Log)
	if err != nil {
		return nil, fmt.Errorf("create log handler: %w", err)
	}
	logmgmt.RegisterRoutes(system, logHandler)

	if deps.Notification != nil {
		notificationHandler, err := notification.NewHandler(deps.Notification)
		if err != nil {
			return nil, fmt.Errorf("create notification handler: %w", err)
		}
		notification.RegisterRoutes(system, notificationHandler)
	}

	if cfg.Environment == config.EnvironmentDev && cfg.Swagger.Enabled {
		registerSwaggerUI(router)
	}

	return &Application{Router: router, Logger: logger, mqtt: mqttRuntime}, nil
}

// StartRuntime starts process-local runtimes before the HTTP server accepts
// traffic. A runtime is expected to manage broker reconnects asynchronously;
// broker availability is therefore represented by readiness, not startup
// failure.
func (a *Application) StartRuntime(ctx context.Context) error {
	if a == nil || a.mqtt == nil {
		return nil
	}
	return a.mqtt.Start(ctx)
}

// StopRuntime stops MQTT before HTTP and database shutdown. The caller owns
// the timeout because MQTT and HTTP have separate shutdown budgets.
func (a *Application) StopRuntime(ctx context.Context) error {
	if a == nil || a.mqtt == nil {
		return nil
	}
	return a.mqtt.Stop(ctx)
}

type combinedReadiness struct {
	database    platformhttp.ReadinessChecker
	mqtt        platformhttp.MQTTRuntime
	mqttEnabled bool
}

func (r combinedReadiness) Ready(ctx context.Context) error {
	if r.database == nil {
		return errors.New("database readiness checker is required")
	}
	if err := r.database.Ready(ctx); err != nil {
		return err
	}
	if r.mqttEnabled && r.mqtt != nil {
		return r.mqtt.Ready(ctx)
	}
	return nil
}

func (d Dependencies) validate() error {
	switch {
	case d.Auth == nil:
		return fmt.Errorf("auth service is required")
	case d.RBAC == nil:
		return fmt.Errorf("rbac service is required")
	case d.Department == nil:
		return fmt.Errorf("department service is required")
	case d.User == nil:
		return fmt.Errorf("user service is required")
	case d.Dictionary == nil:
		return fmt.Errorf("dictionary service is required")
	case d.SysConfig == nil:
		return fmt.Errorf("sysconfig service is required")
	case d.File == nil:
		return fmt.Errorf("file service is required")
	case d.Log == nil:
		return fmt.Errorf("log service is required")
	}
	return nil
}

// Build constructs a router for callers that do not need the process logger.
func Build(cfg config.Config, readiness platformhttp.ReadinessChecker, deps Dependencies) (*gin.Engine, error) {
	application, err := New(cfg, readiness, deps)
	if err != nil {
		return nil, err
	}
	return application.Router, nil
}

func newLogger(cfg config.Config) (*slog.Logger, error) {
	level := new(slog.LevelVar)
	if err := level.UnmarshalText([]byte(cfg.Log.Level)); err != nil {
		return nil, fmt.Errorf("parse log level: %w", err)
	}

	options := &slog.HandlerOptions{
		AddSource: cfg.Log.AddSource,
		Level:     level,
	}
	if cfg.Log.Format == "text" {
		return slog.New(slog.NewTextHandler(os.Stdout, options)), nil
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, options)), nil
}
