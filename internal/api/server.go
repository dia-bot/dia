// Package api is Dia's dashboard backend: a gin HTTP API with Discord OAuth2
// login (Redis-backed HttpOnly sessions + CSRF), per-guild feature config CRUD,
// welcome/rank image previews, and a realtime WebSocket that streams guild
// state changes to the dashboard.
package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/dia-bot/dia/internal/billing"
	"github.com/dia-bot/dia/internal/cache"
	"github.com/dia-bot/dia/internal/config"
	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/eventbus"
	"github.com/dia-bot/dia/internal/guildstate"
	"github.com/dia-bot/dia/internal/imaging"
	"github.com/dia-bot/dia/internal/realtime"
	"github.com/dia-bot/dia/internal/storage"
	"github.com/dia-bot/dia/internal/store"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"golang.org/x/oauth2"
)

const sessionTTL = 7 * 24 * time.Hour

// Deps are the API's injected dependencies.
type Deps struct {
	Config  *config.Config
	Log     *slog.Logger
	Store   *store.Store
	Cache   *cache.Store
	Discord *discord.Client
	Imaging *imaging.Renderer
	Bus     eventbus.Bus
	Storage *storage.Store  // nil when uploads aren't configured
	Billing *billing.Client // nil when Stripe isn't configured
}

// Server is the dashboard API.
type Server struct {
	cfg      *config.Config
	log      *slog.Logger
	store    *store.Store
	cache    *cache.Store
	discord  *discord.Client
	imaging  *imaging.Renderer
	bus      eventbus.Bus
	storage  *storage.Store
	billing  *billing.Client
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
		cache:    d.Cache,
		discord:  d.Discord,
		imaging:  d.Imaging,
		bus:      d.Bus,
		storage:  d.Storage,
		billing:  d.Billing,
		gstate:   guildstate.New(d.Cache),
		hub:      realtime.NewHub(d.Log),
		sessions: newSessionStore(d.Cache, sessionTTL),
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

	// OAuth (no session required). The browser callback lands on the web origin
	// (WebBaseURL + OAuthRedirectPath); the web server completes the flow by
	// calling /auth/exchange server-to-server.
	r.GET("/auth/login", s.handleLogin)
	r.POST("/auth/exchange", s.handleExchange)

	// Realtime WebSocket (auth checked inside the handler off the cookie).
	r.GET("/realtime/:id", s.handleRealtime)

	// Stripe webhook (no session/CSRF — verified by Stripe-Signature instead).
	r.POST("/billing/webhook", s.handleStripeWebhook)

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
	g.POST("/uploads", s.handleUpload)
	g.GET("/fonts", s.handleListFonts)
	g.POST("/fonts", s.handleUploadFont)
	g.DELETE("/fonts/:family", s.handleDeleteFont)
	g.GET("/assets", s.handleListAssets)
	g.DELETE("/assets/:aid", s.handleDeleteAsset)
	g.GET("/emojis", s.handleListEmojis)
	g.GET("/billing", s.handleBillingStatus)
	g.POST("/billing/checkout", s.handleCheckout)
	g.POST("/billing/portal", s.handlePortal)
	g.POST("/welcome/preview", s.handleWelcomePreview)
	g.POST("/welcome/test", s.handleWelcomeTest)
	g.POST("/welcome/actions", s.handleWelcomeActions)
	g.GET("/welcome/variables", s.handleWelcomeVariables)
	g.POST("/rank/preview", s.handleRankPreview)
	g.POST("/layout/preview", s.handleLayoutPreview)
	g.POST("/layout/resolve", s.handleResolveCard)
	g.POST("/templating/preview", s.handleTemplatingPreview)
	g.GET("/leveling/variables", s.handleLevelingVariables)
	g.POST("/leveling/actions", s.handleLevelingActions)
	g.POST("/autorole/actions", s.handleAutoroleActions)

	g.GET("/leaderboard", s.handleLeaderboard)
	g.GET("/level-rewards", s.handleListRewards)
	g.PUT("/level-rewards", s.handleSetReward)
	g.DELETE("/level-rewards/:level", s.handleDeleteReward)

	g.GET("/commands", s.handleListCommands)
	g.GET("/commands/:cid", s.handleGetCommand)
	g.PUT("/commands", s.handleUpsertCommand)
	g.POST("/commands/validate", s.handleValidateCommand)
	g.DELETE("/commands/:cid", s.handleDeleteCommand)
	g.PATCH("/commands/:cid/group", s.handleSetCommandGroup)
	g.GET("/command-groups", s.handleListCommandGroups)
	g.POST("/command-groups", s.handleCreateCommandGroup)
	g.PATCH("/command-group-order", s.handleReorderCommandGroups)
	g.PATCH("/command-groups/:gid", s.handleRenameCommandGroup)
	g.DELETE("/command-groups/:gid", s.handleDeleteCommandGroup)
	g.GET("/command-runs", s.handleListCommandRuns)
	g.GET("/command-runs/:rid", s.handleGetCommandRun)
	g.GET("/command-templates", s.handleListImageTemplates)
	g.PUT("/command-templates", s.handleUpsertImageTemplate)
	g.DELETE("/command-templates/:tid", s.handleDeleteImageTemplate)

	g.GET("/automations", s.handleListAutomations)
	g.GET("/automations/:aid", s.handleGetAutomation)
	g.PUT("/automations", s.handleUpsertAutomation)
	g.POST("/automations/validate", s.handleValidateAutomation)
	g.DELETE("/automations/:aid", s.handleDeleteAutomation)
	g.GET("/automation-triggers", s.handleListTriggers)
	g.GET("/automation-runs", s.handleListAutomationRuns)
	g.GET("/automation-runs/:rid", s.handleGetAutomationRun)

	g.GET("/reaction-roles", s.handleListMenus)
	g.PUT("/reaction-roles", s.handleUpsertMenu)
	g.DELETE("/reaction-roles/:mid", s.handleDeleteMenu)
	g.POST("/reaction-roles/:mid/post", s.handlePostMenu)
	g.POST("/reaction-roles/:mid/actions", s.handleMenuActions)

	g.GET("/cases", s.handleListCases)
	g.GET("/infractions", s.handleListInfractions)
	g.GET("/automod-stats", s.handleAutomodStats)

	g.GET("/automod-rules", s.handleListAutoModRules)
	g.PUT("/automod-rules", s.handleUpsertAutoModRule)
	g.DELETE("/automod-rules/:ruleId", s.handleDeleteAutoModRule)

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
	origins := s.cfg.API.CORSAllowOrigins
	if len(origins) == 0 {
		origins = []string{s.cfg.API.WebBaseURL}
	}
	return cors.New(cors.Config{
		AllowOrigins:     origins,
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
