package main

import (
	"embed"
	"log"
	"os"

	"github.com/GabiHert/positron/internal/run"
	"github.com/GabiHert/positron/internal/speechworker"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if len(os.Args) == 3 && os.Args[1] == speechworker.Flag {
		if err := run.SpeechWorker(os.Args[2]); err != nil {
			log.Fatal(err)
		}
		return
	}
	if err := run.Run(assets); err != nil {
		log.Fatal(err)
	}
}
