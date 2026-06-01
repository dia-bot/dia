// Package api is Dia's dashboard backend: a gin HTTP API with Discord OAuth2
// login (Redis-backed HttpOnly sessions + CSRF), per-guild feature config CRUD,
// welcome/rank image previews, and a realtime WebSocket that streams guild
// state changes to the dashboard.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/dia-bot/dia/internal/config"
	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/eventbus"
	"github.com/dia-bot/dia/internal/guildstate"
	"github.com/dia-bot/dia/internal/imaging"
	"github.com/dia-bot/dia/internal/realtime"
	"github.com/dia-bot/dia/internal/store"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"golang.org/x/oauth2"
)

const sessionTTL = 7 * 24 * time.Hour

// Deps are the API's injected dependencies.
type Deps struct {
	Config  *config.Config
	Log     *slog.Logger
	Store   *store.Store
	Redis   *redis.Client
	Discord *discord.Client
	Imaging *imaging.Renderer
	Bus     eventbus.Bus
}

// Server is the dashboard API.
type Server struct {
	cfg      *config.Config
	log      *slog.Logger
	store    *store.Store
	rdb      *redis.Client
	discord  *discord.Client
	imaging  *imaging.Renderer
	bus      eventbus.Bus
	gstate   *guildstate.Store
	hub      *realtime.Hub
	sessions *sessionStore
	oauth    *oauth2.Config
	upgrader websocket.Upgrader
}

// New builds a Server.
func New(d Deps) *Server {
	webOrigin := d.Config.API.WebBaseURL
	return &Server{
		cfg:      d.Config,
		log:      d.Log,
		store:    d.Store,
		rdb:      d.Redis,
		discord:  d.Discord,
		imaging:  d.Imaging,
		bus:      d.Bus,
		gstate:   guildstate.New(d.Redis),
		hub:      realtime.NewHub(d.Log),
		sessions: newSessionStore(d.Redis, sessionTTL),
		oauth: &oauth2.Config{
			ClientID:     d.Config.Discord.ClientID,
			ClientSecret: d.Config.Discord.ClientSecret,
			RedirectURL:  d.Config.API.OAuthRedirectURL(),
			Scopes:       []string{"identify", "guilds", "email"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://discord.com/oauth2/authorize",
				TokenURL: "https://discord.com/api/oauth2/token",
			},
		},
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return originAllowed(r, webOrigin) },
		},
	}
}

// Handler builds the gin engine with all routes and middleware.
func (s *Server) Handler() http.Handler {
	if s.cfg.IsProd() {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery(), s.logMiddleware(), s.corsMiddleware())

	r.GET("/healthz", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok", "ws_clients": s.hub.Count()}) })

	// OAuth (no session required).
	r.GET("/auth/login", s.handleLogin)
	r.GET(s.cfg.API.OAuthRedirectPath, s.handleCallback)

	// Realtime WebSocket (auth checked inside the handler off the cookie).
	r.GET("/realtime/:id", s.handleRealtime)

	api := r.Group("/api")
	api.GET("/me", s.handleMe) // self-handles the unauthenticated case
	api.POST("/auth/logout", s.handleLogout)

	authed := api.Group("")
	authed.Use(s.requireAuth(), s.csrf())
	authed.GET("/guilds", s.handleListGuilds)
	authed.GET("/welcome/presets", s.handleWelcomePresets)

	g := authed.Group("/guilds/:id")
	g.Use(s.requireGuild())
	g.GET("", s.handleGetGuild)
	g.GET("/features", s.handleListFeatures)
	g.GET("/features/:key", s.handleGetFeature)
	g.PUT("/features/:key", s.handlePutFeature)
	g.POST("/welcome/preview", s.handleWelcomePreview)
	g.POST("/rank/preview", s.handleRankPreview)

	g.GET("/leaderboard", s.handleLeaderboard)
	g.GET("/level-rewards", s.handleListRewards)
	g.PUT("/level-rewards", s.handleSetReward)
	g.DELETE("/level-rewards/:level", s.handleDeleteReward)

	g.GET("/commands", s.handleListCommands)
	g.PUT("/commands", s.handleUpsertCommand)
	g.DELETE("/commands/:cid", s.handleDeleteCommand)

	g.GET("/reaction-roles", s.handleListMenus)
	g.PUT("/reaction-roles", s.handleUpsertMenu)
	g.DELETE("/reaction-roles/:mid", s.handleDeleteMenu)

	g.GET("/cases", s.handleListCases)

	return r
}

// Hub exposes the realtime hub (so cmd/api can start its NATS feed).
func (s *Server) Hub() *realtime.Hub { return s.hub }

func (s *Server) logMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		s.log.Debug("http",
			"method", c.Request.Method, "path", c.Request.URL.Path,
			"status", c.Writer.Status(), "dur_ms", time.Since(start).Milliseconds())
	}
}

func (s *Server) corsMiddleware() gin.HandlerFunc {
	return cors.New(cors.Config{
		AllowOrigins:     []string{s.cfg.API.WebBaseURL},
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "X-CSRF-Token"},
		ExposeHeaders:    []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}

// fail writes a JSON error envelope.
func fail(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, gin.H{"error": msg})
}
