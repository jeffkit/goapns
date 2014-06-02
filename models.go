package main

import (
	"container/list"
	"crypto/tls"
	"encoding/json"
	"errors"
)

type AlertObject struct {
	Body               string      `json:body,omitempty`
	ActionLocalizedKey string      `json:action-loc-key,omitempty`
	LocalizedKey       string      `json:loc-key,omitempty`
	localizedArguments interface{} `json:loc-args,omitempty`
	launchImage        string      `json:launch-image,omitempty`
}

func (alert *AlertObject) IsEmpty() bool {
	return len(alert.Body) == 0 && len(alert.ActionLocalizedKey) == 0 &&
		len(alert.LocalizedKey) == 0 && alert.localizedArguments == nil &&
		len(alert.launchImage) == 0
}

type AlertInfo struct {
	Alert interface{} `json:"alert,omitempty"`
	Badge string      `json:"badge,omitempty,int"`
	Sound string      `json:"sound,omitempty"`
}

func (info *AlertInfo) IsEmpty() bool {
	if info.Alert != nil {
		if inst, ok := info.Alert.(AlertObject); ok {
			if inst.IsEmpty() {
				return len(info.Badge) == 0 && len(info.Sound) == 0
			}
		}
	} else {
		return len(info.Badge) == 0 && len(info.Sound) == 0
	}
	return false
}

type Payload struct {
	Aps    *AlertInfo
	Custom map[string]interface{}
}

func TruncateString(s string, byteLength int) string {
	if len(s) <= byteLength {
		return s
	}

	result, length, rn := "", 0, []rune(s)
	for _, r := range rn {
		if len(string(r))+length > byteLength {
			break
		} else {
			result += string(r)
			length += len(string(r))
		}
	}
	return result
}

func (payload *Payload) IsEmpty() bool {
	return payload.Aps.IsEmpty() && payload.Custom == nil
}

func (payload *Payload) rawJson() ([]byte, error) {
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

func (payload *Payload) Json() ([]byte, error) {
	json, err := payload.rawJson()
	if err != nil {
		return json, err
	}

	if l := len(json) - 256; l > 0 {
		if inst, ok := payload.Aps.Alert.(AlertObject); ok {
			alert := inst.Body
			if len(alert) < l {
				return json, errors.New("payload too large")
			}
			inst.Body = TruncateString(alert, l)
			payload.Aps.Alert = inst
		} else {
			alert := payload.Aps.Alert.(string)
			if len(alert) < l {
				return json, errors.New("payload too large")
			}
			payload.Aps.Alert = TruncateString(alert, l)
		}
		json, err = payload.rawJson()
		return json, err
	} else {
		return json, err
	}
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
