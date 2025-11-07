package main

import (
	"fmt"
	"haproxy/core"
	"log"
	"net"
	"strings"
	"time"
)

var currentTime time.Time
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
	cmd := ""
	if stat, ok := haStats[svname]; ok {
		threshold = stat.Weight
		if stat.Status == "DOWN" && int(stat.Downtime/60) > 10 { //超过了10分钟，即服务器可能已经变化
			now := time.Now()

			// 构造当天的 00:00 和 00:10 时间
			start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			end := time.Date(now.Year(), now.Month(), now.Day(), 0, 10, 0, 0, now.Location())

			if now.After(start) && now.Before(end) {
				cmd = "ready" //每天的这个时间段开放，服务器可能已经开启
				fmt.Println("当前时间在 00:00 到 00:10 之间")
			} else {
				cmd = "maint"
				fmt.Println("当前时间不在 00:00 到 00:10 之间")
			}
		}
	}

	if cmd == "" {
		cmd = fmt.Sprintf("%d%%", threshold)
	}
	conn.Write([]byte(cmd + "\n"))
	log.Println(fmt.Sprintf("svname:%s-->%s", svname, cmd))
}

func main() {
	resetTicker := time.NewTicker(5 * time.Hour)
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	defer resetTicker.Stop()

	haStats = make(map[string]core.ServerStat)
	// 每隔一段时间来进行计算权重，后续使用agent-check
	go func() {
		var err error //第一次的时候必须立即执行1次
		haStats, err = core.GetStats()
		for {
			<-ticker.C
			if haStats == nil || len(haStats) <= 0 || currentTime.IsZero() || time.Now().After(currentTime.Add(15*time.Minute)) {
				haStats, err = core.GetStats()
				if !currentTime.IsZero() {
					currentTime = time.Time{}
				}
				println("---------------")
				if err != nil {
					log.Println(err)
				}
			}
		}
	}()

	//每隔一段时间来进行重置权重，网络情况是一直变化的，保持变化
	go func() {
		for {
			<-resetTicker.C
			core.ResetCountersAll()
			//重置的时候记录一下时间，在重置后15分钟以内不进行收集数据，分配权限
			currentTime = time.Now()
		}
	}()

	log.Println("HAPROXY WEIGHT 服务端口：9099")
	agentServer("9099")
}
