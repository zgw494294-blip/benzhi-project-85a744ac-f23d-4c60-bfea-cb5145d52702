package rigging

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
)

func newID(prefix string) string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	return prefix + "_" + hex.EncodeToString(b)
}

func deterministicID(prefix string, parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write([]byte(part))
	}
	return prefix + "_" + hex.EncodeToString(hash.Sum(nil)[:12])
}

func clean(s string) string { return strings.TrimSpace(s) }

func clonePlan(p *RigPlan) *RigPlan {
	if p == nil {
		return nil
	}
	c := *p
	c.Points = append([]SuspensionPoint(nil), p.Points...)
	c.Tests = append([]LoadTest(nil), p.Tests...)
	c.Issues = append([]SafetyIssue(nil), p.Issues...)
	c.Revisions = append([]RemediationRevision(nil), p.Revisions...)
	c.PlanRevisions = append([]PlanRevision(nil), p.PlanRevisions...)
	c.RetestTasks = append([]RetestTask(nil), p.RetestTasks...)
	c.Reviews = append([]ReviewDecision(nil), p.Reviews...)
	c.Credentials = append([]ClearanceCredential(nil), p.Credentials...)
	c.FrozenPoints = append([]SuspensionPoint(nil), p.FrozenPoints...)
	c.FrozenTests = append([]LoadTest(nil), p.FrozenTests...)
	pointDigests := make(map[string]string, len(c.Points))
	for _, point := range c.Points {
		pointDigests[point.ID] = point.ConfigDigest
	}
	for i := range c.Tests {
		c.Tests[i].CurrentConfiguration = c.Tests[i].PointConfigDigest != "" && c.Tests[i].PointConfigDigest == pointDigests[c.Tests[i].PointID]
	}
	return &c
}

func requestDigest(value any) string {
	b, _ := json.Marshal(value)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func parseDate(value string) (time.Time, error) { return time.Parse("2006-01-02", value) }

func findPoint(p *RigPlan, id string) (*SuspensionPoint, bool) {
	for i := range p.Points {
		if p.Points[i].ID == id {
			return &p.Points[i], true
		}
	}
	return nil, false
}

func findIssue(p *RigPlan, id string) (*SafetyIssue, bool) {
	for i := range p.Issues {
		if p.Issues[i].ID == id {
			return &p.Issues[i], true
		}
	}
	return nil, false
}

func refreshTestConfiguration(p *RigPlan) {
	digests := make(map[string]string, len(p.Points))
	for _, point := range p.Points {
		digests[point.ID] = point.ConfigDigest
	}
	for i := range p.Tests {
		p.Tests[i].CurrentConfiguration = p.Tests[i].PointConfigDigest != "" && p.Tests[i].PointConfigDigest == digests[p.Tests[i].PointID]
	}
}
