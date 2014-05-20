package main

import (
	"crypto/tls"
)

type AlertInfo struct {
	Alert string `json:"alert"`
	Badge int    `json:"badge"`
	Sound string `json:"sound"`
}

type Payload struct {
	Aps    *AlertInfo `json:"aps"`
	Custom interface{}
}

/**
* 要推送给用户的消息
 */
type Notification struct {
	Token   string
	Payload *Payload
	App     string
	Sandbox bool
}

/**
* 从apple APNS服务器返回的结果
 */
type APNSRespone struct {
	Command    byte
	Status     byte
	Identifier int32
	Connection *tls.Conn
	App        string
	Sandbox    bool
}

/**
* 从本地到apple APNS服务器的Socket连接信息
 */
type ConnectInfo struct {
	Connection       *tls.Conn
	App              string
	Sandbox          bool
	currentIndentity int32
}
