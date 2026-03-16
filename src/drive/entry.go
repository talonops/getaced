package drive

import (
	"context"
	"fmt"
	"io"
	"time"

	"getaced.io/src/config"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

type DriveFile struct {
	ID       string
	Name     string
	MimeType string
	Modified time.Time
}

func OAuthConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     config.Config("GOOGLE_CLIENT_ID"),
		ClientSecret: config.Config("GOOGLE_CLIENT_SECRET"),
		RedirectURL:  config.Config("GOOGLE_DRIVE_REDIRECT_URL"),
		Scopes:       []string{"https://www.googleapis.com/auth/drive.readonly"},
		Endpoint:     google.Endpoint,
	}
}

func NewServiceFromRefreshToken(refreshToken string) (*drive.Service, error) {
	cfg := OAuthConfig()
	token := &oauth2.Token{RefreshToken: refreshToken}
	client := cfg.Client(context.Background(), token)

	srv, err := drive.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("create drive service: %w", err)
	}
	return srv, nil
}

func ListNewFiles(srv *drive.Service, folderID string, since time.Time) ([]DriveFile, error) {
	sinceStr := since.Format(time.RFC3339)
	query := fmt.Sprintf(
		"'%s' in parents and (mimeType='image/png' or mimeType='text/html') and modifiedTime > '%s' and trashed=false",
		folderID, sinceStr,
	)

	var files []DriveFile
	pageToken := ""
	for {
		call := srv.Files.List().
			Q(query).
			Fields("nextPageToken, files(id, name, mimeType, modifiedTime)").
			OrderBy("modifiedTime asc").
			PageSize(100)

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		result, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("list files: %w", err)
		}

		for _, f := range result.Files {
			modified, _ := time.Parse(time.RFC3339, f.ModifiedTime)
			files = append(files, DriveFile{
				ID:       f.Id,
				Name:     f.Name,
				MimeType: f.MimeType,
				Modified: modified,
			})
		}

		pageToken = result.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return files, nil
}

func DownloadFile(srv *drive.Service, fileID string) ([]byte, string, error) {
	file, err := srv.Files.Get(fileID).Fields("mimeType").Do()
	if err != nil {
		return nil, "", fmt.Errorf("get file metadata: %w", err)
	}

	resp, err := srv.Files.Get(fileID).Download()
	if err != nil {
		return nil, "", fmt.Errorf("download file: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read file: %w", err)
	}

	return data, file.MimeType, nil
}

func ValidateFolder(srv *drive.Service, folderID string) error {
	file, err := srv.Files.Get(folderID).Fields("mimeType").Do()
	if err != nil {
		return fmt.Errorf("get folder: %w", err)
	}
	if file.MimeType != "application/vnd.google-apps.folder" {
		return fmt.Errorf("not a folder")
	}
	return nil
}
