package seeders

import (
	"englishkorat_go/database"
	"englishkorat_go/models"
	"englishkorat_go/utils"
	"encoding/json"
	"log"
	"time"
)

// SeedAll runs all seeders
func SeedAll() {
	log.Println("Starting database seeding...")

	SeedBranches()
	SeedUsers()
	SeedStudents() 
	SeedTeachers()
	SeedRooms()
	SeedCourses()

	log.Println("Database seeding completed successfully!")
}

// SeedBranches seeds the branches table
func SeedBranches() {
	var count int64
	database.DB.Model(&models.Branch{}).Count(&count)
	if count > 0 {
		log.Println("Branches already seeded, skipping...")
		return
	}

	branches := []models.Branch{
		{
			BaseModel: models.BaseModel{ID: 1, CreatedAt: time.Date(2025, 8, 15, 2, 28, 56, 0, time.UTC)},
			NameEn:    "Branch 1 The Mall Branch",
			NameTh:    "สาขา 1 เดอะมอลล์โคราช",
			Code:      "MALL",
			Address:   "The Mall Korat, Nakhon Ratchasima",
			Phone:     "044-123456",
			Type:      "offline",
			Active:    true,
		},
		{
			BaseModel: models.BaseModel{ID: 2, CreatedAt: time.Date(2025, 8, 15, 2, 28, 56, 0, time.UTC)},
			NameEn:    "Branch 2 Technology Branch",
			NameTh:    "สาขา 2 มหาวิทยาลัยเทคโนโลยีราชมงคลอีสาน",
			Code:      "RMUTI",
			Address:   "RMUTI, Nakhon Ratchasima",
			Phone:     "044-123457",
			Type:      "offline",
			Active:    true,
		},
		{
			BaseModel: models.BaseModel{ID: 3, CreatedAt: time.Date(2025, 8, 15, 2, 28, 56, 0, time.UTC)},
			NameEn:    "Online Branch",
			NameTh:    "แบบออนไลน์",
			Code:      "ONLINE",
			Address:   "Virtual Campus",
			Phone:     "044-123458",
			Type:      "online",
			Active:    true,
		},
	}

	for _, branch := range branches {
		if err := database.DB.Create(&branch).Error; err != nil {
			log.Printf("Error seeding branch %s: %v", branch.Code, err)
		}
	}

	log.Println("Branches seeded successfully")
}

// SeedUsers seeds the users table
func SeedUsers() {
	var count int64
	database.DB.Model(&models.User{}).Count(&count)
	if count > 0 {
		log.Println("Users already seeded, skipping...")
		return
	}

	// Hash the default password
	hashedPassword, _ := utils.HashPassword("password123")

	users := []models.User{
		{
			BaseModel: models.BaseModel{ID: 1, CreatedAt: time.Date(2025, 8, 15, 2, 28, 56, 0, time.UTC)},
			Username:  "admin",
			Password:  hashedPassword,
			Email:     "admin@englishkorat.com",
			Phone:     "0812345678",
			LineID:    "admin_ekls",
			Role:      "admin",
			BranchID:  1,
			Status:    "active",
			Avatar:    "avatars/1/2025/08/21/635e0f1149d42546.webp",
		},
		{
			BaseModel: models.BaseModel{ID: 2, CreatedAt: time.Date(2025, 8, 15, 2, 28, 56, 0, time.UTC)},
			Username:  "owner",
			Password:  hashedPassword,
			Email:     "owner@englishkorat.com",
			Phone:     "0812345679",
			LineID:    "owner_ekls",
			Role:      "owner",
			BranchID:  1,
			Status:    "active",
			Avatar:    "avatars/2/2025/08/20/c424a3c7cc93c92b.webp",
		},
		{
			BaseModel: models.BaseModel{ID: 3, CreatedAt: time.Date(2025, 8, 15, 2, 28, 56, 0, time.UTC)},
			Username:  "alice_w",
			Password:  hashedPassword,
			Email:     "alice.wilson@gmail.com",
			Phone:     "0891234567",
			LineID:    "alice_ekls",
			Role:      "student",
			BranchID:  1,
			Status:    "active",
		},
		{
			BaseModel: models.BaseModel{ID: 8, CreatedAt: time.Date(2025, 8, 15, 2, 28, 56, 0, time.UTC)},
			Username:  "teacher_john",
			Password:  hashedPassword,
			Email:     "john.smith@englishkorat.com",
			Phone:     "0896789012",
			LineID:    "john_teacher",
			Role:      "teacher",
			BranchID:  1,
			Status:    "active",
		},
	}

	for _, user := range users {
		if err := database.DB.Create(&user).Error; err != nil {
			log.Printf("Error seeding user %s: %v", user.Username, err)
		}
	}

	log.Println("Users seeded successfully")
}

