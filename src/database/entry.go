package database

import (
	"fmt"
	"log"

	"getaced.io/src/structs"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Init(dbPath string) error {
	var err error
	DB, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	if err := DB.AutoMigrate(&structs.User{}); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}

	log.Println("database initialized:", dbPath)
	return nil
}
