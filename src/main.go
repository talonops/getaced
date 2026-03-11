package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"outplayed.dev/src/bookmarks"
)

var answersJSONSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"answers": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"question": map[string]interface{}{"type": "string"},
					"answer":   map[string]interface{}{"type": "string"},
				},
				"required":             []string{"question", "answer"},
				"additionalProperties": false,
			},
		},
	},
	"required":             []string{"answers"},
	"additionalProperties": false,
}

type Answer struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type AnswersResponse struct {
	Answers []Answer `json:"answers"`
}

func isImagePath(path string) bool {
	return strings.ToLower(filepath.Ext(path)) == ".png"
}

func isHtmlPath(path string) bool {
	return strings.ToLower(filepath.Ext(path)) == ".html"
}

func getAnswersFromImage(ctx context.Context, client openai.Client, imagePath string) ([]Answer, error) {
	resp, err := client.Responses.New(ctx, responses.ResponseNewParams{
		Model: openai.ChatModelGPT4o,
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: []responses.ResponseInputItemUnionParam{
				responses.ResponseInputItemParamOfMessage(
					responses.ResponseInputMessageContentListParam{
						{OfInputText: &responses.ResponseInputTextParam{
							Text: `You are a high school student answering questions from a worksheet or assignment.
Look at the image and answer every question you see.

Rules:
- If multiple choice, just give the letter (A, B, C, D, etc.)
- If open ended, answer short and simple like a student would. casual, not formal. like you understood it but your not trying to impress anyone.
- Do not explain your reasoning unless the question asks you to
- Do not add anything extra
- Do not skip any questions

Return JSON in this format:
{
  "answers": [
    { "question": "1", "answer": "your answer" }
  ]
}`,
						}},
						{OfInputImage: &responses.ResponseInputImageParam{
							ImageURL: openai.String(imagePath),
						}},
					},
					responses.EasyInputMessageRoleUser,
				),
			},
		},
	})
	if err != nil {
		return nil, err
	}

	text := resp.OutputText()
	if text == "" {
		return nil, fmt.Errorf("empty model response")
	}

	var out AnswersResponse
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, fmt.Errorf("failed to parse model JSON: %w\nraw response: %s", err, text)
	}

	return out.Answers, nil
}

func getAnswersFromReference(ctx context.Context, client openai.Client, reference string) ([]Answer, error) {
	resp, err := client.Responses.New(ctx, responses.ResponseNewParams{
		Model: openai.ChatModelGPT4o,
		Input: responses.ResponseNewParamsInputUnion{
			OfInputItemList: []responses.ResponseInputItemUnionParam{
				responses.ResponseInputItemParamOfMessage(
					responses.ResponseInputMessageContentListParam{
						{OfInputText: &responses.ResponseInputTextParam{
							Text: fmt.Sprintf(`
You are a high school student answering questions.
Read the following reference and answer every question you find.

--- REFERENCE ---
%s
--- END REFERENCE ---

Rules:
- If multiple choice, just give the letter (A, B, C, D, etc.)
- If open ended, answer short and simple like a student would. casual, not formal. like you understood it but your not trying to impress anyone.
- Do not explain your reasoning unless the question asks you to
- Do not add anything extra
- Do not skip any questions

Return JSON in this format:
{
  "answers": [
    { "question": "1", "answer": "your answer" }
  ]
}
`, reference),
						}},
					},
					responses.EasyInputMessageRoleUser,
				),
			},
		},
		Text: responses.ResponseTextConfigParam{
			Format: responses.ResponseFormatTextConfigUnionParam{
				OfJSONSchema: &responses.ResponseFormatTextJSONSchemaConfigParam{
					Name:   "answers",
					Schema: answersJSONSchema,
					Strict: openai.Bool(true),
				},
			},
		},
	})
	if err != nil {
		return nil, err
	}

	text := resp.OutputText()
	if text == "" {
		return nil, fmt.Errorf("empty model response")
	}

	var out AnswersResponse
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return nil, fmt.Errorf("failed to parse model JSON: %w\nraw response: %s", err, text)
	}

	return out.Answers, nil
}

func updateBookmarksWithAnswers(answers []Answer) error {
	bf, err := bookmarks.Read()
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
	return bookmarks.Write(bf)
}

func waitForWrite(path string) {
	var lastSize int64 = -1
	for {
		info, err := os.Stat(path)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if info.Size() == lastSize {
			return
		}
		lastSize = info.Size()
		time.Sleep(1 * time.Second)
	}
}

func processFile(ctx context.Context, client openai.Client, path string) {
	log.Println("processing:", filepath.Base(path))
	waitForWrite(path)
	log.Println("file ready:", filepath.Base(path))

	log.Println("ext:", filepath.Ext(path))
	log.Println("isImage:", isImagePath(path))
	log.Println("isHtml:", isHtmlPath(path))

	// wait for file to finish writing
	time.Sleep(2 * time.Second)

	var answers []Answer
	var err error

	if isImagePath(path) {
		answers, err = getAnswersFromImage(ctx, client, path)
		log.Println(answers)
		if err != nil {
			log.Println("get answers:", err)
			return
		}
	} else if isHtmlPath(path) {
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
	time.Sleep(2 * time.Second)

	if err := updateBookmarksWithAnswers(answers); err != nil {
		log.Println("update bookmarks:", err)
		return
	}

	if runtime.GOOS == "darwin" {
		exec.Command("open", "-a", "Google Chrome").Run()
	} else {
		exec.Command("xvfb-run", "google-chrome", "--no-sandbox", "--no-first-run", "--disable-gpu").Start()
	}

	log.Println("done:", filepath.Base(path))
}

var lastProcessed = make(map[string]time.Time)

func main() {
	ctx := context.Background()

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY environment variable is required")
	}
	client := openai.NewClient(option.WithAPIKey(apiKey))

	watchDir := os.Getenv("OUTPLAYED_WATCH_DIR")
	if watchDir == "" {
		log.Fatal("OUTPLAYED_WATCH_DIR environment variable is required")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	if err := watcher.Add(watchDir); err != nil {
		log.Fatal(err)
	}

	log.Println("watching:", watchDir)
	log.Println("platform:", runtime.GOOS)

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
				if last, exists := lastProcessed[event.Name]; exists && time.Since(last) < 10*time.Second {
					continue
				}
				lastProcessed[event.Name] = time.Now()
				processFile(ctx, client, event.Name)
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Println("watcher error:", err)
		}
	}
}
