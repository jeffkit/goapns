
## 安装依赖

安装redis驱动：
```
go get gopkg.in/redis.v2
```

安装levelDB：
```
wget xxxx/leveldb-1.15.0.tar.gz
tar -xvf leveldb-1.15.0.tar.gz
cd leveldb-1.15.0
make
```

安装snappy
```
udo apt-get install libsnappy-dev
```

安装levigo
```
CGO_CFLAGS="-I/home/jeff/leveldb-1.15.0/include -I/usr/include" CGO_LDFLAGS="-L/home/jeff/leveldb-1.15.0/lib -L/usr/lib -lsnappy" go get github.com/jmhodges/levigo
```

## 配置

编辑goapns的配置文件：/etc/goapns.conf

```
{
	"AppsDir": "/opt/config/apnsagent/apps",
	"AppPort": 8899, 
	"ConnectionIdleSecs": 600, 
	"DbPath": "/opt/data/goapns",
	"QueueWithRedis": true,
	"RedisPassword": "xxxxxxxx",
	"RedisDB": 3
}
```

AppsDir: 指存放推送证书的根目录。目录下存放每个应用的key及cert。目录结构约定如下：

- 每个应用以bundleid为目录名命名顶层目录。
- 每个应用均有develop及production两个子目录。分别存放sandbox及生产环境的证书及密钥。
- 证书及密钥必须命名为cer.pem, key.pem，并且无需输入passphare即可读取。

示例目录结构如下:

```
├── com.toraysoft.music
│   ├── develop
│   │   ├── cer.pem
│   │   └── key.pem
│   └── production
│       ├── cer.pem
│       └── key.pem
├── com.toraysoft.vddist
│   ├── develop
│   │   ├── cer.pem
│   │   └── key.pem
│   └── production
│       ├── cer.pem
│       └── key.pem
```

AppPort: Goapns暴露出来的Http端口。

ConnectionIdleSecs：APNS连接闲置最大时长，单位为秒。如果超过该时长，则会重连。

DbPath：本地LevelDB数据库存储的目录。

QueueWithRedis：如果不使用HTTP接口，可以通过Redis队列给Goapns提供喂消息。

## 运行Goapns

安装完Goapns后，直接在命令行中运行```goapns```即可启动服务。

## HTTP接口说明：
