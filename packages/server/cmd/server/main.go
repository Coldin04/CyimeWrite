package main

import (
	"context"
	"log"
	"os"
	"strings"

	"g.co1d.in/Coldin04/Cyime/server/internal/auth"
	"g.co1d.in/Coldin04/Cyime/server/internal/config"
	"g.co1d.in/Coldin04/Cyime/server/internal/content"
	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/media"
	"g.co1d.in/Coldin04/Cyime/server/internal/middleware"
	"g.co1d.in/Coldin04/Cyime/server/internal/securevalue"
	"g.co1d.in/Coldin04/Cyime/server/internal/user"
	"g.co1d.in/Coldin04/Cyime/server/internal/workspace"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	_ = config.LoadDotEnv(".env")

	// Validate critical secrets BEFORE touching the database. If JWT_SECRET_KEY
	// or APP_ENCRYPTION_KEY is missing, weak, or set to a known insecure
	// default, refuse to start so the operator notices instead of silently
	// shipping forgeable tokens or encrypting stored secrets with a public key.
	if _, err := auth.LoadJWTSecret(); err != nil {
		log.Fatalf("Auth configuration invalid: %v", err)
	}
	if err := securevalue.ValidateEncryptionKey(); err != nil {
		log.Fatalf("Encryption configuration invalid: %v", err)
	}

	// Initialize database
	database.Connect()
	log.Println("Database initialization complete.")
	media.StartAssetGCWorker(context.Background())

	// Create new Fiber app
	app := fiber.New()
	app.Use(recover.New())

	// Add flexible CORS middleware
	app.Use(cors.New(cors.Config{
		AllowOriginsFunc: func(origin string) bool {
			allowedOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
			if allowedOrigins == "" {
				// Default for local development
				return origin == "http://localhost:5173"
			}
			for _, allowed := range strings.Split(allowedOrigins, ",") {
				if origin == allowed {
					return true
				}
			}
			return false
		},
		AllowCredentials: true,
	}))

	// --- ROUTING ---
	api := app.Group("/api/v1")

	// Client config endpoint (public)
	api.Get("/config", config.GetClientConfigHandler)

	// Auth routes
	authRoutes := api.Group("/auth")
	authRoutes.Get("/config", auth.GetAuthConfig)
	authRoutes.Get("/login/:provider", auth.AuthLogin)
	authRoutes.Get("/callback/:provider", auth.AuthCallback)
	authRoutes.Post("/refresh", auth.HandleRefresh)
	authRoutes.Post("/logout", auth.HandleLogout)
	authRoutes.Get("/sessions", middleware.Protected(), auth.HandleListSessions)
	authRoutes.Delete("/sessions/others", middleware.Protected(), auth.HandleRevokeOtherSessions)
	authRoutes.Delete("/sessions/:id", middleware.Protected(), auth.HandleRevokeSession)

	// User routes (protected)
	userRoutes := api.Group("/user", middleware.Protected())
	userRoutes.Get("/me", user.GetMe)
	userRoutes.Get("/overview", user.GetOverview)
	userRoutes.Get("/image-beds/providers", user.ListImageBedProvidersHandler)
	userRoutes.Get("/image-beds", user.ListImageBedConfigsHandler)
	userRoutes.Post("/image-beds", user.CreateImageBedConfigHandler)
	userRoutes.Put("/image-beds/:id", user.UpdateImageBedConfigHandler)
	userRoutes.Delete("/image-beds/:id", user.DeleteImageBedConfigHandler)
	userRoutes.Put("/profile", user.UpdateProfileHandler)
	userRoutes.Post("/avatar", user.UploadAvatarHandler)
	userRoutes.Put("/avatar/github", user.UpdateGitHubAvatarHandler)
	api.Get("/user/avatar/content", user.GetAvatarContentHandler)

	// Workspace routes (protected)
	workspaceRoutes := api.Group("/workspace", middleware.Protected())
	workspaceRoutes.Get("/files", workspace.GetFilesHandler)
	workspaceRoutes.Get("/search", workspace.SearchHandler)
	workspaceRoutes.Get("/shared/summary", workspace.SharedDocumentSummaryHandler)
	workspaceRoutes.Get("/shared/documents", workspace.ListSharedDocumentsHandler)
	workspaceRoutes.Get("/shared/outgoing", workspace.ListOutgoingSharedDocumentsHandler)
	workspaceRoutes.Get("/files/:id", workspace.GetFileHandler)
	workspaceRoutes.Post("/folders", workspace.CreateFolderHandler)
	workspaceRoutes.Post("/documents", workspace.CreateDocumentHandler)
	workspaceRoutes.Post("/files/batch-delete", workspace.BatchDeleteHandler)
	workspaceRoutes.Delete("/files/:id", workspace.DeleteFileHandler)
	workspaceRoutes.Get("/folders/:id/ancestors", workspace.GetFolderAncestorsHandler)
	workspaceRoutes.Get("/trash", workspace.GetTrashHandler)
	workspaceRoutes.Post("/trash/restore", workspace.RestoreTrashHandler)
	workspaceRoutes.Delete("/trash", workspace.PermanentDeleteHandler)
	// Update document title
	workspaceRoutes.Put("/documents/:id/title", workspace.UpdateDocumentTitleHandler)
	workspaceRoutes.Put("/documents/:id/excerpt", workspace.UpdateDocumentExcerptHandler)
	workspaceRoutes.Put("/documents/:id/image-target", workspace.UpdateDocumentImageTargetHandler)
	workspaceRoutes.Put("/documents/:id/public-access", workspace.UpdateDocumentPublicAccessHandler)
	workspaceRoutes.Get("/documents/:id/shares", workspace.ListDocumentMembersHandler)
	workspaceRoutes.Post("/documents/:id/shares", workspace.ShareDocumentHandler)
	workspaceRoutes.Post("/documents/:id/invites", workspace.InviteDocumentByEmailHandler)
	workspaceRoutes.Delete("/documents/:id/shares/me", workspace.LeaveSharedDocumentHandler)
	workspaceRoutes.Delete("/documents/:id/shares/:userId", workspace.RemoveDocumentMemberHandler)
	workspaceRoutes.Post("/document-invites/:id/accept", workspace.AcceptDocumentInviteHandler)
	workspaceRoutes.Post("/document-invites/:id/decline", workspace.DeclineDocumentInviteHandler)
	workspaceRoutes.Get("/documents/:id/presence", workspace.GetDocumentPresenceHandler)
	workspaceRoutes.Put("/documents/:id/presence", workspace.HeartbeatDocumentPresenceHandler)
	// Update folder name
	workspaceRoutes.Put("/folders/:id/name", workspace.UpdateFolderNameHandler)
	// Move document
	workspaceRoutes.Put("/documents/:id/move", workspace.MoveDocumentHandler)
	// Move folder
	workspaceRoutes.Put("/folders/:id/move", workspace.MoveFolderHandler)
	// Batch move files and folders
	workspaceRoutes.Post("/files/batch-move", workspace.BatchMoveHandler)
	// ACL endpoint for realtime collaboration
	workspaceRoutes.Get("/documents/:id/acl", workspace.GetDocumentACLHandler)

	// Realtime routes for Yjs state management
	realtimeRoutes := api.Group("/realtime", middleware.Protected())
	realtimeRoutes.Get("/documents/:id/state", workspace.GetYjsStateHandler)
	realtimeRoutes.Put("/documents/:id/state", workspace.UpdateYjsStateHandler)

	// Edit routes (protected) - for document content management
	editRoutes := api.Group("/edit/documents", middleware.Protected())
	editRoutes.Get("/:id/content", content.GetContentHandler)
	editRoutes.Put("/:id/content", content.UpdateContentHandler)
	editRoutes.Post("/:id/assets", media.UploadDocumentAssetHandler)
	editRoutes.Post("/:id/paste-image", media.UploadDocumentImageHandler)

	// Media read routes:
	// - URL exchange is protected by JWT.
	// - Private content/thumbnail fallback endpoints require JWT and enforce ACL per request.
	api.Get("/media/assets", middleware.Protected(), media.ListAssetsHandler)
	api.Get("/media/shared-assets", middleware.Protected(), media.ListSharedAssetsHandler)
	api.Get("/media/assets/:id/url", middleware.Protected(), media.GetAssetURLHandler)
	api.Post("/media/assets/resolve", middleware.Protected(), media.ResolveAssetURLsHandler)
	api.Get("/media/assets/:id/references", middleware.Protected(), media.GetAssetReferencesHandler)
	api.Delete("/media/assets/:id", middleware.Protected(), media.DeleteAssetHandler)
	api.Get("/media/assets/:id/thumbnail", middleware.Protected(), media.GetAssetThumbnailHandler)
	api.Get("/media/assets/:id/content", middleware.Protected(), media.GetAssetContentHandler)

	// Notifications routes (protected)
	notificationRoutes := api.Group("/notifications", middleware.Protected())
	notificationRoutes.Get("/", workspace.ListNotificationsHandler)
	notificationRoutes.Delete("/", workspace.ClearNotificationsHandler)
	notificationRoutes.Post("/:id/read", workspace.MarkNotificationReadHandler)

	// Public document read routes (no authentication)
	publicRoutes := api.Group("/public", middleware.OptionalProtected())
	publicRoutes.Get("/documents/:id", workspace.GetPublicDocumentHandler)
	publicRoutes.Get("/documents/:id/content", workspace.GetPublicDocumentContentHandler)

	// Simple root route to check if server is up
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString("Hello from Cyime Server!")
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on port %s...", port)
	// Start server
	if err := app.Listen(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
