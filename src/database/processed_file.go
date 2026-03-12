package database

import "gorm.io/gorm"

type ProcessedFile struct {
	gorm.Model
	AccountID uint
	Account   Account `gorm:"foreignKey:AccountID"`
	FileName  string
	FilePath  string
	FileType  string // "image" or "html"
	Status    string // "success" or "error"
	Error     string
}

func RecordProcessedFile(accountID uint, fileName, filePath, fileType, status, errMsg string) error {
	return DB.Create(&ProcessedFile{
		AccountID: accountID,
		FileName:  fileName,
		FilePath:  filePath,
		FileType:  fileType,
		Status:    status,
		Error:     errMsg,
	}).Error
}

func GetProcessedFiles(accountID uint) ([]ProcessedFile, error) {
	var files []ProcessedFile
	if err := DB.Where("account_id = ?", accountID).Find(&files).Error; err != nil {
		return nil, err
	}
	return files, nil
}
