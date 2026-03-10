package main

import (
	"log"

	"github.com/fsnotify/fsnotify"
)

func main() {

	// Create new watcher.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	// Start listening for events.
	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op.String() == "CREATE" {
					log.Println("event:", event)
					if event.Has(fsnotify.Write) {
						log.Println("modified file:", event.Name)
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Println("error:", err)
			}
		}
	}()

	// Add a path.
	err = watcher.Add("/Users/yourbaba4life/Library/CloudStorage/GoogleDrive-383997@eriesd.org/My Drive/checkmate")
	if err != nil {
		log.Fatal(err)
	}

	// Block main goroutine forever.
	<-make(chan struct{})

	/*
		bf, err := bookmarks.Read()
		if err != nil {
			panic(err)
		}

		// Add a bookmark to the bar
		//bookmarks.Add(bf, "Test Bookmark", "https://example.com")

		bookmarks.Clear(bf)

		if err := bookmarks.Write(bf); err != nil {
			panic(err)
		}
	*/
}
