package main

import (
	"container/list"
	"crypto/tls"
	"encoding/json"
	"log"
	"path"
	"reflect"
	"strings"
	"sync"
)

type AlertObject struct {
	Body               string      `json:"body,omitempty"`
	ActionLocalizedKey string      `json:"action-loc-key,omitempty"`
	LocalizedKey       string      `json:"loc-key,omitempty"`
	LocalizedArguments interface{} `json:"loc-args,omitempty"`
	LaunchImage        string      `json:"launch-image,omitempty"`
}

func (alert *AlertObject) IsEmpty() bool {
	return len(alert.Body) == 0 && len(alert.ActionLocalizedKey) == 0 &&
		len(alert.LocalizedKey) == 0 && alert.LocalizedArguments == nil &&
		len(alert.LaunchImage) == 0
}

type AlertInfo struct {
	Alert interface{} `json:"alert,omitempty"`
	Badge int         `json:"badge,omitempty,int"`
	Sound string      `json:"sound,omitempty"`
}

func (info *AlertInfo) IsEmpty() bool {
	if info.Alert != nil {
		if inst, ok := info.Alert.(AlertObject); ok {
			if inst.IsEmpty() {
				return info.Badge == 0 && len(info.Sound) == 0
			}
		}
	} else {
		return info.Badge == 0 && len(info.Sound) == 0
	}
	return false
}

type Payload struct {
	Aps    *AlertInfo             `json:"aps,omitempty"`
	Custom map[string]interface{} `json:"custom,omitempty"`
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
			// if len(alert) < l {
			// 	return json, errors.New("payload too large")
			// }
			inst.Body = TruncateString(alert, l)
			payload.Aps.Alert = inst
		} else {
			alert := payload.Aps.Alert.(string)
			// if len(alert) < l {
			// 	return json, errors.New("payload too large")
			// }
			payload.Aps.Alert = TruncateString(alert, l)
		}
		json, err = payload.rawJson()
		return json, err
	} else {
		return json, err
	}
}

func MakePayloadFromString(str string) (payload Payload, e error) {
	var dict map[string]interface{} = make(map[string]interface{})
	err := json.Unmarshal([]byte(str), &dict)
	if err != nil {
		log.Println(err)
		return payload, err
	}
	return MakePayloadFromMap(dict)
}

func MakePayloadFromMap(dict map[string]interface{}) (payload Payload, e error) {
	var custom map[string]interface{} = make(map[string]interface{})
	for key, v := range dict {
		if key == "aps" {
			continue
		}
		custom[key] = v
		delete(dict, key)
	}
	if len(custom) != 0 {
		dict["custom"] = custom
	}

	bytes, err := json.Marshal(dict)
	if err != nil {
		return payload, err
		log.Println(err)
	}

	err = json.Unmarshal(bytes, &payload)
	if err != nil {
		return payload, err
		log.Println(err)
	}
	if reflect.ValueOf(payload.Aps.Alert).Kind() == reflect.Map {
		bytes, err := json.Marshal(payload.Aps.Alert)
		log.Println(string(bytes))
		if err != nil {
			return payload, err
			log.Println(err)
		} else {
			var obj AlertObject
			err := json.Unmarshal(bytes, &obj)
			if err != nil {
				return payload, err
				log.Println(err)
			}

			payload.Aps.Alert = obj
		}
	}
	return payload, nil
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

type AppConfig struct {
	AppsDir            string `json:",omitempty"`
	AppPort            int64  `json:",omitempty"`
	DbPath             string `json:",omitempty"`
	ConnectionIdleSecs int64  `json:",omitempty"`

	QueueWithRedis bool   `json:",omitempty"`
	RedisHost      string `json:",omitempty"`
	RedisPort      int64  `json:",omitempty"`
	RedisDB        int64  `json:",omitempty"`
	RedisPassword  string `json:",omitempty"`
	RedisPoolsize  int64  `json:",omitempty"`
}

func NewConfig() AppConfig {
	return AppConfig{
		AppsDir:            "/etc/goapns/apps",
		AppPort:            9872,
		DbPath:             "/etc/goapns/db",
		ConnectionIdleSecs: 600,
		QueueWithRedis:     false,
		RedisHost:          "localhost",
		RedisPort:          6379,
		RedisDB:            0,
		RedisPassword:      "",
		RedisPoolsize:      10,
	}
}

func (appConfig *AppConfig) Display() {
	format := `GoAPNS Config:
	appsDir:%s
	appPort:%d
	dbPath:%s
	connectionIdleSesc:%d

	queueWithRedis:%t

	redisHost:%s
	redisPort:%d
	redisDB:%d
	redisPassword:hidden, (%d)chars
	redisPoolsize:%d`
	log.Printf(format, appConfig.AppsDir, appConfig.AppPort, appConfig.DbPath, appConfig.ConnectionIdleSecs,
		appConfig.QueueWithRedis, appConfig.RedisHost, appConfig.RedisPort, appConfig.RedisDB,
		len(appConfig.RedisPassword), appConfig.RedisPoolsize)
}

/**
* 从本地到apple APNS服务器的Socket连接信息
 */
type ConnectInfo struct {
	Connection       *tls.Conn
	App              string
	Sandbox          bool
	currentIndentity int32      // 通过该连接已发送的最大ID
	number           int32      // 连接号数
	lastActivity     int64      // 最后活跃时间
	mutext           sync.Mutex // 同步锁
	listeningQueue   bool       // 正在监听redis的队列吗
}

func (info *ConnectInfo) Reconnect() {
	// renew the connection because of the long time idel.
	if info.Connection == nil {
		return
	}
	info.mutext.Lock()
	log.Println("get the connectionInfo lock")
	if info.Connection == nil {
		log.Println("already reconneting... quit!")
		info.mutext.Unlock()
		return
	}

	info.Connection.Close()
	info.Connection = nil

	info.mutext.Unlock()

	appname := info.App
	folder := path.Join(appConfig.AppsDir, appname, PRODUCTION_FOLDER)
	if info.Sandbox {
		appname = strings.Replace(appname, DEVELOP_SUBFIX, "", 1)
		folder = path.Join(appConfig.AppsDir, appname, DEVELOP_FOLDER)
	}

	go connect(appname, path.Join(folder, KEY_FILE_NAME), path.Join(folder, CERT_FILE_NAME), info.Sandbox)
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
