package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/karl/conclave/internal/auth"
	"github.com/karl/conclave/internal/config"
	"github.com/karl/conclave/internal/db"
	"github.com/karl/conclave/internal/handlers"
	apimiddleware "github.com/karl/conclave/internal/middleware"
	"github.com/karl/conclave/internal/ws"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	migrationsPath, _ := filepath.Abs("migrations")
	if err := db.Migrate(cfg.DatabaseURL, migrationsPath); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	log.Println("migrations applied")

	authSvc := auth.New(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.BaseURL+"/api/auth/callback", cfg.JWTSecret)
	hub := ws.NewHub()
	go hub.Run()

	handlers.StartRetentionWorker(ctx, pool)

	authH := handlers.NewAuth(authSvc, pool, cfg.BaseURL, cfg.FrontendURL)
	usersH := handlers.NewUsers(pool, cfg.AvatarDir, cfg.BaseURL, cfg.InstanceAdminEmail)
	adminH := handlers.NewAdmin(pool, cfg.InstanceAdminEmail)
	serversH := handlers.NewServers(pool, hub, cfg.InstanceAdminEmail)
	channelsH := handlers.NewChannels(pool, hub)
	threadsH := handlers.NewThreads(pool, hub)
	pushH := handlers.NewPush(pool, cfg.VAPIDPublicKey, cfg.VAPIDPrivateKey, cfg.VAPIDEmail)
	friendsH := handlers.NewFriends(pool, hub)
	messagesH := handlers.NewMessages(pool, hub, pushH, cfg.AvatarDir, cfg.BaseURL)
	dmsH := handlers.NewDMs(pool, hub, pushH, cfg.AvatarDir, cfg.BaseURL)
	wsH := handlers.NewWS(hub, authSvc, pool, cfg.BaseURL, cfg.FrontendURL)
	go wsH.RunPresenceBroadcaster()

	r := chi.NewRouter()
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{cfg.BaseURL, cfg.FrontendURL},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	// public (no auth)
	r.Get("/api/theme", adminH.GetTheme)
	r.Get("/api/config", adminH.GetConfig)
	r.Get("/api/auth/login", authH.Login)
	r.Get("/api/auth/callback", authH.Callback)
	r.Post("/api/auth/logout", authH.Logout)

	// websocket
	r.Get("/ws", wsH.Handle)

	// serve avatars
	r.Handle("/avatars/*", http.StripPrefix("/avatars/", http.FileServer(http.Dir(cfg.AvatarDir))))

	r.Group(func(r chi.Router) {
		r.Use(apimiddleware.Auth(authSvc, pool))

		// users
		r.Get("/api/users/me", usersH.Me)
		r.Patch("/api/users/me", usersH.UpdateMe)
		r.Post("/api/users/me/avatar", usersH.UploadAvatar)
		r.Get("/api/users/{userID}", usersH.GetUser)

		// servers
		r.Get("/api/servers", serversH.List)
		r.Get("/api/servers/discover", serversH.Discover)
		r.Get("/api/servers/{serverID}/presence", serversH.Presence)
		r.Post("/api/servers", serversH.Create)
		r.Get("/api/servers/{serverID}", serversH.Get)
		r.Patch("/api/servers/{serverID}", serversH.Update)
		r.Delete("/api/servers/{serverID}", serversH.Delete)
		r.Post("/api/servers/{serverID}/icon", func(w http.ResponseWriter, r *http.Request) {
			serversH.UploadIcon(w, r, cfg.AvatarDir, cfg.BaseURL)
		})
		r.Post("/api/servers/{serverID}/join", serversH.Join)
		r.Post("/api/servers/{serverID}/join-request", serversH.RequestJoin)
		r.Get("/api/servers/{serverID}/join-requests", serversH.ListJoinRequests)
		r.Patch("/api/servers/{serverID}/join-requests/{requestID}", serversH.ReviewJoinRequest)
		r.Delete("/api/servers/{serverID}/leave", serversH.Leave)
		r.Get("/api/servers/{serverID}/members", serversH.Members)
		r.Patch("/api/servers/{serverID}/members/{userID}", serversH.UpdateMember)
		r.Delete("/api/servers/{serverID}/members/{userID}", serversH.KickMember)
		r.Post("/api/servers/{serverID}/members/{userID}/ban", serversH.BanMember)
		r.Delete("/api/servers/{serverID}/bans/{userID}", serversH.UnbanMember)
		r.Get("/api/servers/{serverID}/bans", serversH.ListBans)
		r.Post("/api/servers/{serverID}/invites", serversH.CreateInvite)
		r.Post("/api/invites/{code}/join", serversH.JoinByInvite)

		// channels
		r.Get("/api/servers/{serverID}/channels", channelsH.List)
		r.Post("/api/servers/{serverID}/channels", channelsH.Create)
		r.Patch("/api/servers/{serverID}/channels/{channelID}", channelsH.Update)
		r.Delete("/api/servers/{serverID}/channels/{channelID}", channelsH.Delete)
		r.Post("/api/servers/{serverID}/channels/{channelID}/read", channelsH.MarkRead)
		r.Get("/api/servers/{serverID}/voice", channelsH.VoiceState)

		// threads
		r.Get("/api/servers/{serverID}/channels/{channelID}/threads", threadsH.List)
		r.Post("/api/servers/{serverID}/channels/{channelID}/threads", threadsH.Create)
		r.Get("/api/threads/{threadID}/messages", threadsH.ListMessages)
		r.Post("/api/threads/{threadID}/messages", threadsH.SendMessage)

		// messages
		r.Get("/api/servers/{serverID}/channels/{channelID}/messages", messagesH.List)
		r.Post("/api/servers/{serverID}/channels/{channelID}/messages", messagesH.Send)
		r.Patch("/api/servers/{serverID}/channels/{channelID}/messages/{messageID}", messagesH.Edit)
		r.Delete("/api/servers/{serverID}/channels/{channelID}/messages/{messageID}", messagesH.Delete)

		// DMs
		r.Get("/api/dms", dmsH.ListConversations)
		r.Post("/api/dms/{userID}", dmsH.GetOrCreate)
		r.Get("/api/dms/conversations/{convID}/messages", dmsH.ListMessages)
		r.Post("/api/dms/conversations/{convID}/messages", dmsH.SendMessage)
		r.Delete("/api/dms/conversations/{convID}/messages/{messageID}", dmsH.DeleteMessage)

		// file upload
		r.Post("/api/upload", handlers.UploadFile(cfg.AvatarDir, cfg.BaseURL))

		// friends
		r.Get("/api/friends", friendsH.List)
		r.Get("/api/friends/requests", friendsH.Requests)
		r.Get("/api/friends/sent", friendsH.Sent)
		r.Post("/api/friends/request/{userID}", friendsH.SendRequest)
		r.Post("/api/friends/accept/{userID}", friendsH.Accept)
		r.Delete("/api/friends/{userID}", friendsH.Remove)
		r.Get("/api/users/search", friendsH.SearchUsers)

		// push notifications
		r.Get("/api/push/key", pushH.GetPublicKey)
		r.Post("/api/push/subscribe", pushH.Subscribe)
		r.Delete("/api/push/subscribe", pushH.Unsubscribe)

		// instance admin
		r.Get("/api/admin/settings", adminH.GetSettings)
		r.Patch("/api/admin/settings", adminH.UpdateSettings)
		r.Post("/api/admin/retention/run", adminH.RunRetention)
		r.Get("/api/admin/users", adminH.ListInstanceUsers)
		r.Post("/api/admin/users/{userID}/ban", adminH.BanInstanceUser)
		r.Delete("/api/admin/users/{userID}/ban", adminH.UnbanInstanceUser)
	})

	// serve frontend (SvelteKit static build) with SPA fallback
	if _, err := os.Stat(cfg.StaticDir); err == nil {
		r.Handle("/*", spaHandler(cfg.StaticDir))
	}

	log.Printf("listening on :%s", cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, r))
}

func spaHandler(staticDir string) http.HandlerFunc {
	fs := http.FileServer(http.Dir(staticDir))
	return func(w http.ResponseWriter, r *http.Request) {
		path := filepath.Join(staticDir, filepath.Clean("/"+r.URL.Path))
		if _, err := os.Stat(path); os.IsNotExist(err) {
			http.ServeFile(w, r, filepath.Join(staticDir, "index.html"))
			return
		}
		fs.ServeHTTP(w, r)
	}
}
