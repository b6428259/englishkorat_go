package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/line/line-bot-sdk-go/linebot"
	"gorm.io/gorm"
)

type LineWebhookHandler struct {
	DB  *gorm.DB
	Bot *linebot.Client
}

func NewLineWebhookHandler(db *gorm.DB) *LineWebhookHandler {
	secret := os.Getenv("LINE_CHANNEL_SECRET")
	token := os.Getenv("LINE_CHANNEL_ACCESS_TOKEN")

	if secret == "" || token == "" {
		log.Println("⚠️ LINE credentials missing: webhook disabled")
		return &LineWebhookHandler{DB: db, Bot: nil}
	}

	bot, err := linebot.New(secret, token)
	if err != nil {
		log.Fatalf("cannot create LINE bot client: %v", err)
	}
	return &LineWebhookHandler{DB: db, Bot: bot}
}

// Handle รับ webhook event
func (h *LineWebhookHandler) Handle(c *fiber.Ctx) error {
	log.Println("📥 Received webhook request")
	log.Printf("📥 Headers: %+v", c.GetReqHeaders())

	if h.Bot == nil {
		log.Println("⚠️ LINE Bot not initialized")
		return c.SendStatus(fiber.StatusOK)
	}

	signature := c.Get("X-Line-Signature")
	if signature == "" {
		log.Println("❌ Missing signature header")
		return c.SendStatus(fiber.StatusBadRequest)
	}

	if !validateSignature(os.Getenv("LINE_CHANNEL_SECRET"), c.Body(), signature) {
		expected := computeSignature(os.Getenv("LINE_CHANNEL_SECRET"), c.Body())
		log.Printf("❌ Signature mismatch\nHeader: %s\nExpected: %s\nBody: %s",
			signature, expected, string(c.Body()))
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	// ✅ ตอบกลับ 200 ก่อน เพื่อให้ LINE Verify ผ่าน
	go func(body []byte) {
		var webhook struct {
			Events []*linebot.Event `json:"events"`
		}
		if err := json.Unmarshal(body, &webhook); err != nil {
			log.Printf("❌ Failed to parse event JSON: %v", err)
			return
		}

		for _, event := range webhook.Events {
			log.Printf("📌 Event: %s, Source: %+v\n", event.Type, event.Source)

			switch event.Type {
			case linebot.EventTypeJoin:
				groupID := event.Source.GroupID
				if groupID != "" {
					groupSummary, err := h.Bot.GetGroupSummary(groupID).Do()
					if err == nil {
						log.Printf("✅ Bot joined group: %s (%s)", groupSummary.GroupName, groupID)
						// TODO: บันทึก groupName + groupID ลง DB
					}
				}
			case linebot.EventTypeMessage:
				if message, ok := event.Message.(*linebot.TextMessage); ok {
					log.Printf("💬 Received text message: %s", message.Text)
				}
			default:
				log.Printf("ℹ️ Skipped event type: %s", event.Type)
			}
		}
	}(c.Body())

	return c.SendStatus(fiber.StatusOK)
}

// computeSignature ใช้คำนวณ expected signature เพื่อ debug
func computeSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// validateSignature ตรวจสอบว่า signature ถูกต้อง
func validateSignature(secret string, body []byte, signature string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(signature), []byte(expected))
}
