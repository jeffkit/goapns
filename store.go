package main

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"github.com/jmhodges/levigo"
	"log"
)

var db *levigo.DB

func getDB() *levigo.DB {
	if db == nil {
		opts := levigo.NewOptions()
		opts.SetCache(levigo.NewLRUCache(3 << 30))
		opts.SetCreateIfMissing(true)
		_db, err := levigo.Open(dbPath, opts)
		if err != nil {
			log.Fatalln("can not open database, ", err)
		}
		db = _db
	}
	return db
}

func StoreMessage(notification *Notification, msgID int32, connectNum int32) {
	// 序列化该消息到数据库。消息的key为：app_msgID.
	wo := levigo.NewWriteOptions()
	defer wo.Close()
	key := fmt.Sprintf("%s_%d_%d", notification.App, connectNum, msgID)
	log.Println("store message to database ", key)
	var body bytes.Buffer
	enc := gob.NewEncoder(&body)
	enc.Encode(notification)
	err := getDB().Put(wo, []byte(key), body.Bytes())
	if err != nil {
		log.Println("can not store message to database")
	}
}

func GetMessage(ro *levigo.ReadOptions, info *ConnectInfo, identifier int32) *Notification {

	key := fmt.Sprintf("%s_%d_%d", info.App, info.number, identifier)
	data, err := getDB().Get(ro, []byte(key))
	if err != nil {
		log.Println("can not get data from leveldb", err)
	}
	body := bytes.NewBuffer(data)
	var notification Notification
	dec := gob.NewDecoder(body)
	err = dec.Decode(&notification)
	if err != nil {
		log.Println("can not decode notification from archive", err)
		return nil
	}
	return &notification
}

func GetMessages(info *ConnectInfo, fromID int32, toID int32) []*Notification {
	log.Printf("get message from %d to %d", fromID, toID)
	ro := levigo.NewReadOptions()
	defer ro.Close()
	result := make([]*Notification, toID-fromID+1, toID-fromID+1)
	log.Println("message result length ", len(result))
	for i := fromID; i < toID+1; i++ {
		log.Println("get message with id", i)
		notification := GetMessage(ro, info, i)
		if notification != nil {
			result[i-fromID] = notification
		}
	}
	return result
}

func getLatestIdentity() int32 {
	ro := levigo.NewReadOptions()
	defer ro.Close()
	data, err := getDB().Get(ro, []byte("latest_indentity"))
	if err != nil {
		log.Println("cannot get latest_indentity")
		return 0
	}
	buf := bytes.NewBuffer(data)
	var result int32
	err = binary.Read(buf, binary.BigEndian, &result)
	if err != nil {
		log.Println("invalid indentity", err)
		return 0
	}
	return result
}

func storeLatestIdentity(msgID int32) {
	wo := levigo.NewWriteOptions()
	defer wo.Close()
	buf := bytes.NewBuffer([]byte{})
	binary.Write(buf, binary.BigEndian, msgID)
	err := getDB().Put(wo, []byte("latest_indentity"), buf.Bytes())
	if err != nil {
		log.Println("error when store latestIdentity", err)
	}
}
