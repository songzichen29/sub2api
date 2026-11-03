package qqcommunity

import "strings"

// No external retrieval tool is configured. Handle explicit live X lookups
// without a paid retry or a fictional promise to search in the background.
func isLiveXLookup(text string) bool {
	text = strings.ToLower(text)
	contains := func(words ...string) bool {
		for _, word := range words {
			if strings.Contains(text, word) {
				return true
			}
		}
		return false
	}
	return contains("推特", "twitter", "x.com", "推文") && contains("最新", "实时", "最近一条", "最近一个", "刚刚") && contains("查", "搜", "检索", "看下", "看一下", "是什么", "哪条", "发了什么")
}
