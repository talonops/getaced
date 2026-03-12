package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	openai "github.com/sashabaranov/go-openai"
	"outplayed.dev/src/bookmarks"
	"outplayed.dev/src/utils"
)

type Account struct {
	Email    string
	WatchDir string
	Profile  string // Chrome profile directory name (e.g. "Profile 1")
}

var answersJSONSchema = &openai.ChatCompletionResponseFormatJSONSchema{
	Name:   "answers",
	Strict: true,
	Schema: json.RawMessage(`{
		"type": "object",
		"properties": {
			"answers": {
				"type": "array",
				"items": {
					"type": "object",
					"properties": {
						"question": {"type": "string"},
						"answer":   {"type": "string"}
					},
					"required": ["question", "answer"],
					"additionalProperties": false
				}
			}
		},
		"required": ["answers"],
		"additionalProperties": false
	}`),
}

type Answer struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type AnswersResponse struct {
	Answers []Answer `json:"answers"`
}

const systemPrompt = `You are a high school student answering questions from a worksheet or assignment.

Rules:
- Number each question starting from 1
- If multiple choice, answer with just the letter (A, B, C, D, etc.)
- If open ended, answer short and casual like a real student. no formal language.
- Do not explain your reasoning unless the question asks you to
- Do not add anything extra
- Do not skip any questions

Example output:
{"answers": [{"question": "1", "answer": "B"}, {"question": "2", "answer": "its when the brain associates two things together"}]}`

func parseAnswers(text string) ([]Answer, error) {
	// Strip markdown code fences if present
	cleaned := strings.TrimSpace(text)
	if strings.HasPrefix(cleaned, "```") {
		// Remove opening fence (```json or ```)
		if idx := strings.Index(cleaned, "\n"); idx != -1 {
			cleaned = cleaned[idx+1:]
		}
		// Remove closing fence
		if idx := strings.LastIndex(cleaned, "```"); idx != -1 {
			cleaned = cleaned[:idx]
		}
		cleaned = strings.TrimSpace(cleaned)
	}

	var out AnswersResponse
	if err := json.Unmarshal([]byte(cleaned), &out); err != nil {
		return nil, fmt.Errorf("failed to parse model JSON: %w\nraw response: %s", err, text)
	}
	return out.Answers, nil
}

func getAnswersFromImage(ctx context.Context, client *openai.Client, imagePath string) ([]Answer, error) {
	data, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, err
	}

	mime := http.DetectContentType(data)
	dataURL := fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data))

	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT5ChatLatest,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
			{
				Role: openai.ChatMessageRoleUser,
				MultiContent: []openai.ChatMessagePart{
					{
						Type: openai.ChatMessagePartTypeText,
						Text: "Look at the image and answer every question you see.",
					},
					{
						Type: openai.ChatMessagePartTypeImageURL,
						ImageURL: &openai.ChatMessageImageURL{
							URL:    dataURL,
							Detail: openai.ImageURLDetailAuto,
						},
					},
				},
			},
		},
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type:       openai.ChatCompletionResponseFormatTypeJSONSchema,
			JSONSchema: answersJSONSchema,
		},
	})
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty model response")
	}

	return parseAnswers(resp.Choices[0].Message.Content)
}

func getAnswersFromReference(ctx context.Context, client *openai.Client, reference string) ([]Answer, error) {
	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openai.GPT5ChatLatest,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt,
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: fmt.Sprintf("Read the following reference and answer every question you find.\n\n--- REFERENCE ---\n%s\n--- END REFERENCE ---", reference),
			},
		},
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type:       openai.ChatCompletionResponseFormatTypeJSONSchema,
			JSONSchema: answersJSONSchema,
		},
	})
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("empty model response")
	}

	return parseAnswers(resp.Choices[0].Message.Content)
}

func updateBookmarksWithAnswers(profile string, answers []Answer) error {
	bf, err := bookmarks.Read(profile)
	if err != nil {
		return err
	}

	bookmarks.Clear(bf)

	entries := make([]bookmarks.BookmarkEntry, 0, len(answers))
	schoologyURL := "https://eriesd.schoology.com"
	for _, a := range answers {
		entries = append(entries, bookmarks.BookmarkEntry{
			Name: a.Question + ". " + a.Answer,
			URL:  schoologyURL,
		})
	}

	bookmarks.AddFolder(bf, "school work", entries)
	return bookmarks.Write(profile, bf)
}

