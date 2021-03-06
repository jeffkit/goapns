package main

import (
	"log"
	"math"
	"runtime/debug"
)

////////////////////// Global Variables ///////////////////////////

/// channels
var socketCN chan *ConnectInfo = make(chan *ConnectInfo, 10)
var messageCN chan *Notification = make(chan *Notification, 1000) // message channel
var responseCN chan *APNSRespone = make(chan *APNSRespone, 100)   // error responses from apns
var identityCN chan int32 = make(chan int32, 4)                   // id generator channel
var countDownCN chan int32 = make(chan int32)                     // countdown timer channel

// socket container
var sockets map[string]*ConnectInfo = make(map[string]*ConnectInfo)

var errorBuckets map[string]*ErrorBucket = make(map[string]*ErrorBucket)

// configs

var (
	appConfig AppConfig

	shutingDown   bool
	countDownTime int
)

var APNS_ERROR map[string]string = make(map[string]string)

const (
	APNS_ERROR_NO_ERROR             = 0
	APNS_ERROR_PROCESSING_ERROR     = 1
	APNS_ERROR_MISSING_DEVICE_TOKEN = 2
	APNS_ERROR_MISSING_TOPIC        = 3
	APNS_ERROR_MISSING_PAYLOAD      = 4
	APNS_ERROR_INVALID_TOKEN_SIZE   = 5
	APNS_ERROR_INVALID_TOPIC_SIZE   = 6
	APNS_ERROR_INVALID_PAYLOAD_SIZE = 7
	APNS_ERROR_INVALID_TOKEN        = 8
	APNS_ERROR_SHUTDOWN             = 10
	APNS_ERROR_NONE                 = 255

	DEVELOP_SUBFIX    = "_dev"
	DEVELOP_FOLDER    = "develop"
	PRODUCTION_FOLDER = "production"
	CERT_FILE_NAME    = "cer.pem"
	KEY_FILE_NAME     = "key.pem"

	APNS_ENDPOINT                  = "gateway.push.apple.com:2195"
	APNS_SANDBOX_ENDPOINT          = "gateway.sandbox.push.apple.com:2195"
	APNS_FEEDBACK_ENDPOINT         = "feedback.push.apple.com:2196"
	APNS_SANDBOX_FEEDBACK_ENDPOINT = "feedback.sandbox.push.apple.com:2196"

	SHUTDOWN_COUNTDOWN_TIME = 4

	EXTERN_MESSAGE_QUEUE_PREFIX = "goapns:message:"
)

func LogError(errno byte, msgID int32) {
	errMsg := "NO errors encountered"
	switch errno {
	case APNS_ERROR_PROCESSING_ERROR:
		errMsg = "Processing error"
	case APNS_ERROR_MISSING_DEVICE_TOKEN:
		errMsg = "Missing device token"
	case APNS_ERROR_MISSING_TOPIC:
		errMsg = "Missing topic"
	case APNS_ERROR_MISSING_PAYLOAD:
		errMsg = "Missing payload"
	case APNS_ERROR_INVALID_TOKEN_SIZE:
		errMsg = "Invalid token size"
	case APNS_ERROR_INVALID_TOPIC_SIZE:
		errMsg = "Invalid topic size"
	case APNS_ERROR_INVALID_PAYLOAD_SIZE:
		errMsg = "Invalid payload size"
	case APNS_ERROR_INVALID_TOKEN:
		errMsg = "Invalid token"
	case APNS_ERROR_SHUTDOWN:
		errMsg = "Shutdown"
	case APNS_ERROR_NONE:
		errMsg = "None (unknown)"
	}
	log.Printf("send message %d error: %s", msgID, errMsg)
}

// Identity Generator

var identity int32
var generatorRound int

func GenerateIdentity() {
	for generatorRound = 0; generatorRound < math.MaxInt32; generatorRound++ {
		for identity = getLatestIdentity(); identity < math.MaxInt32; identity++ {
			identityCN <- identity + 1
			storeLatestIdentity(identity + 1)
		}
	}
}

func GetIdentity() int32 {
	return <-identityCN
}

func CapturePanic(message string) {
	if err := recover(); err != nil {
		log.Println(message)
		log.Printf("got runtime panic %v\n, stack %s\n", err, debug.Stack())
	}
}
