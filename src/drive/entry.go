package drive

import (
	"context"
	"fmt"
	"io"
	"regexp"
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
	Parents  []string
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

func NewServiceFromToken(token *oauth2.Token) (*drive.Service, error) {
	cfg := OAuthConfig()
	client := cfg.Client(context.Background(), token)
	srv, err := drive.NewService(context.Background(), option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("create drive service: %w", err)
	}
	return srv, nil
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

var validFolderID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

func ListNewFiles(srv *drive.Service, folderID string, since time.Time) ([]DriveFile, error) {
	if !validFolderID.MatchString(folderID) {
		return nil, fmt.Errorf("invalid folder ID format")
	}

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

// GetStartPageToken returns the initial page token for the Changes API
func GetStartPageToken(srv *drive.Service) (string, error) {
	res, err := srv.Changes.GetStartPageToken().Do()
	if err != nil {
		return "", fmt.Errorf("get start page token: %w", err)
	}
	return res.StartPageToken, nil
}

// RegisterWatch registers a push notification channel for Drive changes
func RegisterWatch(srv *drive.Service, channelID, webhookURL, pageToken, channelToken string) (int64, error) {
	channel := &drive.Channel{
		Id:         channelID,
		Type:       "web_hook",
		Address:    webhookURL,
		Token:      channelToken,
		Expiration: time.Now().Add(7 * 24 * time.Hour).UnixMilli(),
	}
	res, err := srv.Changes.Watch(pageToken, channel).Do()
	if err != nil {
		return 0, fmt.Errorf("register watch: %w", err)
	}
	return res.Expiration, nil
}

// StopWatch stops a previously registered push notification channel
func StopWatch(srv *drive.Service, channelID, resourceID string) error {
	err := srv.Channels.Stop(&drive.Channel{
		Id:         channelID,
		ResourceId: resourceID,
	}).Do()
	if err != nil {
		return fmt.Errorf("stop watch: %w", err)
	}
	return nil
}

// ListChanges returns changed files since the given page token
func ListChanges(srv *drive.Service, pageToken string) ([]DriveFile, string, error) {
	var files []DriveFile
	currentToken := pageToken

	for {
		res, err := srv.Changes.List(currentToken).
			Fields("nextPageToken, newStartPageToken, changes(file(id,name,mimeType,modifiedTime,parents),removed)").
			PageSize(100).
			Do()
		if err != nil {
			return nil, "", fmt.Errorf("list changes: %w", err)
		}

		for _, change := range res.Changes {
			if change.Removed || change.File == nil {
				continue
			}
			mime := change.File.MimeType
			if mime != "image/png" && mime != "text/html" {
				continue
			}
			modified, _ := time.Parse(time.RFC3339, change.File.ModifiedTime)
			f := DriveFile{
				ID:       change.File.Id,
				Name:     change.File.Name,
				MimeType: mime,
				Modified: modified,
			}
			// Store parents so caller can filter by folder
			f.Parents = change.File.Parents
			files = append(files, f)
		}

		if res.NewStartPageToken != "" {
			return files, res.NewStartPageToken, nil
		}
		currentToken = res.NextPageToken
	}
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
