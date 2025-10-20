package main

import (
	"fmt"
	"haproxy/core"
	"log"
	"net"
	"strings"
	"time"
)

var haStats map[string]core.ServerStat

func agentServer(port string) {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		panic(err)
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handle(conn)
	}
}

func handle(conn net.Conn) {
	defer conn.Close()

	// 读取 HAProxy 发送的 agent-send 字符串
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		log.Println("read error:", err)
		conn.Write([]byte("100%\n"))
		return
	}
	svname := strings.TrimSpace(string(buf[:n])) // 去除 \n 等空白
	//fmt.Printf("收到 agent-send: %s, 解析 svname: %s\n", svname, svname) // 日志，可选

	//没有任何数据，可能程序未配置成功
	if haStats == nil || len(haStats) == 0 {
		log.Println("未从192.168.5.2中拉取数据：" + svname)
		return
	}

	threshold := 100
	if stat, ok := haStats[svname]; ok {
		threshold = stat.Weight
	}

	result := fmt.Sprintf("%d%%\n", threshold)
	conn.Write([]byte(result))
	//log.Println(fmt.Sprintf("svname:%s-->%s", svname, result))
}

func main() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	haStats = make(map[string]core.ServerStat)
	// 每隔一段时间来进行计算权重，后续使用agent-check
	go func() {
		var err error //第一次的时候必须立即执行1次
		haStats, err = core.GetStats()
		for {
			<-ticker.C
			haStats, err = core.GetStats()
			if err != nil {
				log.Println(err)
			}
		}
	}()

	log.Println("HAPROXY WEIGHT 服务端口：9099")
	agentServer("9099")
}