func startChromeWithSync(profile string) {
	if runtime.GOOS != "darwin" {
		exec.Command("pkill", "-f", "Xvfb").Run()
		exec.Command("pkill", "-f", "google-chrome").Run()
		time.Sleep(2 * time.Second)
		exec.Command("Xvfb", ":99", "-screen", "0", "1024x768x24").Start()
		time.Sleep(1 * time.Second)
		cmd := exec.Command("google-chrome",
			"--no-sandbox", "--no-first-run", "--disable-gpu",
			"--profile-directory="+profile,
		)
		cmd.Env = append(os.Environ(), "DISPLAY=:99")
		cmd.Start()
		log.Println("chrome started with profile:", profile)
	}
}

// findChromeProfile scans Chrome profile directories to find which one
// is signed in with the given email.
func findChromeProfile(email string) (string, error) {
	home, _ := os.UserHomeDir()
	var chromeDir string
	switch runtime.GOOS {
	case "darwin":
		chromeDir = filepath.Join(home, "Library", "Application Support", "Google", "Chrome")
	case "windows":
		chromeDir = filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "User Data")
	default:
		chromeDir = filepath.Join(home, ".config", "google-chrome")
	}

	entries, err := os.ReadDir(chromeDir)
	if err != nil {
		return "", fmt.Errorf("read chrome dir: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		prefsPath := filepath.Join(chromeDir, e.Name(), "Preferences")
		data, err := os.ReadFile(prefsPath)
		if err != nil {
			continue
		}
		// Look for the email in account_info
		if strings.Contains(string(data), email) {
			return e.Name(), nil
		}
	}
	return "", fmt.Errorf("no chrome profile found for %s", email)
}

func processFile(ctx context.Context, client *openai.Client, acct *Account, path string) {
	log.Printf("[%s] processing: %s", acct.Email, filepath.Base(path))

	log.Println("ext:", filepath.Ext(path))
	log.Println("isImage:", utils.IsImagePath(path))
	log.Println("isHtml:", utils.IsHtmlPath(path))

	var answers []Answer
	var err error

	if utils.IsImagePath(path) {
		answers, err = getAnswersFromImage(ctx, client, path)
		log.Println(answers)
		if err != nil {
			log.Println("get answers:", err)
			return
		}
	} else if utils.IsHtmlPath(path) {
		htmlContent, err := os.ReadFile(path)
		if err != nil {
			log.Println("read html file:", err)
			return
		}

		cmd := exec.Command("python3", "utils/extract.py")
		cmd.Stdin = strings.NewReader(string(htmlContent))
		output, err := cmd.Output()
		if err != nil {
			log.Println("extract.py error:", err)
			return
		}

		answers, err = getAnswersFromReference(ctx, client, string(output))
		if err != nil {
			log.Println("get answers:", err)
			return
		}
	} else {
		log.Println("skipping unknown file type:", filepath.Base(path))
		return
	}

	if runtime.GOOS == "darwin" {
		exec.Command("pkill", "-f", "Google Chrome").Run()
	} else {
		exec.Command("pkill", "-f", "google-chrome").Run()
	}
	time.Sleep(5 * time.Second)

	if err := updateBookmarksWithAnswers(acct.Profile, answers); err != nil {
		log.Printf("[%s] update bookmarks: %v", acct.Email, err)
		return
	}

	startChromeWithSync(acct.Profile)

	log.Printf("[%s] done: %s", acct.Email, filepath.Base(path))
}

func main() {
	ctx := context.Background()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}
	client := openai.NewClient(apiKey)

	home, _ := os.UserHomeDir()

	accounts := []Account{
		{Email: "383997@eriesd.org", WatchDir: filepath.Join(home, "drive", "383997_eriesd.org")},
		{Email: "380307@eriesd.org", WatchDir: filepath.Join(home, "drive", "380307_eriesd.org")},
	}

	// Discover Chrome profile for each account
	for i := range accounts {
		profile, err := findChromeProfile(accounts[i].Email)
		if err != nil {
			log.Fatalf("finding chrome profile for %s: %v", accounts[i].Email, err)
		}
		accounts[i].Profile = profile
		log.Printf("account %s -> chrome profile %s", accounts[i].Email, profile)
	}

	// Build a map from watch dir to account for quick lookup
	watchToAccount := make(map[string]*Account)
	for i := range accounts {
		watchToAccount[accounts[i].WatchDir] = &accounts[i]
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	for _, acct := range accounts {
		if err := watcher.Add(acct.WatchDir); err != nil {
			log.Fatalf("watch %s: %v", acct.WatchDir, err)
		}
		log.Println("watching:", acct.WatchDir)
	}

	log.Println("platform:", runtime.GOOS)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&fsnotify.Create != 0 {
				dir := filepath.Dir(event.Name)
				acct, ok := watchToAccount[dir]
				if !ok {
					log.Println("no account for dir:", dir)
					continue
				}
				processFile(ctx, client, acct, event.Name)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("watcher error:", err)
		}
	}
}
