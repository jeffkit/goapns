package main

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

func Initialize(path *string) {
	file, err := os.Open(*path)
	if err != nil {
		log.Fatalf("config file %d not found\n", *path)
	}
	content := make([]byte, 1024)

	n, err := file.Read(content)
	if err != nil && err != io.EOF {
		log.Fatalln("error occur when reading config file!", err)
	}

	config := make(map[string]interface{})
	err = json.Unmarshal(content[:n], &config)
	if err != nil {
		log.Fatalln("wrong json format: ", err)
	}
	appsDir = config["appsDir"].(string)
	appPort = int(config["appPort"].(float64))
	dbPath = config["dbPath"].(string)
	connectionIdleSecs = int64(config["connectionIdleSecs"].(float64))
}

func connect(app string, keyFile string, certFile string, sandbox bool) {
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
	app := info.App
	if sockets[app] == nil {
		sockets[app] = info
	} else {
		sockets[app].Connection = info.Connection
		sockets[app].lastActivity = info.lastActivity
	}
	go monitorConn(info.Connection, info.App, info.Sandbox)

	// 看看有没有消息需要重新发。
	if HasPendingMessage(info) {
		bucket := ErrorBucketForApp(info.App)
		for {
			notification := bucket.Next()
			if notification == nil {
				break
			}
			go Notify(notification)
		}
	}
}

/**
初始化socket连接，创建完后扔给channel
*/
func MakeSocket() {
	// 创建几个socket？创建完后，由谁管理。
	walkErr := filepath.Walk(appsDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}

		if info.Name() != DEVELOP_FOLDER && info.Name() != PRODUCTION_FOLDER {
			return nil
		}

		buff := bytes.NewBufferString(appsDir)
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
	}
}

/**
监听redis队列，主动获取推送消息
*/
func SubscribeRedisQ() {
	log.Print("subscribing Redis Queue")
}

func Notify(message *Notification) {
	// 根据app找到相应的socket。
	info := sockets[message.App]
	conn := info.Connection
	if conn == nil {
		// 扔进等待队列。
		AddErrorMessage(message)
		return
	}

	if time.Now().Unix()-info.lastActivity > connectionIdleSecs {
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
	log.Println("push!")
	go pushMessage(conn, message.Token, msgID, message.Payload)

	info.currentIndentity = msgID
	info.lastActivity = time.Now().Unix()
	log.Println("finish push")
}

func pushMessage(conn *tls.Conn, token string, identity int32, payload *Payload) {
	if len(token) == 0 {
		log.Println("missing token")
		return
	}

	if payload == nil || payload.IsEmpty() {
		log.Println("not a valid payload")
		return
	}

	buf := new(bytes.Buffer)

	// command
	var command byte = 1
	err := binary.Write(buf, binary.BigEndian, command)
	if err != nil {
		log.Printf("fail to write command to buffer %s", err)
	}

	// identifier
	err = binary.Write(buf, binary.BigEndian, identity)
	if err != nil {
		log.Printf("fail to write identity to buffer %s", err)
	}

	// expires
	var expires int32 = int32(time.Now().AddDate(0, 0, 1).Unix())
	err = binary.Write(buf, binary.BigEndian, expires)
	if err != nil {
		log.Printf("fail to write expires to buffer %s", err)
	}

	// token length
	var tokenLength int16 = 32
	err = binary.Write(buf, binary.BigEndian, tokenLength)
	if err != nil {
		log.Printf("fail to write tokensize to buffer %s", err)
	}

	// token content
	tokenBytes, err := hex.DecodeString(token)
	if len(tokenBytes) != int(tokenLength) {
		log.Println("invalid token! ")
		return
	}

	err = binary.Write(buf, binary.BigEndian, tokenBytes)
	if err != nil {
		log.Printf("fail to write token to buffer %s", err)
	}

	// payload length

	payloadBytes, err := payload.Json()
	if err != nil {
		log.Printf("json marshal error %s", err)
	}
	if len(payloadBytes) > 256 {
		// 压缩payload的长度。

	}

	fmt.Printf("payload %s\n", string(payloadBytes))
	var payloadLength int16 = int16(len(payloadBytes))
	err = binary.Write(buf, binary.BigEndian, payloadLength)
	if err != nil {
		log.Printf("fail to write payoadLength to buffer %s", err)
	}

	// payload content
	err = binary.Write(buf, binary.BigEndian, payloadBytes)
	if err != nil {
		log.Printf("fail to write payloadBytes to buffer %s", err)
	}

	// write to socket
	size, err := conn.Write(buf.Bytes())
	fmt.Printf("body: %x\n", buf.Bytes())
	log.Printf("write body size %d", size)
	if err != nil {
		log.Printf("error when write to socket %s, %d", err, size)
	}

}

/**
处理APNS服务器返回的错误：
- 记录发送失败的原因
- 重发indentifier之后的消息。
*/
func HandleError(err *APNSRespone) {
	log.Print("Got an response from APNS Gateway")

	// 干掉这条socket
	socketKey := err.App
	dir := path.Join(appsDir, err.App)
	if err.Sandbox {
		dir = path.Join(strings.Replace(dir, DEVELOP_SUBFIX, "", 1), DEVELOP_FOLDER)
	} else {
		dir = path.Join(dir, PRODUCTION_FOLDER)
	}

	info := sockets[socketKey]
	info.Connection = nil

	if err.Command == 8 {
		LogError(err.Status, err.Identifier)
		messages := GetMessages(info, err.Identifier+1, info.currentIndentity)
		for i := 0; i < len(messages); i++ {
			msg := messages[i]
			if msg != nil {
				AddErrorMessage(messages[i])
			}
		}
	}

	go connect(err.App,
		path.Join(dir, KEY_FILE_NAME),
		path.Join(dir, CERT_FILE_NAME),
		err.Sandbox)
}
