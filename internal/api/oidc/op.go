package oidc

import (
	"context"
	"time"

	"github.com/caos/logging"
	"github.com/caos/oidc/pkg/op"

	http_utils "github.com/caos/zitadel/internal/api/http"
	"github.com/caos/zitadel/internal/api/http/middleware"
	"github.com/caos/zitadel/internal/auth/repository"
	"github.com/caos/zitadel/internal/config/types"
	"github.com/caos/zitadel/internal/id"
)

type OPHandlerConfig struct {
	OPConfig              *op.Config
	StorageConfig         StorageConfig
	UserAgentCookieConfig *middleware.UserAgentCookieConfig
	Cache                 *middleware.CacheConfig
	Endpoints             *EndpointConfig
}

type StorageConfig struct {
	DefaultLoginURL            string
	SigningKeyAlgorithm        string
	DefaultAccessTokenLifetime types.Duration
	DefaultIdTokenLifetime     types.Duration
}

type EndpointConfig struct {
	Auth       *Endpoint
	Token      *Endpoint
	Userinfo   *Endpoint
	EndSession *Endpoint
	Keys       *Endpoint
}

type Endpoint struct {
	Path string
	URL  string
}

type OPStorage struct {
	repo                       repository.Repository
	defaultLoginURL            string
	defaultAccessTokenLifetime time.Duration
	defaultIdTokenLifetime     time.Duration
	signingKeyAlgorithm        string
}

func NewProvider(ctx context.Context, config OPHandlerConfig, repo repository.Repository, localDevMode bool) op.OpenIDProvider {
	cookieHandler, err := middleware.NewUserAgentHandler(config.UserAgentCookieConfig, id.SonyFlakeGenerator, localDevMode)
	logging.Log("OIDC-sd4fd").OnError(err).Panic("cannot user agent handler")
	config.OPConfig.CodeMethodS256 = true
	provider, err := op.NewOpenIDProvider(
		ctx,
		config.OPConfig,
		newStorage(config.StorageConfig, repo),
		op.WithHttpInterceptors(
			middleware.NoCacheInterceptor,
			cookieHandler,
			http_utils.CopyHeadersToContext,
		),
		op.WithCustomAuthEndpoint(op.NewEndpointWithURL(config.Endpoints.Auth.Path, config.Endpoints.Auth.URL)),
		op.WithCustomTokenEndpoint(op.NewEndpointWithURL(config.Endpoints.Token.Path, config.Endpoints.Token.URL)),
		op.WithCustomUserinfoEndpoint(op.NewEndpointWithURL(config.Endpoints.Userinfo.Path, config.Endpoints.Userinfo.URL)),
		op.WithCustomEndSessionEndpoint(op.NewEndpointWithURL(config.Endpoints.EndSession.Path, config.Endpoints.EndSession.URL)),
		op.WithCustomKeysEndpoint(op.NewEndpointWithURL(config.Endpoints.Keys.Path, config.Endpoints.Keys.URL)),
		op.WithRetry(3, time.Duration(30*time.Second)),
	)
	logging.Log("OIDC-asf13").OnError(err).Panic("cannot create provider")
	return provider
}

func newStorage(config StorageConfig, repo repository.Repository) *OPStorage {
	return &OPStorage{
		repo:                       repo,
		defaultLoginURL:            config.DefaultLoginURL,
		signingKeyAlgorithm:        config.SigningKeyAlgorithm,
		defaultAccessTokenLifetime: config.DefaultAccessTokenLifetime.Duration,
		defaultIdTokenLifetime:     config.DefaultIdTokenLifetime.Duration,
	}
}

func (o *OPStorage) Health(ctx context.Context) error {
	return o.repo.Health(ctx)
}