// SeedStudents seeds the students table
func SeedStudents() {
	var count int64
	database.DB.Model(&models.Student{}).Count(&count)
	if count > 0 {
		log.Println("Students already seeded, skipping...")
		return
	}

	// Create availability schedule JSON
	availabilitySchedule := map[string]interface{}{
		"monday":    []map[string]string{{"start_time": "17:00:00", "end_time": "19:00:00"}},
		"wednesday": []map[string]string{{"start_time": "17:00:00", "end_time": "19:00:00"}},
		"friday":    []map[string]string{{"start_time": "19:00:00", "end_time": "21:00:00"}},
	}
	scheduleJSON, _ := json.Marshal(availabilitySchedule)

	students := []models.Student{
		{
			BaseModel:               models.BaseModel{ID: 1, CreatedAt: time.Date(2025, 8, 15, 2, 28, 57, 0, time.UTC)},
			UserID:                  3,
			FirstName:               "อลิซ",
			LastName:                "วิลสัน",
			Nickname:                "Alice",
			Age:                     25,
			AgeGroup:                "adults",
			CEFRLevel:               "B1",
			GrammarScore:           75,
			SpeakingScore:          70,
			ListeningScore:         80,
			ReadingScore:           77,
			WritingScore:           75,
			LearningPreferences:    "Works in hospitality industry, wants to improve English for career advancement",
			AvailabilitySchedule:   scheduleJSON,
			PreferredTeacherType:   "native",
			RegistrationStatus:     "finding_group",
			DepositAmount:          3000,
			PaymentStatus:          "partial",
			LastStatusUpdate:       &time.Time{},
			DaysWaiting:            28,
		},
	}

	for _, student := range students {
		if err := database.DB.Create(&student).Error; err != nil {
			log.Printf("Error seeding student with UserID %d: %v", student.UserID, err)
		}
	}

	log.Println("Students seeded successfully")
}

// SeedTeachers seeds the teachers table
func SeedTeachers() {
	var count int64
	database.DB.Model(&models.Teacher{}).Count(&count)
	if count > 0 {
		log.Println("Teachers already seeded, skipping...")
		return
	}

	teachers := []models.Teacher{
		{
			BaseModel:       models.BaseModel{ID: 1, CreatedAt: time.Date(2025, 8, 20, 6, 15, 59, 0, time.UTC)},
			UserID:          1,
			FirstName:       "Admin",
			LastName:        "",
			Nickname:        "Admin",
			TeacherType:     "Both",
			Specializations: "Admin who can also teach",
			Active:          true,
			BranchID:        1,
		},
		{
			BaseModel:       models.BaseModel{ID: 2, CreatedAt: time.Date(2025, 8, 20, 6, 15, 59, 0, time.UTC)},
			UserID:          8,
			FirstName:       "John",
			LastName:        "Smith",
			Nickname:        "John",
			TeacherType:     "Both",
			Specializations: "Adult Conversation, IELTS Preparation",
			Certifications:  "TESOL Certificate, IELTS Trainer",
			Active:          true,
			BranchID:        1,
		},
	}

	for _, teacher := range teachers {
		if err := database.DB.Create(&teacher).Error; err != nil {
			log.Printf("Error seeding teacher with UserID %d: %v", teacher.UserID, err)
		}
	}

	log.Println("Teachers seeded successfully")
}

