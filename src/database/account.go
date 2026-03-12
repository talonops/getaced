package database

import "gorm.io/gorm"

type Account struct {
	gorm.Model
	Email    string `gorm:"uniqueIndex"`
	WatchDir string
	Profile  string // Chrome profile directory name (e.g. "Profile 1")
}

func CreateAccount(email, watchDir, profile string) (*Account, error) {
	acct := Account{Email: email, WatchDir: watchDir, Profile: profile}
	if err := DB.Create(&acct).Error; err != nil {
		return nil, err
	}
	return &acct, nil
}

func GetAccountByEmail(email string) (*Account, error) {
	var acct Account
	if err := DB.Where("email = ?", email).First(&acct).Error; err != nil {
		return nil, err
	}
	return &acct, nil
}

func GetAllAccounts() ([]Account, error) {
	var accounts []Account
	if err := DB.Find(&accounts).Error; err != nil {
		return nil, err
	}
	return accounts, nil
}

func UpdateAccountProfile(email, profile string) error {
	return DB.Model(&Account{}).Where("email = ?", email).Update("profile", profile).Error
}
