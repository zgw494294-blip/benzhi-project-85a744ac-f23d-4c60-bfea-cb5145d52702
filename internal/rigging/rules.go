package rigging

import (
	"fmt"
	"sort"
	"time"
)

type ruleFinding struct {
	PointID string
	Code    string
	Text    string
}

func evaluateRules(p *RigPlan) []ruleFinding {
	var findings []ruleFinding
	var total float64
	for _, point := range p.Points {
		total += point.PlannedLoadKg
		if point.PlannedLoadKg > point.RatedLoadKg {
			findings = append(findings, ruleFinding{point.ID, "POINT_OVERLOAD", "计划载荷超过吊点额定载荷"})
		}
		if point.PrimaryPointID == "" && (point.RedundantPointID == "" || point.RedundantPointID == point.ID) {
			findings = append(findings, ruleFinding{point.ID, "REDUNDANCY_MISSING", "未登记有效的独立安全冗余吊点"})
		}
		if certificateExpired(point.CertificateExpiresOn, p.PerformanceDate) {
			findings = append(findings, ruleFinding{point.ID, "CERTIFICATE_EXPIRED", "设备或钢索证书在演出日期前失效"})
		}
		latest, ok := latestTest(p.Tests, point.ID, point.ConfigDigest)
		if !ok {
			findings = append(findings, ruleFinding{point.ID, "TEST_MISSING", "尚无载荷试验记录"})
		} else if latest.Outcome != OutcomePass || latest.MeasuredLoadKg < latest.TargetLoadKg || latest.HoldSeconds < 60 || latest.DeformationMm > 2 {
			findings = append(findings, ruleFinding{point.ID, "TEST_FAILED", "最近试验未通过载荷、保持时长或变形限制"})
		}
	}
	if total > p.RatedTotalLoadKg {
		findings = append(findings, ruleFinding{"", "PLAN_OVERLOAD", fmt.Sprintf("计划总载荷 %.2f kg 超过方案额定总载荷", total)})
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].PointID == findings[j].PointID {
			return findings[i].Code < findings[j].Code
		}
		return findings[i].PointID < findings[j].PointID
	})
	return findings
}

func latestTest(tests []LoadTest, pointID, configDigest string) (LoadTest, bool) {
	var result LoadTest
	ok := false
	for _, t := range tests {
		if t.PointID == pointID && t.PointConfigDigest == configDigest && (!ok || t.PerformedAt.After(result.PerformedAt)) {
			result, ok = t, true
		}
	}
	return result, ok
}

func refreshIssues(p *RigPlan, now time.Time, allowClose bool) {
	findings := evaluateRules(p)
	active := make(map[string]ruleFinding, len(findings))
	for _, f := range findings {
		active[f.PointID+"|"+f.Code] = f
	}
	known := make(map[string]bool)
	for i := range p.Issues {
		issue := &p.Issues[i]
		key := issue.PointID + "|" + issue.RuleCode
		known[key] = true
		finding, still := active[key]
		if still {
			issue.Description = finding.Text
			if issue.Status == IssueClosed {
				issue.Status = IssueOpen
				issue.ClosedAt = nil
				issue.OpenedAt = now
			}
			continue
		}
		if issue.RuleCode == "TEST_MISSING" && issue.Status != IssueRetestPending {
			issue.Status = IssueClosed
			closed := now
			issue.ClosedAt = &closed
			continue
		}
		retestPointID := issue.PointID
		if retestPointID == "" {
			for j := len(p.Revisions) - 1; j >= 0; j-- {
				if p.Revisions[j].IssueID == issue.ID {
					retestPointID = p.Revisions[j].PointID
					break
				}
			}
		}
		if allowClose && issue.Status == IssueRetestPending && hasPassingRetest(p.Tests, retestPointID, issue.OpenedAt) {
			issue.Status = IssueClosed
			closed := now
			issue.ClosedAt = &closed
		}
	}
	for key, f := range active {
		if known[key] {
			continue
		}
		p.Issues = append(p.Issues, SafetyIssue{
			ID: deterministicID("issue", p.ID, f.PointID, f.Code), PlanID: p.ID, PointID: f.PointID, RuleCode: f.Code,
			Severity: "blocking", Description: f.Text, Status: IssueOpen, OpenedAt: now,
		})
	}
	sort.Slice(p.Issues, func(i, j int) bool { return p.Issues[i].ID < p.Issues[j].ID })
}

func hasPassingRetest(tests []LoadTest, pointID string, after time.Time) bool {
	for _, t := range tests {
		if t.PointID == pointID && t.TestKind == TestRetest && t.PerformedAt.After(after) &&
			t.Outcome == OutcomePass && t.MeasuredLoadKg >= t.TargetLoadKg && t.HoldSeconds >= 60 && t.DeformationMm <= 2 {
			return true
		}
	}
	return false
}

func openIssues(p *RigPlan) []SafetyIssue {
	var issues []SafetyIssue
	for _, issue := range p.Issues {
		if issue.Status != IssueClosed {
			issues = append(issues, issue)
		}
	}
	return issues
}
