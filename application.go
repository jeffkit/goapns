package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func test_payload() {
	/**
	所有接口都使用一致的Json格式接口：参数payload的格式与苹果官方指定的payload格式一致。另有参数：
	- token: 接收推送的设备ID，可以是一个或多个，这些token必须是统一的sanbox或非sandbox版。
	- sandbox: 是否沙盒，代表token是否sandbox的。
	- app：消息属于哪个应用。
	*/
	str := `
	{
		"payload": {
		    "aps" : {
		        "alert" : {
		            "loc-key" : "GAME_PLAY_REQUEST_FORMAT",
		            "loc-args" : ["Jenna", "Frank"]
		        },
		        "badge" : 9,
		        "sound" : "bingbong.aiff"
		    },
		    "acme1" : "bar",
			"acme2" : 42
		},
		"token": "12344",
		"sandbox": true,
		"app": "com.toraysoft.music"
	}`
	var dict map[string]interface{} = make(map[string]interface{})
	json.Unmarshal([]byte(str), &dict)
	pl, _ := MakePayloadFromMap(dict["payload"].(map[string]interface{}))
	log.Println(pl.Custom)
	log.Println(pl.Aps.Alert)
}

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

	go StartFeedbackService()

	// 监听新应用或移除应用

	log.Print("Just wait for the channels")
	signalCN := make(chan os.Signal, 1)
	signal.Notify(signalCN, syscall.SIGTERM, syscall.SIGINT,
		syscall.SIGHUP, syscall.SIGQUIT)
	for {
		select {
		case info := <-socketCN: // 一条通向APNS的socket连接完成！
			log.Printf("socket for %s created!\n", info.App)
			go SocketConnected(info)
		case message := <-messageCN: // 收到一条要推送的消息！
			go Notify(message)
			if shutingDown {
				log.Println("new message come during shutdown time, reset counter")
				countDownTime = 1
			}
		case rsp := <-responseCN: // 收到一条来自APNS的错误通知
			log.Printf("got apns erro response for %s\n", rsp.App)
			go HandleError(rsp)
		case _ = <-signalCN: // 收到系统信号，要关闭服务器
			log.Println("got interupt or kill signal")
			shutingDown = true
			if countDownTime == 0 {
				log.Println("count down not start, start it")
				countDownTime = 1
				go countDown()
			}
		case _ = <-countDownCN: // 收到倒数
			countDownTime += 1
		}

		if shutingDown && countDownTime >= SHUTDOWN_COUNTDOWN_TIME {
			log.Println("count down finish, no more new message, shutdown server")
			// 关闭sockets
			for _, info := range sockets {
				info.Connection.Close()
			}
			break
		}
	}
	log.Print("Bye！server shutdonw gracefully!!")
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
