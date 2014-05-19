package main

import (
	"log"
)

func main() {
	log.Print("GO! APNS GO!")
	Initialize()

	go GenerateIndentity()
	// 创建连接。
	go MakeSocket()

	// 启动http服务。
	go StartHttpServer()

	// 监听队列，视配置定
	go SubscribeRedisQ()

	// 监听新应用或移除应用

	log.Print("Just wait for the channels")
	for {
		select {
		case info := <-socketCN:
			// 一条通向APNS的socket连接完成！
			log.Printf("socket for %s created!\n", info.App)
			SocketConnected(info)
		case message := <-messageCN:
			// 收到一条要推送的消息！
			log.Printf("got new message %s\n", message)
			Notify(message)
		case rsp := <-responseCN:
			// 收到一条来自APNS的错误通知
			log.Printf("got apns erro response for %s\n", rsp.App)
			HandleError(rsp)
		}
	}
	log.Print("end of server!")
}