// SeedRooms seeds the rooms table
func SeedRooms() {
	var count int64
	database.DB.Model(&models.Room{}).Count(&count)
	if count > 0 {
		log.Println("Rooms already seeded, skipping...")
		return
	}

	// Create equipment JSON
	equipment1, _ := json.Marshal([]string{"whiteboard", "projector", "air_conditioning"})
	equipment2, _ := json.Marshal([]string{"whiteboard", "speakers", "air_conditioning"})
	equipment3, _ := json.Marshal([]string{"zoom_pro", "breakout_rooms", "recording"})

	rooms := []models.Room{
		{
			BaseModel: models.BaseModel{ID: 1, CreatedAt: time.Date(2025, 8, 15, 2, 28, 57, 0, time.UTC)},
			BranchID:  1,
			RoomName:  "Room A1",
			Capacity:  8,
			Equipment: equipment1,
			Status:    "available",
		},
		{
			BaseModel: models.BaseModel{ID: 2, CreatedAt: time.Date(2025, 8, 15, 2, 28, 57, 0, time.UTC)},
			BranchID:  1,
			RoomName:  "Room A2",
			Capacity:  6,
			Equipment: equipment2,
			Status:    "available",
		},
		{
			BaseModel: models.BaseModel{ID: 6, CreatedAt: time.Date(2025, 8, 15, 2, 28, 57, 0, time.UTC)},
			BranchID:  3,
			RoomName:  "Virtual Room 1",
			Capacity:  20,
			Equipment: equipment3,
			Status:    "available",
		},
	}

	for _, room := range rooms {
		if err := database.DB.Create(&room).Error; err != nil {
			log.Printf("Error seeding room %s: %v", room.RoomName, err)
		}
	}

	log.Println("Rooms seeded successfully")
}

// SeedCourses seeds the courses table
func SeedCourses() {
	var count int64
	database.DB.Model(&models.Course{}).Count(&count)
	if count > 0 {
		log.Println("Courses already seeded, skipping...")
		return
	}

	courses := []models.Course{
		{
			BaseModel:   models.BaseModel{ID: 47, CreatedAt: time.Date(2025, 8, 15, 2, 28, 58, 0, time.UTC)},
			Name:        "TOEIC Foundation",
			Code:        "TECH-TOEIC-FOUND",
			CourseType:  "toeic_prep",
			BranchID:    2,
			Description: "TOEIC preparation foundation",
			Status:      "active",
			CategoryID:  5,
			Level:       "Foundation",
		},
		{
			BaseModel:   models.BaseModel{ID: 57, CreatedAt: time.Date(2025, 8, 15, 2, 28, 58, 0, time.UTC)},
			Name:        "Online Kids Conversation",
			Code:        "ONLINE-CONV-KIDS",
			CourseType:  "conversation_kids",
			BranchID:    3,
			Description: "Online conversation for kids",
			Status:      "active",
			CategoryID:  1,
			Level:       "Kids",
		},
		{
			BaseModel:   models.BaseModel{ID: 72, CreatedAt: time.Date(2025, 8, 19, 13, 49, 32, 0, time.UTC)},
			Name:        "Contact Admin - เพื่อหาคอร์สที่เหมาะสม",
			Code:        "CONTACT",
			CourseType:  "",
			BranchID:    3,
			Description: "",
			Status:      "active",
			CategoryID:  8,
			Level:       "AM",
		},
	}

	for _, course := range courses {
		if err := database.DB.Create(&course).Error; err != nil {
			log.Printf("Error seeding course %s: %v", course.Code, err)
		}
	}

	log.Println("Courses seeded successfully")
}