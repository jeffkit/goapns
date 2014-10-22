package main

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"gopkg.in/redis.v2"
	"log"
	"os"
	"path"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
)

func connect(app string, keyFile string, certFile string, sandbox bool) {
	defer CapturePanic(fmt.Sprintf("connection to apns server error %s", app))
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Printf("server : loadKeys: %s", err)
	}
	config := tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true}
	endPoint := APNS_ENDPOINT
	if sandbox {
		endPoint = APNS_SANDBOX_ENDPOINT
	}
	conn, err := tls.Dial("tcp", endPoint, &config)
	if err != nil {
		log.Println("连接服务器有误", err)
		return
	}
	log.Println("client is connect to ", conn.RemoteAddr())
	state := conn.ConnectionState()

	log.Println("client: hand shake ", state.HandshakeComplete)
	log.Println("client: mutual", state.NegotiatedProtocolIsMutual)
	if sandbox {
		app = app + DEVELOP_SUBFIX
	}
	info := &ConnectInfo{Connection: conn, App: app, Sandbox: sandbox, lastActivity: time.Now().Unix()}
	socketCN <- info
}

/**
* 监听APNSSocket的返回结果，当有返回时，意味着发生错误了，这时把错误发到channel，同时关闭socket。
 */
func monitorConn(conn *tls.Conn, app string, sandbox bool) {
	defer CapturePanic(fmt.Sprint("panic when monitor Connection %s", app))
	defer conn.Close()
	reply := make([]byte, 6)
	n, err := conn.Read(reply)

	if err != nil && reply[0] != 8 {
		if strings.HasSuffix(err.Error(), "use of closed network connection") {
			log.Println("close the network connection")
			return
		} else {
			log.Printf("error when read from socket %s, %d", err, n)
		}
	}
	log.Printf("return %x, the id is %x", reply, reply[2:])
	buf := bytes.NewBuffer(reply[2:])
	var id int32
	binary.Read(buf, binary.BigEndian, &id)

	rsp := &APNSRespone{reply[0], reply[1], id, conn, app, sandbox}
	responseCN <- rsp
}

func SocketConnected(info *ConnectInfo) {
	defer CapturePanic("panic after socket connected")
	app := info.App
	if sockets[app] == nil {
		sockets[app] = info
	} else {
		sockets[app].Connection = info.Connection
		sockets[app].lastActivity = info.lastActivity
	}
	go monitorConn(info.Connection, info.App, info.Sandbox)

	// 有没有在监听redis队列？
	if !sockets[app].listeningQueue && appConfig.QueueWithRedis {
		go WatchMessageQueue(app)
	}

	// 看看有没有消息需要重新发。
	if HasPendingMessage(info) {
		log.Println("has pending message ! ")
		bucket := ErrorBucketForApp(info.App)
		for {
			log.Println("send pending message")
			notification := bucket.Next()
			if notification == nil {
				log.Println("ok, quit")
				break
			}
			go Notify(notification)
		}
	}
}

func WatchMessageQueue(app string) {
	defer CapturePanic("panic when watch message queue")

	cli := redis.NewTCPClient(&redis.Options{
		Addr:        fmt.Sprintf("%s:%d", appConfig.RedisHost, appConfig.RedisPort),
		Password:    appConfig.RedisPassword,
		DB:          appConfig.RedisDB,
		PoolSize:    int(appConfig.RedisPoolsize),
		DialTimeout: 10 * time.Second,
	})
	sockets[app].listeningQueue = true
	for {
		msg := cli.BRPop(20, EXTERN_MESSAGE_QUEUE_PREFIX+app)

		if shutingDown {
			//把msg格式转换成Notification，然后入内部队列。
			result, err := msg.Result()
			if err != nil {
				log.Println("shuting down, return last message back to queue")
				if len(result) > 1 {
					cli.RPush(EXTERN_MESSAGE_QUEUE_PREFIX+app, result[1])
				}

			}
			break
		}

		result, err := msg.Result()
		if err != nil {
			if err != redis.Nil && !strings.Contains(err.Error(), "i/o timeout") {
				errMsg := "you need to check the redis config and make sure the redis server is running"
				log.Printf("ERROR: redis: %s, %s", err, errMsg)
				time.Sleep(5 * time.Second)
			}
			continue
		}
		// result[0]为键名 result[1]为键值
		var dict map[string]interface{} = make(map[string]interface{})
		err = json.Unmarshal([]byte(result[1]), &dict)
		if err != nil {
			log.Println(err)
			continue
		}
		payload, err := MakePayloadFromMap(dict["payload"].(map[string]interface{}))
		if err != nil {
			log.Println(err)
			continue
		}
		sandbox := dict["sandbox"].(bool)
		token := dict["token"]
		if tk, ok := token.([]interface{}); ok {
			for t := range tk {
				message := &Notification{tk[t].(string), &payload, app, sandbox}
				go Notify(message)
			}
		} else {
			message := &Notification{token.(string), &payload, app, sandbox}
			go Notify(message)
		}
	}

}

