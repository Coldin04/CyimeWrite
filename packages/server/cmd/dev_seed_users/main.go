package main

import (
	"errors"
	"flag"
	"fmt"
	"log"

	"g.co1d.in/Coldin04/Cyime/server/internal/config"
	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func main() {
	count := flag.Int("count", 100, "number of mock users to seed")
	flag.Parse()

	if *count <= 0 {
		log.Fatalf("count must be greater than 0")
	}

	if err := config.LoadDotEnv(".env"); err != nil {
		log.Fatalf("加载 .env 失败: %v", err)
	}

	database.Connect()

	createdCount, skippedCount, err := seedMockUsers(*count)
	if err != nil {
		log.Fatalf("生成测试用户失败: %v", err)
	}

	fmt.Printf("✅ 已处理 %d 个测试用户：新增 %d，跳过 %d。\n", *count, createdCount, skippedCount)
}

func seedMockUsers(count int) (createdCount int, skippedCount int, err error) {
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		for i := 1; i <= count; i++ {
			email := fmt.Sprintf("mock-user-%03d@cyime.test", i)
			displayName := fmt.Sprintf("Mock User %03d", i)

			var existing models.User
			if err := tx.Where("email = ?", email).First(&existing).Error; err == nil {
				skippedCount++
				continue
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}

			user := models.User{
				ID:                uuid.New(),
				Email:             strPtr(email),
				EmailVerified:     true,
				DisplayName:       strPtr(displayName),
				DocumentQuotaMode: models.DocumentQuotaModeInherit,
			}
			if err := tx.Create(&user).Error; err != nil {
				return err
			}
			createdCount++
		}
		return nil
	})
	return createdCount, skippedCount, err
}

func strPtr(value string) *string {
	return &value
}
