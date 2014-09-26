package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
)

func Initialize(path *string) {

	if path == nil {
		return
	}

	file, err := os.Open(*path)
	if err != nil {
		log.Fatalf("config file %d not found\n", *path)
	}
	content := make([]byte, 1024)

	n, err := file.Read(content)
	if err != nil && err != io.EOF {
		log.Fatalln("error occur when reading config file!", err)
	}

	appConfig = NewConfig()

	err = json.Unmarshal(content[:n], &appConfig)
	if err != nil {
		log.Fatalln("wrong json format: ", err)
	}

	appConfig.Display()
}
