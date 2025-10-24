package core

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"github.com/gookit/goutil/netutil/httpreq"
	"github.com/gookit/goutil/strutil"
	"io"
	"log"
	"math"
	"net"
	"strings"
	"time"
)

// ServerStat 表示一个服务器的统计信息（简化版）
type ServerStat struct {
	PXName  string // Proxy name
	SVName  string // Server name
	Status  string // UP/DOWN
	Weight  int    // 当前权重
	ChkFail int    //检测失败次数，进行自动调整权重
	WRedis  int    //失败，切换次数
	WRetr   int    //失败，重试次数
	EResp   int    //返回失败，如果失败3次，可以当作代理失败
}

// getFieldIndex 从表头获取字段索引
func getFieldIndex(header []string, fieldName string) int {
	for i, h := range header {
		if strings.TrimSpace(h) == fieldName {
			return i
		}
	}
	return -1
}

// getField 从行中获取指定索引的值
func getField(row []string, idx int) string {
	if idx >= 0 && idx < len(row) {
		return strings.TrimSpace(row[idx])
	}
	return ""
}

func Reduce[T any, R any](items []T, init R, f func(R, T) R) R {
	acc := init
	for _, item := range items {
		acc = f(acc, item)
	}
	return acc
}

// sendHaproxyCommand 发送命令到 HAProxy TCP socket 并返回响应
func SendHaproxyCommand(command string) (string, error) {
	host := "192.168.5.2"
	port := "9999"
	// 建立 TCP 连接（超时 5s）
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
	if err != nil {
		return "", fmt.Errorf("连接失败: %v", err)
	}
	defer conn.Close() // 确保断开

	// 发送命令（以 \n 结束）
	fullCmd := command + "\n"
	_, err = conn.Write([]byte(fullCmd))
	if err != nil {
		return "", fmt.Errorf("发送命令失败: %v", err)
	}

	// 读取响应（用 bufio 读取一行）
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %v", err)
	}

	return strings.TrimSpace(response), nil // 去除 \n 等空白
}

func ResetCountersAll() {
	command := "clear counters all"
	result, err := SendHaproxyCommand(command)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(fmt.Sprintf("操作成功[%s]%s", command, result))
}

func GetStats() (map[string]ServerStat, error) {
	resp, err := httpreq.Get("http://192.168.5.2/stats;csv")
	if err != nil {
		return nil, err
	}
	csvBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	readerCSV := strings.NewReader(string(csvBytes))
	csvReader := csv.NewReader(readerCSV)
	csvReader.Comma = ','          // 默认逗号分隔
	csvReader.FieldsPerRecord = -1 // 允许变长记录

	records, err := csvReader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("无数据行")
	}

	// 第一行是表头，提取关键列索引
	header := records[0]
	pxNameIdx := getFieldIndex(header, "# pxname")
	svNameIdx := getFieldIndex(header, "svname")
	statusIdx := getFieldIndex(header, "status")
	weightIdx := getFieldIndex(header, "weight")
	chkFailIdx := getFieldIndex(header, "chkfail")
	wredisIdx := getFieldIndex(header, "wredis")
	wretrIdx := getFieldIndex(header, "wretr")
	erespIdx := getFieldIndex(header, "eresp")

	// 解析数据行
	stats := make(map[string][]ServerStat)
	for i := 1; i < len(records); i++ {
		row := records[i]
		if len(row) < 2 { // 跳过空行
			continue
		}
		pxName := getField(row, pxNameIdx)
		svName := getField(row, svNameIdx)
		if svName == "BACKEND" || svName == "FRONTEND" {
			continue //跳过前后面数据
		}
		stat := ServerStat{
			PXName:  pxName,
			SVName:  svName,
			Status:  getField(row, statusIdx),
			Weight:  strutil.IntOr(getField(row, weightIdx), 50),
			ChkFail: strutil.IntOr(getField(row, chkFailIdx), 0),
			WRedis:  strutil.IntOr(getField(row, wredisIdx), 0),
			WRetr:   strutil.IntOr(getField(row, wretrIdx), 0),
			EResp:   strutil.IntOr(getField(row, erespIdx), 0),
		}
		if _, ok := stats[pxName]; !ok {
			stats[pxName] = []ServerStat{}
		}
		stats[pxName] = append(stats[pxName], stat)
	}

	result := make(map[string]ServerStat)
	for _, stat := range stats {
		var rawWeights []float64
		chkCount := 0
		totalRaw := Reduce(stat, 0.0, func(acc float64, s ServerStat) float64 {
			w := s.WRedis + s.WRetr + s.EResp
			chkCount += w //统计错误次数
			if s.Status != "UP" || s.EResp > 3 {
				rawWeights = append(rawWeights, 0)
				return acc
			}
			raw := 1.0 / float64(w+1)
			rawWeights = append(rawWeights, raw)
			return acc + raw
		})

		for i := range stat {
			if chkCount <= 1 { //全部都正常
				stat[i].Weight = 100
			} else {
				if stat[i].Status == "UP" && stat[i].EResp <= 3 {
					stat[i].Weight = int(math.Round((rawWeights[i] / totalRaw) * 100)) //int(100 - float64(stat[i].ChkFail)/float64(total)*100)
				} else {
					stat[i].Weight = 0 //无效服务器，不需要任何权重
				}
			}

			result[stat[i].SVName] = stat[i]
		}
	}

	//log.Println("获得成功【从http://192.168.5.2/stats;csv获得数据】")
	return result, nil
}
