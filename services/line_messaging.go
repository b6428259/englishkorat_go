package services

import (
	"fmt"
	"log"
	"os"

	"github.com/line/line-bot-sdk-go/linebot"
)

// LineMessagingService ดูแลการเชื่อมต่อ LINE Messaging API
type LineMessagingService struct {
	Bot *linebot.Client
}

// NewLineMessagingService สร้าง instance ใหม่
func NewLineMessagingService() *LineMessagingService {
	channelSecret := os.Getenv("LINE_CHANNEL_SECRET")
	channelToken := os.Getenv("LINE_CHANNEL_ACCESS_TOKEN")

	if channelSecret == "" || channelToken == "" {
		log.Println("⚠️ LINE Messaging API disabled: missing LINE_CHANNEL_SECRET or LINE_CHANNEL_ACCESS_TOKEN")
		return &LineMessagingService{Bot: nil}
	}

	bot, err := linebot.New(channelSecret, channelToken)
	if err != nil {
		log.Fatalf("❌ Cannot create LINE bot client: %v", err)
	}

	return &LineMessagingService{Bot: bot}
}

// SendLineMessageToGroup ส่งข้อความไปยังกลุ่มตาม GroupID
func (s *LineMessagingService) SendLineMessageToGroup(groupID string, message string) error {
	if s.Bot == nil {
		return fmt.Errorf("LINE Bot client is not initialized")
	}

	_, err := s.Bot.PushMessage(groupID, linebot.NewTextMessage(message)).Do()
	if err != nil {
		return fmt.Errorf("LINE Messaging API failed: %v", err)
	}
	return nil
}
