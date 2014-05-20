package main

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

func Initialize() {
	// 初始化一些变量
	appsDir = "/Users/jeff/Desktop/pushapps"
	appPort = 8080
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
		log.Print("连接服务器有误")
		return
	}
	log.Print("client is connect to ", conn.RemoteAddr())
	state := conn.ConnectionState()

	log.Print("client: hand shake ", state.HandshakeComplete)
	log.Print("client: mutual", state.NegotiatedProtocolIsMutual)
	info := &ConnectInfo{conn, app, sandbox}
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
		log.Printf("error when read from socket %s, %d", err, n)
	}
	log.Printf("return %x", reply)
	buf := bytes.NewBuffer(reply[2:])
	id, _ := binary.ReadUvarint(buf)
	rsp := &APNSRespone{reply[0], reply[1], int32(id), conn, app, sandbox}
	responseCN <- rsp
}

func SocketConnected(info *ConnectInfo) {
	app := info.App
	if info.Sandbox {
		app = path.Join(app, DEVELOP_SUBFIX)
	}
	if sockets[app] == nil {
		sockets[app] = info
	} else {
		sockets[app].Connection = info.Connection
	}

	go monitorConn(info.Connection, info.App, info.Sandbox)
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
		return
	}
	// 生成消息id

	msgID := GetIndentity()

	// 消息存入缓存，过期消失，如果失败会尝试重发。
	go pushMessage(conn, message.Token, msgID, message.Payload)
	info.currentIndentity = msgID
}

func pushMessage(conn *tls.Conn, token string, identity int32, payload *Payload) {
	fmt.Print(payload)
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("json marshal error %s", err)
	}

	fmt.Printf("payload %s\n", string(payloadBytes))
	os.Stdout.Write(payloadBytes)

	buf := new(bytes.Buffer)
	var command byte = 1
	err = binary.Write(buf, binary.BigEndian, command)
	if err != nil {
		log.Printf("fail to write command to buffer %s", err)
	}

	err = binary.Write(buf, binary.BigEndian, identity)
	if err != nil {
		log.Printf("fail to write identity to buffer %s", err)
	}

	var expires int32 = int32(time.Now().AddDate(0, 0, 1).Unix())
	err = binary.Write(buf, binary.BigEndian, expires)
	if err != nil {
		log.Printf("fail to write expires to buffer %s", err)
	}

	var tokenLength int16 = 32
	err = binary.Write(buf, binary.BigEndian, tokenLength)
	if err != nil {
		log.Printf("fail to write tokensize to buffer %s", err)
	}

	tokenBytes, err := hex.DecodeString(token)
	err = binary.Write(buf, binary.BigEndian, tokenBytes)
	if err != nil {
		log.Printf("fail to write token to buffer %s", err)
	}

	var payloadLength int16 = int16(len(payloadBytes))
	fmt.Printf("payloadLength %d\n", len(payloadBytes))
	err = binary.Write(buf, binary.BigEndian, payloadLength)
	if err != nil {
		log.Printf("fail to write payoadLength to buffer %s", err)
	}
	err = binary.Write(buf, binary.BigEndian, payloadBytes)
	if err != nil {
		log.Printf("fail to write payloadBytes to buffer %s", err)
	}

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
		socketKey = err.App + DEVELOP_SUBFIX
		dir = path.Join(dir, DEVELOP_FOLDER)
	} else {
		dir = path.Join(dir, PRODUCTION_FOLDER)
	}

	sockets[socketKey].Connection = nil

	// TODO:重建socket, 重建完成后需要重发该错误ID之后的消息。
	if err.Command == 8 {
		LogError(err.Status, err.Identifier)
	}
	go connect(err.App,
		path.Join(dir, KEY_FILE_NAME),
		path.Join(dir, CERT_FILE_NAME),
		err.Sandbox)
}
