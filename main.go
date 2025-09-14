package main

import (
	"englishkorat_go/config"
	"englishkorat_go/database"
	"englishkorat_go/middleware"
	"englishkorat_go/routes"
	"englishkorat_go/services"
	"englishkorat_go/services/notifications"
	"englishkorat_go/services/websocket"
	"englishkorat_go/handlers"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/sirupsen/logrus"
)

func init() {
	// Initialize logging
	setupLogging()

	// Load configuration
	config.LoadConfig()

	// Connect to database
	database.Connect()

	// Start log maintenance scheduler
	logArchiveService := services.NewLogArchiveService()
	logArchiveService.StartLogMaintenanceScheduler()

	// Note: ScheduleManager and Notification worker will be wired and started in main()

	// ✅ Start Notification Scheduler
	notificationScheduler := services.NewNotificationScheduler()
	go notificationScheduler.StartScheduler()

	log.Printf("🔍 LINE_CHANNEL_SECRET length: %d", len(os.Getenv("LINE_CHANNEL_SECRET")))
	log.Printf("🔍 LINE_CHANNEL_ACCESS_TOKEN length: %d", len(os.Getenv("LINE_CHANNEL_ACCESS_TOKEN")))
}

func main() {
	// Create WebSocket hub first
	wsHub := websocket.NewHub()
	go wsHub.Run()

	// Create Fiber app
	app := fiber.New(fiber.Config{
		ErrorHandler: customErrorHandler,
		BodyLimit:    int(config.AppConfig.MaxFileSize),
	})

	// Global middleware
	app.Use(recover.New())
	app.Use(helmet.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins:     "*",
		AllowMethods:     "GET,POST,HEAD,PUT,DELETE,PATCH,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization",
		AllowCredentials: true,
	}))

	// Custom middleware
	app.Use(middleware.LoggerMiddleware())
	app.Use(middleware.LogActivityMiddleware())

	// Health check endpoint
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "ok",
			"service": "English Korat API",
			"version": "1.0.0",
		})
	})

	// Note: WebSocket upgrade and authentication are handled in routes.SetupRoutes

	// Wire notifications to the WebSocket hub globally so any new Service uses it (incl. schedulers)
	notifications.SetDefaultWSHub(wsHub)
	notifService := notifications.NewService()
	notifService.SetWebSocketHub(wsHub)
	if config.AppConfig.UseRedisNotifications {
		stopNotif := make(chan struct{})
		notifService.StartWorker(stopNotif)
	}

	// Start schedule management services after WebSocket hub is ready
	scheduleManager := services.NewScheduleManager()
	scheduleManager.SetWebSocketHub(wsHub)
	scheduleManager.Start()

	// API routes
	routes.SetupRoutes(app, wsHub)
	routes.SetupStaticRoutes(app)

	// ✅ LINE Webhook Route
	lineChannelSecret := os.Getenv("LINE_CHANNEL_SECRET")
	lineChannelToken := os.Getenv("LINE_CHANNEL_ACCESS_TOKEN")
	if lineChannelSecret != "" && lineChannelToken != "" {
		lineHandler := handlers.NewLineWebhookHandler(database.DB)

		// POST route สำหรับ LINE webhook จริง
		app.Post("/line/webhook", func(c *fiber.Ctx) error {
			return lineHandler.Handle(c)
		})

		// GET route สำหรับทดสอบผ่าน browser
		app.Get("/line/webhook", func(c *fiber.Ctx) error {
			return c.JSON(fiber.Map{
				"status":  "ok",
				"message": "LINE webhook endpoint ready (use POST for real events)",
			})
		})

		log.Println("✅ LINE Webhook enabled at /line/webhook")
	} else {
		log.Println("⚠️ LINE Webhook disabled: Missing LINE_CHANNEL_SECRET or LINE_CHANNEL_ACCESS_TOKEN")
	}

	for _, r := range app.Stack() {
    for _, route := range r {
        log.Printf("📌 Registered route: %s %s", route.Method, route.Path)
    }
	}

	// 404 handler
	app.Use(func(c *fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error":  "Route not found",
			"path":   c.Path(),
			"method": c.Method(),
		})
	})

	// Start server (listen on all interfaces for Docker/production)
	port := "localhost:" + config.AppConfig.Port
	log.Printf("🚀 Server starting on port %s", config.AppConfig.Port)
	log.Printf("📚 English Korat API v1.0.0")
	log.Printf("🌍 Environment: %s", config.AppConfig.AppEnv)

	if err := app.Listen(port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

// setupLogging configures the logging system
func setupLogging() {
	// Create logs directory if it doesn't exist
	if err := os.MkdirAll("logs", 0755); err != nil {
		log.Printf("Warning: Could not create logs directory: %v", err)
	}

	// Configure logrus
	logrus.SetFormatter(&logrus.JSONFormatter{})

	// Set log level
	level, err := logrus.ParseLevel("info") // Default to info
	if err == nil {
		logrus.SetLevel(level)
	}

	// Log to both file and stdout in development
	if os.Getenv("APP_ENV") == "development" {
		logrus.SetOutput(os.Stdout)
	} else {
		// In production, log to file
		file, err := os.OpenFile("logs/app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			logrus.SetOutput(file)
		}
	}
}

// customErrorHandler handles application errors
func customErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "Internal Server Error"

	// Check if it's a Fiber error
	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		message = e.Message
	}

	// Log the error
	logrus.WithFields(logrus.Fields{
		"error":  err.Error(),
		"path":   c.Path(),
		"method": c.Method(),
		"ip":     c.IP(),
		"status": code,
	}).Error("Request error")

	// Send error response
	return c.Status(code).JSON(fiber.Map{
		"error":  message,
		"code":   code,
		"path":   c.Path(),
		"method": c.Method(),
	})
}
