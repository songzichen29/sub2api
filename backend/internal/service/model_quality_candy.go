package service

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	qualityCandyUnits       = regexp.MustCompile(`\\(?:text|mathrm)\{\s*(个|颗|糖果)\s*\}`)
	qualityCandyBox         = regexp.MustCompile(`\\boxed\{\s*([0-9零一二两三四五六七八九十百]+\s*(?:个|颗)?(?:糖果)?)\s*\}`)
	qualityCandyAnswer      = regexp.MustCompile(`(?i)^((?:(?:因此|所以|故|综上所述|综上|由此可知|总之)[，,:： ]*)?)((?:最终|最后)?(?:答案|结论)(?:应该是|应为|是|为)?|(?:所求(?:的)?)?(?:最少|最小|至少)(?:需要|要)?(?:取出|抽取|取|拿出)?(?:的)?(?:(?:总数|数量)(?:应该是|应为|是|为)?)?|the (?:final )?answer is|minimum is)([ ：:=]*)(?:需要|取出|抽取|取)?\s*([0-9零一二两三四五六七八九十百]*)(.*)$`)
	qualityCandyBare        = regexp.MustCompile(`^([0-9零一二两三四五六七八九十百]+)\s*(?:颗|个)?(?:糖果)?[。.!！]?$`)
	qualityCandyAlternative = regexp.MustCompile(`(?:^|[，,])\s*(?:或|还是|也可能)`)
	qualityCandyTail        = regexp.MustCompile(`^\s*(?:颗|个)?(?:糖果)?\s*(?:[.!！]?$|[，,。；;：:].*)$`)
)

// 只识别明确结论或独立答案，不把证明中出现的 21 当作答对。
func qualityCandyFinalAnswer(text string) (int, bool) {
	text = strings.Map(func(r rune) rune {
		if r >= '０' && r <= '９' {
			return r - '０' + '0'
		}
		return r
	}, text)
	text = strings.ReplaceAll(text, `\\`, `\`)
	text = qualityCandyUnits.ReplaceAllString(text, "$1")
	text = qualityCandyBox.ReplaceAllString(text, "$1")
	text = strings.NewReplacer("*", "", "_", "", "`", "", "$", "", `\(`, "", `\)`, "", `\[`, "", `\]`, "").Replace(text)
	answer, priority, found, ambiguous := 0, 0, false, false
	continuationRank := 0
	lines := strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '。' || r == '；' || r == ';' })
	for i, line := range lines {
		line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
		line = strings.TrimPrefix(line, "- ")
		if line == "" {
			continue
		}
		candidate, rank, declared, header := "", 1, false, false
		if match := qualityCandyAnswer.FindStringSubmatch(line); match != nil {
			prefix, number, tail := match[2], match[4], match[5]
			// 无数字时必须有明确声明边界，不能把“最少性证明”等标题当作答案。
			if number == "" && strings.TrimSpace(tail) != "" && match[3] == "" && !strings.HasSuffix(prefix, "是") && !strings.HasSuffix(prefix, "为") && !strings.HasPrefix(tail, "不是") {
				continue
			}
			declared = true
			if match[1] != "" || strings.Contains(prefix, "最终") || strings.Contains(prefix, "最后") || strings.Contains(prefix, "数量") || strings.Contains(prefix, "总数") || strings.Contains(strings.ToLower(prefix), "final") {
				rank = 2
			}
			header = number == "" && strings.TrimSpace(tail) == ""
			if qualityCandyTail.MatchString(tail) && !qualityCandyAlternative.MatchString(tail) {
				candidate = number
			}
		} else if match := qualityCandyBare.FindStringSubmatch(line); match != nil && (i == 0 || i == len(lines)-1 || continuationRank > 0) {
			candidate, declared = match[1], true
			if continuationRank > 0 {
				rank = continuationRank
			}
		} else if continuationRank > 0 {
			rank, declared = continuationRank, true
		}
		continuationRank = 0
		if !declared || rank < priority {
			continue
		}
		if rank > priority {
			found, ambiguous, priority = false, false, rank
		}
		if header {
			continuationRank = rank
			continue
		}
		value, ok := qualityCandyNumber(candidate)
		if !ok {
			// 显式结论无法解析时保留歧义，不能退回较早出现的正确数字。
			ambiguous = true
			continue
		}
		if found && value != answer {
			ambiguous = true
		}
		answer, found = value, true
	}
	return answer, found && !ambiguous && continuationRank == 0
}

func qualityCandyNumber(text string) (int, bool) {
	if value, err := strconv.Atoi(text); err == nil {
		return value, true
	}
	// 本题数量有限，只支持常见的两位中文整数，复杂表达保留为无法判定。
	digits := map[string]int{"零": 0, "一": 1, "二": 2, "三": 3, "四": 4, "五": 5, "六": 6, "七": 7, "八": 8, "九": 9}
	text = strings.ReplaceAll(text, "两", "二")
	if value, ok := digits[text]; ok {
		return value, true
	}
	parts := strings.Split(text, "十")
	if len(parts) != 2 {
		return 0, false
	}
	tens, units := 1, 0
	var ok bool
	if parts[0] != "" {
		tens, ok = digits[parts[0]]
		if !ok || tens == 0 {
			return 0, false
		}
	}
	if parts[1] != "" {
		units, ok = digits[parts[1]]
		if !ok || units == 0 {
			return 0, false
		}
	}
	return tens*10 + units, true
}

func assessQualityCandy(result *ModelQualityResult) (string, string) {
	answer, ok := qualityCandyFinalAnswer(result.Answer)
	verdict, score, confidence := "unknown", 1, 0.0
	reason := "本地答案核对：未识别到唯一明确的最终答案，无法判定；不使用模型裁判"
	if ok {
		confidence = 1
		verdict, score = "degraded", 0
		reason = "本地答案核对：最终答案为" + strconv.Itoa(answer) + "，正确答案为21"
		if answer == 21 {
			verdict, score = "normal", 2
			reason = "本地答案核对：最终答案21正确；不评价证明、不使用模型裁判"
		}
	}
	result.Source = qualityBenchmarkVersion + ":answer_exact"
	result.Assessment = &ModelQualityAssessment{Version: qualityBenchmarkVersion + ":answer-exact-v1", Basis: "answer_exact", Confidence: confidence, Checks: []ModelQualityCheck{{ID: "answer", Score: score, Evidence: reason}}}
	return verdict, reason
}
