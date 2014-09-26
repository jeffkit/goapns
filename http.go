package main

import (
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
)

/**
启动http服务，接受HTTP推送请求
*/
func StartHttpServer() error {
	log.Print("starting Http server")
	http.HandleFunc("/push", pushHandler)
	return http.ListenAndServe(":8080", nil)
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
	if shutingDown {
		io.WriteString(w, "server maintaining... please try later")
		return
	}

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
	badge, err := strconv.Atoi(request.FormValue("badge"))
	if err != nil {
		log.Println(err)
		badge = 0
	}
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
