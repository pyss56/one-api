package model

import (
	"strconv"
	"strings"
)

// ParseChannelIds 将形如 "1,2,3" 的渠道ID白名单字符串解析为 int 切片。
// 入参为 nil 或空串时返回空切片（表示不限制）。
func ParseChannelIds(s *string) []int {
	ids := make([]int, 0)
	if s == nil || *s == "" {
		return ids
	}
	for _, p := range strings.Split(*s, ",") {
		if id, err := strconv.Atoi(strings.TrimSpace(p)); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}
