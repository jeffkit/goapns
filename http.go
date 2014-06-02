package main

import (
	"io"
	"log"
	"net/http"
	"strings"
)

/**
启动http服务，接受HTTP推送请求
*/
func StartHttpServer() {
	log.Print("starting Http server")
	http.HandleFunc("/push", pushHandler)
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Printf("http server start fail %s \n", err)
	} else {
		log.Print("http server started!")
	}
}

//////////// HTTP Method ////////////////

/**
推送消息
参数：
- app
- token
- message
- badge
- sound
- custom
- sandbox
*/
func pushHandler(w http.ResponseWriter, request *http.Request) {
	log.Print("handle push request")
	request.ParseForm()

	f := request.Form
	app := request.FormValue("app")
	if len(app) == 0 {
		io.WriteString(w, "app is required!")
		return
	}

	sandbox := false
	sb := request.FormValue("sandbox")
	if sb == "1" || sb == "true" {
		sandbox = true
		app = app + DEVELOP_SUBFIX
	}
	if sockets[app] == nil {
		io.WriteString(w, "invalid app")
		return
	}
	message := request.FormValue("message")
	badge := request.FormValue("badge")
	sound := request.FormValue("sound")
	tokens := f["token"]

	payload := &Payload{
		Aps: &AlertInfo{Alert: message, Badge: badge, Sound: sound}}
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		tokenSb := sandbox
		tokenApp := app
		if strings.HasPrefix(token, "sb:") {
			token = strings.Replace(token, "sb:", "", -1)
			tokenSb = true

			if !strings.HasSuffix(app, DEVELOP_SUBFIX) {
				tokenApp = app + DEVELOP_SUBFIX
			}
		}
		notification := &Notification{Token: token, Payload: payload, App: tokenApp, Sandbox: tokenSb}
		messageCN <- notification
	}

	io.WriteString(w, "hello go apns!")
}
