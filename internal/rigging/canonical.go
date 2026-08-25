package rigging

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

type frozenManifest struct {
	PlanID           string            `json:"planId"`
	VenueName        string            `json:"venueName"`
	PerformanceDate  string            `json:"performanceDate"`
	RatedTotalLoadKg float64           `json:"ratedTotalLoadKg"`
	Points           []SuspensionPoint `json:"points"`
	Tests            []LoadTest        `json:"tests"`
}

func FrozenDigest(p *RigPlan) (string, error) {
	points := append([]SuspensionPoint(nil), p.Points...)
	tests := append([]LoadTest(nil), p.Tests...)
	sort.Slice(points, func(i, j int) bool { return points[i].ID < points[j].ID })
	sort.Slice(tests, func(i, j int) bool {
		if tests[i].PointID == tests[j].PointID {
			return tests[i].ID < tests[j].ID
		}
		return tests[i].PointID < tests[j].PointID
	})
	b, err := json.Marshal(frozenManifest{p.ID, p.VenueName, p.PerformanceDate, p.RatedTotalLoadKg, points, tests})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func FrozenProjectionDigest(p *RigPlan) (string, error) {
	copy := *p
	copy.Points = append([]SuspensionPoint(nil), p.FrozenPoints...)
	copy.Tests = append([]LoadTest(nil), p.FrozenTests...)
	return FrozenDigest(&copy)
}

func PointConfigDigest(point SuspensionPoint) string {
	value := struct {
		RatedLoadKg          float64 `json:"ratedLoadKg"`
		PlannedLoadKg        float64 `json:"plannedLoadKg"`
		DeviceModel          string  `json:"deviceModel"`
		CableSpec            string  `json:"cableSpec"`
		PrimaryPointID       string  `json:"primaryPointId"`
		RedundantPointID     string  `json:"redundantPointId"`
		CertificateExpiresOn string  `json:"certificateExpiresOn"`
	}{point.RatedLoadKg, point.PlannedLoadKg, clean(point.DeviceModel), clean(point.CableSpec), clean(point.PrimaryPointID), clean(point.RedundantPointID), point.CertificateExpiresOn}
	b, _ := json.Marshal(value)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
