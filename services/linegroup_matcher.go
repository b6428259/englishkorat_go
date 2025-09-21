package services

import (
	"log"
	"regexp"
	"strings"

	"englishkorat_go/models"
	"englishkorat_go/database"
)

type LineGroupMatcher struct{}

func NewLineGroupMatcher() *LineGroupMatcher {
	return &LineGroupMatcher{}
}

// normalizeName ทำความสะอาด string ให้เหลือรูปแบบเทียบได้
func normalizeName(s string) string {
	// 1. lower case
	s = strings.ToLower(s)
	// 2. trim space หน้า/หลัง
	s = strings.TrimSpace(s)
	// 3. replace multiple spaces ด้วย space เดียว
	re := regexp.MustCompile(`\s+`)
	s = re.ReplaceAllString(s, " ")
	return s
}
func (m *LineGroupMatcher) MatchLineGroupsToGroups() {
	db := database.DB
	var lineGroups []models.LineGroup

	if err := db.Find(&lineGroups).Error; err != nil {
		log.Printf("❌ Error fetching LineGroups: %v", err)
		return
	}

	for _, lg := range lineGroups {
		cleanLG := normalizeName(lg.GroupName)
		log.Printf("🔍 Matching LineGroup '%s' → '%s'", lg.GroupName, cleanLG)

		var groups []models.Group
		if err := db.Find(&groups).Error; err != nil {
			log.Printf("❌ Error fetching Groups: %v", err)
			continue
		}

		matched := false
		for _, g := range groups {
			cleanG := normalizeName(g.GroupName)
			if cleanLG == cleanG {
				if lg.MatchedGroupID == nil || *lg.MatchedGroupID != g.ID {
					lg.MatchedGroupID = &g.ID
					if err := db.Save(&lg).Error; err != nil {
						log.Printf("❌ Failed to update LineGroup '%s' with MatchedGroupID=%d: %v",
							lg.GroupName, g.ID, err)
					} else {
						log.Printf("✅ Matched LineGroup '%s' → Group '%s' (ID=%d)",
							lg.GroupName, g.GroupName, g.ID)
					}
				} else {
					log.Printf("ℹ️ Already matched: '%s' (Group ID=%d)", lg.GroupName, g.ID)
				}
				matched = true
				break
			}
		}

		if !matched {
			log.Printf("⚠️ No matching Group found for LineGroup '%s' (normalized='%s')", lg.GroupName, cleanLG)
		}
	}
}