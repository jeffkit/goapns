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
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
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
	Connection *tls.Conn
	App        string
	Sandbox    bool
}

////////////////////// Global Variables ///////////////////////////

/// channels
var socketCN chan *ConnectInfo = make(chan *ConnectInfo)    // 到APNS的socket频道
var messageCN chan *Notification = make(chan *Notification) // 新推送消息的频道
var responseCN chan *APNSRespone = make(chan *APNSRespone)  // APNS服务端返回错误响应频道

// socket container
var sockets map[string]*tls.Conn = make(map[string]*tls.Conn)

var testConn *tls.Conn

// configs

var appsDir string // 推送应用的根目录
var appPort int    // web接口的端口

//////////// HTTP Method ////////////////

func pushHandler(w http.ResponseWriter, request *http.Request) {
	log.Print("handle push request")
	//go pushMessage(testConn)
	message := &Notification{
		Token: "7171d635e3e44dd34f4b18893de50c1476805f921e28d92e76a9e29a5281c576",
		Payload: &Payload{
			Aps: &AlertInfo{Alert: "你好，姐夫", Badge: 1, Sound: ""}},
		App: "com.toraysoft.music"}
	go notify(message)
	io.WriteString(w, "hello go apns!")
}

func connect(app string, keyFile string, certFile string, sandbox bool) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Printf("server : loadKeys: %s", err)
	}
	config := tls.Config{Certificates: []tls.Certificate{cert}, InsecureSkipVerify: true}
	endPoint := "gateway.push.apple.com:2195"
	if sandbox {
		endPoint = "gateway.sandbox.push.apple.com:2195"
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

	if err != nil {
		log.Printf("error when read from socket %s, %d", err, n)
	}
	fmt.Printf("return %x", reply)
	rsp := &APNSRespone{8, 8, 1024, conn, app, sandbox}
	responseCN <- rsp
}

func socketConnected(info *ConnectInfo) {
	app := info.App
	if info.Sandbox {
		app = app + "_dev"
	}
	if sockets[app] != nil {
		delete(sockets, app)
	}
	sockets[app] = info.Connection
	//sockets[app] = make([]*tls.Conn, 0, 100)
	// log.Printf("sockets %x and for app %x\n", sockets, sockets[app])
	// length := len(sockets[app])
	// log.Printf("length %d and cap %d\n", length, cap(sockets[app]))
	// if length < cap(sockets[app]) {
	// 	//sockets[app][length] = info.Connection
	// 	sockets[app] = append(sockets[app], info.Connection)
	// }

	go monitorConn(info.Connection, info.App, info.Sandbox)
}

/**
初始化socket连接，创建完后扔给channel
*/
func makeSocket() {
	// 创建几个socket？创建完后，由谁管理。
	walkErr := filepath.Walk(appsDir, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}

		if info.Name() != "develop" && info.Name() != "production" {
			return nil
		}

		app := strings.Replace(path.Dir(filePath), appsDir+"/", "", 1)
		log.Println("create socket for app :", app)
		sandbox := false
		if info.Name() == "develop" {
			sandbox = true
		}
		go connect(app, filePath+"/key.pem", filePath+"/cer.pem", sandbox)
		return nil
	})

	if walkErr != nil {
		log.Print("读取证书有问题哇", walkErr)
	}
}

/**
启动http服务，接受HTTP推送请求
*/
func startHttpServer() {
	log.Print("starting Http server")
	http.HandleFunc("/push", pushHandler)
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Printf("http server start fail %s \n", err)
	} else {
		log.Print("http server started!")
	}
}

/**
监听redis队列，主动获取推送消息
*/
func subscribeRedisQ() {
	log.Print("subscribing Redis Queue")
}

func notify(message *Notification) {
	// 根据app找到相应的socket。
	conn := sockets[message.App]
	if conn == nil {
		return
	}

	// 生成消息id
	var msgID int32
	msgID = 10010

	// 消息存入缓存，过期消失，如果失败会尝试重发。
	pushMessage(conn, message.Token, msgID, message.Payload)
}

func pushMessage(conn *tls.Conn, token string, identity int32, payload *Payload) {
	// info := AlertInfo{"你好，我是来自地球的男人，问一问你现在过得怎么样了，这条消息那么长，你看得了吗？看得完吗？在推送的界面里面。", 1, "default"}
	// payload := Payload{Aps: info}
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
func handleError(err *APNSRespone) {
	log.Print("Got an response from APNS Gateway")
	// 干掉这条socket

	// 重建socket
}

func initialize() {
	// 初始化一些变量
	appsDir = "/Users/jeff/Desktop/pushapps"
	appPort = 8080
}

func main() {
	log.Print("GO! APNS GO!")
	initialize()
	// 创建连接。
	go makeSocket()

	// 启动http服务。
	go startHttpServer()

	// 监听队列，视配置定
	go subscribeRedisQ()

	// 监听新应用或移除应用

	log.Print("Just wait for the channels")
	for {
		select {
		case info := <-socketCN:
			// 一条通向APNS的socket连接完成！
			log.Printf("socket for %s created!\n", info.App)
			socketConnected(info)
		case message := <-messageCN:
			// 收到一条要推送的消息！
			log.Printf("got new message %s\n", message)
			notify(message)
		case rsp := <-responseCN:
			// 收到一条来自APNS的错误通知
			log.Printf("got apns erro response for %s\n", rsp.App)
			handleError(rsp)
		}
	}
	log.Print("end of server!")
}