/**
初始化socket连接，创建完后扔给channel
*/
func MakeSocket() (e error) {
	// 创建几个socket？创建完后，由谁管理。
	walkErr := filepath.Walk(appConfig.AppsDir, func(filePath string, info os.FileInfo, err error) error {
		defer CapturePanic(fmt.Sprintf("unkonw error when walk to appsDir %s, filePath %s, info.name %s", appConfig.AppsDir, filePath, info.Name()))

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
		log.Println("create socket for app :", app)
		sandbox := false
		if info.Name() == DEVELOP_FOLDER {
			sandbox = true
		}
		go connect(app,
			path.Join(filePath, KEY_FILE_NAME),
			path.Join(filePath, CERT_FILE_NAME),
			sandbox)
		return nil
	})

	if walkErr != nil {
		log.Print("读取证书有问题哇", walkErr)
		return walkErr
	} else {
		return nil
	}
}

func Notify(message *Notification) {
	defer CapturePanic("notify fail")
	// 先看该token是否在badtoken集合内
	if isBadToken(message.App, message.Token) {
		log.Println("token is a bad token ,skip push :", message.Token)
		return
	}
	// 根据app找到相应的socket。
	info := sockets[message.App]
	conn := info.Connection
	if conn == nil {
		// 扔进等待队列。
		AddErrorMessage(message)
		return
	}

	if time.Now().Unix()-info.lastActivity > appConfig.ConnectionIdleSecs {
		log.Println("connection is idle for a long time, reconnect!")
		go info.Reconnect()
		AddFallbackMessage(message)
		return
	}

	// 如果ErrorBucket内有东西，等待处理完毕，先扔回去。
	if HasPendingMessage(info) {
		log.Println("has peding message!")
		AddFallbackMessage(message)
		return
	}

	msgID := GetIdentity()
	log.Println("store message into leveldb")
	StoreMessage(message, msgID, info.number)
	// 消息存入缓存，过期消失，如果失败会尝试重发。
	log.Println("push! ", message.App, message.Token)
	err := pushMessage(conn, message.Token, msgID, message.Payload)
	if err != nil {
		if err.Error() == "socket write error" {
			AddFallbackMessage(message)
		}
		log.Println(err)
		return
	}
	info.currentIndentity = msgID
	info.lastActivity = time.Now().Unix()
	log.Println("finish push")
}

func pushMessage(conn *tls.Conn, token string, identity int32, payload *Payload) error {
	if len(token) == 0 {
		return errors.New("missing token")
	}

	if payload == nil || payload.IsEmpty() {
		return errors.New("not a valid payload")
	}

	buf := new(bytes.Buffer)

	// command
	var command byte = 1
	err := binary.Write(buf, binary.BigEndian, command)
	if err != nil {
		return err
	}

	// identifier
	err = binary.Write(buf, binary.BigEndian, identity)
	if err != nil {
		return err
	}

	// expires
	var expires int32 = int32(time.Now().AddDate(0, 0, 1).Unix())
	err = binary.Write(buf, binary.BigEndian, expires)
	if err != nil {
		return err
	}

	// token length
	var tokenLength int16 = 32
	err = binary.Write(buf, binary.BigEndian, tokenLength)
	if err != nil {
		return err
	}

	// token content
	tokenBytes, err := hex.DecodeString(token)
	if len(tokenBytes) != int(tokenLength) {
		return errors.New("invalid token!")
	}

	err = binary.Write(buf, binary.BigEndian, tokenBytes)
	if err != nil {
		return err
	}

	// payload length

	payloadBytes, err := payload.Json()
	if err != nil {
		return err
	}

	fmt.Printf("payload %s\n", string(payloadBytes))
	var payloadLength int16 = int16(len(payloadBytes))
	err = binary.Write(buf, binary.BigEndian, payloadLength)
	if err != nil {
		return err
	}

	// payload content
	err = binary.Write(buf, binary.BigEndian, payloadBytes)
	if err != nil {
		return err
	}

	// write to socket
	size, err := conn.Write(buf.Bytes())
	fmt.Printf("body: %x\n", buf.Bytes())
	log.Printf("write body size %d", size)
	if err != nil {
		log.Printf("error when write to socket %s, %d", err, size)
		return errors.New("socket write error")
	}
	return nil
}

/**
处理APNS服务器返回的错误：
- 记录发送失败的原因
- 重发indentifier之后的消息。
*/
func HandleError(err *APNSRespone) {
	socketKey := err.App
	dir := path.Join(appConfig.AppsDir, err.App)
	defer func(message string) {
		appname := err.App
		if err.Sandbox {
			appname = strings.Replace(err.App, DEVELOP_SUBFIX, "", 1)
		}
		go connect(appname,
			path.Join(dir, KEY_FILE_NAME),
			path.Join(dir, CERT_FILE_NAME),
			err.Sandbox)
		if e := recover(); e != nil {
			log.Println(message)
			log.Printf("got runtime panic %v\n, stack %s\n", e, debug.Stack())
		}
	}("fail to handle error")

	// 干掉这条socket

	if err.Sandbox {
		dir = path.Join(strings.Replace(dir, DEVELOP_SUBFIX, "", 1), DEVELOP_FOLDER)
	} else {
		dir = path.Join(dir, PRODUCTION_FOLDER)
	}
	info := sockets[socketKey]
	info.Connection = nil

	if err.Command == 8 {
		LogError(err.Status, err.Identifier)
		if err.Identifier < info.currentIndentity {
			messages := GetMessages(info, err.Identifier+1, info.currentIndentity)
			for i := 0; i < len(messages); i++ {
				msg := messages[i]
				if msg != nil {
					log.Println("add message to error buket")
					AddErrorMessage(messages[i])
				}
			}
		}

	}
}
