package service

import "strings"

// Verdict policy is independent of the task/rubric and JSON transport versions.
// Saved native-v1 artifacts remain judgeable without regenerating them; finished
// results retain the policy version and verdict that were originally stored.
const qualityVerdictVersion = "verdict-v2"

type qualityVerdictCriterion struct {
	id    string
	label string
	core  bool
}

func qualityVerdictCriteria(benchmark string) []qualityVerdictCriterion {
	switch benchmark {
	case "candy":
		return []qualityVerdictCriterion{
			{"answer", "最终答案", true},
			{"sufficiency", "充分性证明", true},
			{"minimality", "最小性证明", true},
		}
	case "pelican":
		return []qualityVerdictCriterion{
			{"subject", "鹈鹕主体", true},
			{"bicycle", "自行车结构", true},
			{"riding_pose", "骑行动作", true},
			{"motion", "运动关系", true},
			{"completeness", "作品完整性", true},
		}
	case "creative":
		return []qualityVerdictCriterion{
			{"subject", "主题与主体", true},
			{"composition", "场景构图", false},
			{"motion", "连续运动", true},
			{"interaction", "暂停/继续功能", true},
			{"completeness", "作品完整性", true},
		}
	default:
		return nil
	}
}

// Evaluate validated checks, never the judge's free-form overall conclusion.
// A score of 1 is uncertainty, not a fractional pass that other scores can
// compensate for. Composition alone cannot establish a core capability defect.
func qualityAssessmentVerdict(benchmark string, assessment *ModelQualityAssessment) (string, string) {
	if assessment.Confidence < 0.8 {
		return "unknown", "裁判置信度不足（低于80%）"
	}
	scores := make(map[string]int, len(assessment.Checks))
	for _, check := range assessment.Checks {
		scores[check.ID] = check.Score
	}
	var failures, uncertain, nonCoreFailures []string
	for _, criterion := range qualityVerdictCriteria(benchmark) {
		score := scores[criterion.id]
		switch {
		case criterion.core && score == 0:
			failures = append(failures, criterion.label)
		case criterion.core && score == 1:
			uncertain = append(uncertain, criterion.label)
		case !criterion.core && score == 0:
			nonCoreFailures = append(nonCoreFailures, criterion.label)
		}
	}
	if len(failures) > 0 {
		return "degraded", "核心项存在明确缺陷：" + strings.Join(failures, "、")
	}
	if len(uncertain) > 0 {
		return "unknown", "核心项尚未充分验证：" + strings.Join(uncertain, "、")
	}
	if len(nonCoreFailures) > 0 {
		return "unknown", strings.Join(nonCoreFailures, "、") + "未满足要求，不能单独据此判定降智"
	}
	return "normal", "核心要求均有充分证据"
}
