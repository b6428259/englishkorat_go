package services

import (
	"englishkorat_go/database"
	"englishkorat_go/models"
	"fmt"
	"time"
)

// สร้างการลา
func CreateAbsence(groupID, sessionID uint, userId uint, reason string) (*models.Absence, error) {
	// ตรวจสอบ quota
	var quota models.GroupLeaveQuota
	if err := database.DB.Where("group_id = ?", groupID).First(&quota).Error; err != nil {
		return nil, fmt.Errorf("ไม่พบสิทธิ์การลา")
	}
	if quota.UsedQuota >= quota.TotalQuota {
		return nil, fmt.Errorf("ใช้สิทธิ์ลาครบแล้ว")
	}

	absence := models.Absence{
		GroupID:   groupID,
		SessionID: sessionID,
		Reason:    reason,
		Status:    "pending",
		CreatedBy: userId,
	}
	if err := database.DB.Create(&absence).Error; err != nil {
		return nil, err
	}
	return &absence, nil
}

// อนุมัติ / ปฏิเสธการลา
func ApproveAbsence(absenceID, adminID uint, approve bool) error {
	var absence models.Absence
	if err := database.DB.First(&absence, absenceID).Error; err != nil {
		return err
	}

	if absence.Status != "pending" {
		return fmt.Errorf("สถานะการลาได้รับการดำเนินการแล้ว")
	}

	if approve {
		absence.Status = "approved"
		// อัพเดต quota
		var quota models.GroupLeaveQuota
		if err := database.DB.Where("group_id = ?", absence.GroupID).First(&quota).Error; err == nil {
			quota.UsedQuota++
			quota.LastUsedAt = ptrTime(time.Now())
			database.DB.Save(&quota)
		}
		// อัพเดตสถานะ session
		database.DB.Model(&models.Schedule_Sessions{}).
			Where("id = ?", absence.SessionID).
			Update("status", "rescheduled")
	} else {
		absence.Status = "rejected"
	}

	absence.ApprovedBy = &adminID
	absence.ApprovedAt = ptrTime(time.Now())
	return database.DB.Save(&absence).Error
}

func ptrTime(t time.Time) *time.Time { return &t }
