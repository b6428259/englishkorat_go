package controllers

import (
	"englishkorat_go/config"
	"englishkorat_go/database"
	"englishkorat_go/models"
	"englishkorat_go/services/websocket"
	"log"
	"net/http"

	"github.com/gofiber/fiber/v2"
	fiberws "github.com/gofiber/websocket/v2"
	"github.com/golang-jwt/jwt/v4"
)

type WebSocketController struct {
	hub *websocket.Hub
}

type Claims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	BranchID uint   `json:"branch_id"`
	jwt.RegisteredClaims
}

// validateJWT validates a JWT token and returns user info
func (wsc *WebSocketController) validateJWT(tokenString string) (*models.User, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(config.AppConfig.JWTSecret), nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, jwt.ErrInvalidKey
	}

	// Verify user still exists and is active
	var user models.User
	if err := database.DB.Where("id = ? AND status = ?", claims.UserID, "active").First(&user).Error; err != nil {
		return nil, err
	}

	return &user, nil
}

func NewWebSocketController(hub *websocket.Hub) *WebSocketController {
	return &WebSocketController{
		hub: hub,
	}
}

// HandleWebSocket upgrades HTTP connection to WebSocket for notifications using Fiber middleware
func (wsc *WebSocketController) HandleWebSocket(c *fiber.Ctx) error {
	// This should not be called directly - use the websocket middleware route instead
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
		"error": "Use the WebSocket endpoint: ws://<host>/ws?token=YOUR_JWT",
	})
}

// WebSocketHandler returns a Fiber WebSocket handler that validates JWT and connects to hub
func (wsc *WebSocketController) WebSocketHandler() fiber.Handler {
	return fiberws.New(func(c *fiberws.Conn) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("WebSocket handler panic: %v", r)
			}
		}()

		// Get token from query parameter
		token := c.Query("token")
		if token == "" {
			log.Println("WebSocket connection rejected: missing token")
			c.WriteMessage(fiberws.CloseMessage, []byte("Missing token"))
			c.Close()
			return
		}

		// Parse and validate JWT token
		user, err := wsc.validateJWT(token)
		if err != nil {
			log.Printf("WebSocket connection rejected: invalid token: %v", err)
			c.WriteMessage(fiberws.CloseMessage, []byte("Invalid token"))
			c.Close()
			return
		}

		log.Printf("WebSocket connection established for user ID: %d (%s)", user.ID, user.Username)

		// Use the hub's Fiber websocket handler
		wsc.hub.ServeFiberWS(c, user.ID)
	})
}

// HandleWebSocketHTTP handles WebSocket upgrade using standard HTTP handler (legacy)
func (wsc *WebSocketController) HandleWebSocketHTTP(w http.ResponseWriter, r *http.Request, userID uint) {
	wsc.hub.ServeWS(w, r, userID)
}

// GetWebSocketStats returns WebSocket connection statistics (admin only)
func (wsc *WebSocketController) GetWebSocketStats(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"connected_clients": wsc.hub.GetClientCount(),
		"status":            "active",
	})
}
