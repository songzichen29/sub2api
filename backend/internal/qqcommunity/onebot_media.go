package qqcommunity

import (
	"fmt"
	"regexp"
	"strings"
)

// Names verified against NapCatQQ face_config.json at
// 98d781d19815d1625b24d717ce280ecf0a4f6d0c. Unknown IDs have no inferred meaning.
var qqFaceNames = map[string]string{
	"0": "惊讶", "1": "撇嘴", "2": "色", "3": "发呆", "4": "得意", "5": "流泪", "6": "害羞", "7": "闭嘴", "8": "睡", "9": "大哭",
	"10": "尴尬", "11": "发怒", "12": "调皮", "13": "呲牙", "14": "微笑", "15": "难过", "16": "酷", "18": "抓狂", "19": "吐",
	"20": "偷笑", "21": "可爱", "22": "白眼", "23": "傲慢", "24": "饥饿", "25": "困", "26": "惊恐", "27": "流汗", "28": "憨笑", "29": "悠闲", "30": "奋斗", "31": "咒骂",
	"66": "爱心", "74": "太阳", "76": "赞", "96": "冷汗", "97": "擦汗", "99": "鼓掌", "100": "糗大了", "101": "坏笑", "102": "左哼哼", "103": "右哼哼", "104": "哈欠", "105": "鄙视", "106": "委屈", "107": "快哭了", "109": "左亲亲", "110": "吓", "111": "可怜", "112": "菜刀", "114": "篮球", "116": "示爱", "118": "抱拳", "119": "勾引", "120": "拳头", "123": "NO", "124": "OK", "125": "转圈", "129": "挥手", "147": "棒棒糖", "171": "茶", "178": "斜眼笑", "182": "笑哭", "212": "托腮", "277": "汪汪",
}

var marketEmojiID = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)

func appendOneBotMedia(e *Event, seg obSegment) {
	switch seg.Type {
	case "image", "mface":
		u := obValue(seg.Data["url"])
		id := obValue(seg.Data["emoji_id"])
		if u == "" && marketEmojiID.MatchString(id) {
			// NapCat's marketFaceElement converter uses this exact public URL.
			u = fmt.Sprintf("https://gxh.vip.qq.com/club/item/parcel/item/%s/%s/raw300.gif", id[:2], id)
		}
		file := obValue(seg.Data["file"])
		if u == "" && strings.HasPrefix(file, "https://") {
			u = file
		}
		if u == "" {
			e.UnsupportedMedia = true
			return
		}
		if id != "" {
			file = id
		}
		e.Attachments = append(e.Attachments, Attachment{URL: u, FileID: limitText(file, 200), ContentType: "image/unknown"})
	case "face":
		id := obValue(seg.Data["id"])
		e.Faces = append(e.Faces, id)
		if qqFaceNames[id] == "" {
			e.UnsupportedMedia = true
		}
	default:
		// Do not accidentally turn mixed video, voice, forwards or cards into a
		// text-only interjection with missing context.
		e.UnsupportedMedia = true
	}
}
