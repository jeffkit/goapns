package main

import (
	"encoding/json"
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
	http.HandleFunc("/push2", pushHandler2)
	http.HandleFunc("/recover_token", recoverHandler)
	return http.ListenAndServe(":"+strconv.Itoa(int(appConfig.AppPort)), nil)
}

//////////// HTTP Method ////////////////

func recoverHandler(w http.ResponseWriter, request *http.Request) {
	request.ParseForm()
	app := request.FormValue("app")
	if len(app) == 0 {
		io.WriteString(w, "app is required!")
		return
	}

	sb := request.FormValue("sandbox")
	if sb == "1" || sb == "true" {
		app = app + DEVELOP_SUBFIX
	}

	token := request.FormValue("token")
	if len(token) == 0 {
		io.WriteString(w, "token is required")
		return
	}

	recoverToken(app, token)
	io.WriteString(w, "ok!")
	return
}

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
func pushHandler2(w http.ResponseWriter, request *http.Request) {
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

func pushHandler(w http.ResponseWriter, request *http.Request) {
	p := make([]byte, request.ContentLength)
	_, err := request.Body.Read(p)
	if err != nil {
		log.Println("read request body fail")
	}
	log.Println(string(p))
	var dict map[string]interface{} = make(map[string]interface{})
	err = json.Unmarshal(p, &dict)
	if err != nil {
		log.Println("error when decode json", err)
		io.WriteString(w, "error when decode json body")
		return
	}

	log.Println(dict)

	app := dict["app"].(string)

	sandbox := false
	if val, ok := dict["sandbox"]; ok {
		if sb, ok := val.(bool); ok {
			sandbox = sb
		}
	}
	payload := dict["payload"].(map[string]interface{})

	if sandbox {
		app = app + DEVELOP_SUBFIX
	}

	payloadObj, err := MakePayloadFromMap(payload)

	if err != nil {
		io.WriteString(w, "invalid payload format")
		return
	}
	token := dict["token"]
	if tk, ok := token.([]interface{}); ok {
		for t := range tk {
			message := &Notification{tk[t].(string), &payloadObj, app, sandbox}
			go Notify(message)
		}
	} else {
		message := &Notification{token.(string), &payloadObj, app, sandbox}
		go Notify(message)
	}
}
