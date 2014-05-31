package main

import (
	"container/list"
	"crypto/tls"
	"encoding/json"
)

type AlertObject struct {
	Body               string      `json:body`
	ActionLocalizedKey string      `json:action-loc-key`
	LocalizedKey       string      `json:loc-key`
	localizedArguments interface{} `json:loc-args`
	launchImage        string      `json:launch-image`
}

type AlertInfo struct {
	Alert interface{} `json:"alert,omitempty"`
	Badge int         `json:"badge,omitempty"`
	Sound string      `json:"sound,omitempty"`
}

type Payload struct {
	Aps    *AlertInfo
	Custom map[string]interface{}
}

func (payload *Payload) Json() ([]byte, error) {
	result := make(map[string]interface{})
	result["aps"] = payload.Aps
	if payload.Custom != nil {
		for k, v := range payload.Custom {
			result[k] = v
		}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return data, nil
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
	currentIndentity int32 // 通过该连接已发送的最大ID
	number           int32 // 连接号数
}

type ErrorBucket struct {
	ErrorMessages    *list.List
	FallbackMessages *list.List
	App              string
}

func ErrorBucketForApp(app string) *ErrorBucket {
	if errorBuckets[app] == nil {
		bucket := NewErrorBucket(app)
		errorBuckets[app] = bucket
	}
	return errorBuckets[app]
}

func NewErrorBucket(app string) *ErrorBucket {
	return &ErrorBucket{list.New(), list.New(), app}
}

func AddErrorMessage(notification *Notification) {
	bucket := ErrorBucketForApp(notification.App)
	bucket.AddErrorMessage(notification)
}

func AddFallbackMessage(notification *Notification) {
	bucket := ErrorBucketForApp(notification.App)
	bucket.AddFallbackMessage(notification)
}

func HasPendingMessage(info *ConnectInfo) bool {
	bucket := ErrorBucketForApp(info.App)
	return bucket.ErrorMessages.Len() != 0 || bucket.FallbackMessages.Len() != 0
}

func (bucket *ErrorBucket) AddErrorMessage(notification *Notification) {
	bucket.ErrorMessages.PushBack(notification)
}

func (bucket *ErrorBucket) AddFallbackMessage(notification *Notification) {
	bucket.FallbackMessages.PushBack(notification)
}

func (bucket *ErrorBucket) Next() *Notification {
	var ele *list.Element
	if bucket.ErrorMessages.Len() > 0 {
		ele = bucket.ErrorMessages.Front()
		if ele != nil {
			bucket.ErrorMessages.Remove(ele)
			return ele.Value.(*Notification)
		}
	}

	if bucket.FallbackMessages.Len() > 0 {
		ele = bucket.FallbackMessages.Front()
		if ele != nil {
			bucket.FallbackMessages.Remove(ele)
			return ele.Value.(*Notification)
		}
	}

	return nil
}
