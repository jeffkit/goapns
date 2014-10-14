package main

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

func StartFeedbackService() {
	defer CapturePanic("Feedback service occur runtime error!")
	tick := time.NewTicker(1 * time.Hour)

	for {
		select {
		case _ = <-tick.C:
			log.Println("get up for the dead tokens!")
			runFeedbackJob()
		}
	}
}

func runFeedbackJob() {
	// 遍历应用，创建对应连接收取非法device
	walkErr := filepath.Walk(appConfig.AppsDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}

		if info.Name() != DEVELOP_FOLDER && info.Name() != PRODUCTION_FOLDER {
			return nil
		}

		buff := bytes.NewBufferString(appConfig.AppsDir)
		buff.WriteRune(os.PathSeparator)
		app := strings.Replace(path.Dir(filePath), buff.String(), "", 1)
		sandbox := false
		if info.Name() == DEVELOP_FOLDER {
			sandbox = true
		}
		go getFeedback(app,
			path.Join(filePath, KEY_FILE_NAME),
			path.Join(filePath, CERT_FILE_NAME),
			sandbox)
		return nil
	})

	if walkErr != nil {
		log.Print("读取证书有问题哇", walkErr)
	}
}

func getFeedback(app string, keyFile string, certFile string, sandbox bool) {
	// 连接feedback service and read。
	log.Println("get feedback")
	defer CapturePanic(fmt.Sprintf("get feedback for %s fail", app))
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Printf("server : loadKeys: %s", err)
	}
	config := tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true}
	endPoint := APNS_FEEDBACK_ENDPOINT
	if sandbox {
		endPoint = APNS_SANDBOX_FEEDBACK_ENDPOINT
	}
	conn, err := tls.Dial("tcp", endPoint, &config)
	defer conn.Close()

	if err != nil {
		log.Println("error when connection to feedback server", err)
		return
	}

	for {
		info := make([]byte, 6)
		n, err := conn.Read(info)
		if n < 6 || err != nil {
			log.Println("1 EOF? ", err)
			break
		}
		length := bytes.NewBuffer(info[4:])
		var tokenLength int16
		binary.Read(length, binary.BigEndian, &tokenLength)

		token := make([]byte, tokenLength)
		n, err = conn.Read(token)
		if n < int(tokenLength) || err != nil {
			log.Println("2 EOF? ", err)
			break
		}
		addBadToken(app, string(token))
	}
}
