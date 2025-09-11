package services

import (
	"fmt"
	"time"
)

// ScheduleManager จัดการ schedulers ทั้งหมด
type ScheduleManager struct {
	notificationScheduler *NotificationScheduler
}

// NewScheduleManager สร้าง ScheduleManager ใหม่
func NewScheduleManager() *ScheduleManager {
	return &ScheduleManager{
		notificationScheduler: NewNotificationScheduler(),
	}
}

// Start เริ่มต้น schedulers ทั้งหมด
func (sm *ScheduleManager) Start() {
	fmt.Println("Starting schedule manager...")

	// เริ่ม notification scheduler
	go sm.notificationScheduler.StartScheduler()

	// เริ่ม daily reminder scheduler (ทำงานทุกวันเวลา 07:00)
	go sm.startDailyReminderScheduler()

	// เริ่ม missed session checker (ทำงานทุก 1 ชั่วโมง)
	go sm.startMissedSessionChecker()

	fmt.Println("All schedulers started successfully")
}

// startDailyReminderScheduler ตั้งเวลาส่ง daily reminder
func (sm *ScheduleManager) startDailyReminderScheduler() {
	// คำนวณเวลา 07:00 วันถัดไป
	now := time.Now()
	next7AM := time.Date(now.Year(), now.Month(), now.Day(), 7, 0, 0, 0, now.Location())

	// ถ้าเกิน 07:00 แล้ว ให้ตั้งเป็น 07:00 วันถัดไป
	if now.After(next7AM) {
		next7AM = next7AM.AddDate(0, 0, 1)
	}

	// รอจนถึง 07:00
	time.Sleep(next7AM.Sub(now))

	// ส่ง daily reminder ครั้งแรก
	sm.notificationScheduler.SendDailyScheduleReminder()

	// ตั้ง ticker ให้ทำงานทุกๆ 24 ชั่วโมง
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sm.notificationScheduler.SendDailyScheduleReminder()
		}
	}
}

// startMissedSessionChecker ตั้งเวลาตรวจสอบ missed sessions
func (sm *ScheduleManager) startMissedSessionChecker() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sm.notificationScheduler.CheckMissedSessions()
		}
	}
}

// SendTestNotification ส่ง notification ทดสอบ (สำหรับ development)
func (sm *ScheduleManager) SendTestNotification() {
	sm.notificationScheduler.CheckUpcomingSessions()
}
