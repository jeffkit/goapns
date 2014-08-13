package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	defer CapturePanic("Server will shutdonw with an runtime error!")
	configFile := flag.String("file",
		"/etc/goapns.conf",
		"location of config file")
	flag.Parse()

	log.Printf("config file path %s \n", *configFile)

	Initialize(configFile)

	go GenerateIdentity()
	// 创建连接。
	err := MakeSocket()
	if err != nil {
		log.Println(err)
		log.Fatalln("fail to create sockets, abort!")
	}

	// 启动http服务。
	go StartHttpServer()
	// 监听队列，视配置定
	go SubscribeRedisQ()

	go StartFeedbackService()

	// 监听新应用或移除应用

	log.Print("Just wait for the channels")
	signalCN := make(chan os.Signal, 1)
	signal.Notify(signalCN, syscall.SIGTERM, syscall.SIGINT,
		syscall.SIGHUP, syscall.SIGQUIT)
	for {
		select {
		case info := <-socketCN:
			// 一条通向APNS的socket连接完成！
			log.Printf("socket for %s created!\n", info.App)
			go SocketConnected(info)
		case message := <-messageCN:
			// 收到一条要推送的消息！
			//log.Printf("got new message %s\n", message)
			go Notify(message)
			if shutingDown {
				log.Println("new message come during shutdown time, reset counter")
				countDownTime = 1
			}
		case rsp := <-responseCN:
			// 收到一条来自APNS的错误通知
			log.Printf("got apns erro response for %s\n", rsp.App)
			go HandleError(rsp)
		case _ = <-signalCN:
			log.Println("got interupt or kill signal")
			shutingDown = true
			if countDownTime == 0 {
				log.Println("count down not start, start it")
				countDownTime = 1
				go countDown()
			}
		case _ = <-countDownCN:
			countDownTime += 1
		}

		if shutingDown && countDownTime >= SHUTDOWN_COUNTDOWN_TIME {
			log.Println("count down finish, no more new message, shutdown server")
			break
		}
	}
	log.Print("server shutdonw gracefully!!")
}

func countDown() {
	tick := time.NewTicker(1 * time.Second)
	for {
		select {
		case _ = <-tick.C:
			countDownCN <- 1
		}
	}
}
