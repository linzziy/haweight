package core

import (
	"encoding/csv"
	"fmt"
	"github.com/gookit/goutil/netutil/httpreq"
	"github.com/gookit/goutil/strutil"
	"io"
	"strings"
)

// ServerStat 表示一个服务器的统计信息（简化版）
type ServerStat struct {
	PXName  string // Proxy name
	SVName  string // Server name
	Status  string // UP/DOWN
	Weight  int    // 当前权重
	ChkFail int    //检测失败次数，进行自动调整权重
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
		}
		if _, ok := stats[pxName]; !ok {
			stats[pxName] = []ServerStat{}
		}
		stats[pxName] = append(stats[pxName], stat)
	}

	result := make(map[string]ServerStat)
	for _, stat := range stats {
		total := Reduce(stat, 0.0, func(acc int, s ServerStat) int {
			return acc + s.ChkFail
		})

		for i := range stat {
			if total <= 1 { //全部都正常
				stat[i].Weight = 100
			} else {
				stat[i].Weight = int(100 - float64(stat[i].ChkFail)/float64(total)*100)
			}

			result[stat[i].SVName] = stat[i]
		}
	}

	//log.Println("获得成功【从http://192.168.5.2/stats;csv获得数据】")
	return result, nil
}
