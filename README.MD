Basically this tool scans google drive for changes in a folder, and then feeds that image into
a LLM, and extracts the question number, and answer. Then it will create a bookmark on the chromebooks
account, and put the answer as "1. A", and onwards.

so if its a .png then we will just answer 1

implement resources so you can pass in resources it can reference

OPENAI_API_KEY=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 OUTPLAYED_WATCH_DIR="/Users/yourbaba4life/Library/CloudStorage/GoogleDrive-383997@eriesd.org/My Drive/school" go run src/main.go

OPENAI_API_KEY=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 OUTPLAYED_WATCH_DIR=/home/talon/drive/school ./main

kill $(lsof -ti :3000)
