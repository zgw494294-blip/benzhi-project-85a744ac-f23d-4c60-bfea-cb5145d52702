package rigging

import (
	"fmt"
	"time"
)

func validateCreate(r CreatePlanRequest) error {
	var fields []FieldError
	if clean(r.VenueName) == "" {
		fields = append(fields, FieldError{"venueName", "场地不能为空"})
	}
	if _, err := parseDate(r.PerformanceDate); err != nil {
		fields = append(fields, FieldError{"performanceDate", "演出日期必须为 YYYY-MM-DD"})
	}
	if r.RatedTotalLoadKg <= 0 {
		fields = append(fields, FieldError{"ratedTotalLoadKg", "额定总载荷必须大于 0 kg"})
	}
	if clean(r.OwnerName) == "" {
		fields = append(fields, FieldError{"ownerName", "负责人不能为空"})
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

func validatePoint(p *RigPlan, r AddPointRequest) error {
	return validatePointUpdate(p, "", r)
}

func validatePointUpdate(p *RigPlan, pointID string, r AddPointRequest) error {
	var fields []FieldError
	if clean(r.Label) == "" {
		fields = append(fields, FieldError{"label", "吊点标签不能为空"})
	}
	if r.RatedLoadKg <= 0 {
		fields = append(fields, FieldError{"ratedLoadKg", "额定载荷必须大于 0 kg"})
	}
	if r.PlannedLoadKg <= 0 {
		fields = append(fields, FieldError{"plannedLoadKg", "计划载荷必须大于 0 kg"})
	}
	if clean(r.DeviceModel) == "" {
		fields = append(fields, FieldError{"deviceModel", "设备型号不能为空"})
	}
	if clean(r.CableSpec) == "" {
		fields = append(fields, FieldError{"cableSpec", "钢索规格不能为空"})
	}
	if _, err := parseDate(r.CertificateExpiresOn); err != nil {
		fields = append(fields, FieldError{"certificateExpiresOn", "证书日期必须为 YYYY-MM-DD"})
	}
	for _, point := range p.Points {
		if point.ID != pointID && point.Label == clean(r.Label) {
			fields = append(fields, FieldError{"label", "吊点标签已存在"})
		}
	}
	if r.RedundantPointID != "" {
		if r.RedundantPointID == pointID {
			fields = append(fields, FieldError{"redundantPointId", "冗余吊点不能指向自身"})
		}
		if _, ok := findPoint(p, r.RedundantPointID); !ok {
			fields = append(fields, FieldError{"redundantPointId", "冗余吊点不存在"})
		}
	}
	if r.PrimaryPointID != "" {
		if r.PrimaryPointID == pointID {
			fields = append(fields, FieldError{"primaryPointId", "主吊点不能指向自身"})
		}
		if _, ok := findPoint(p, r.PrimaryPointID); !ok {
			fields = append(fields, FieldError{"primaryPointId", "主吊点不存在"})
		}
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

func validatePointGraph(p *RigPlan) error {
	var fields []FieldError
	for _, relation := range []struct {
		name string
		next func(SuspensionPoint) string
	}{
		{"primaryPointId", func(point SuspensionPoint) string { return point.PrimaryPointID }},
		{"redundantPointId", func(point SuspensionPoint) string { return point.RedundantPointID }},
	} {
		for _, start := range p.Points {
			seen := map[string]bool{}
			current := start
			for {
				if seen[current.ID] {
					fields = append(fields, FieldError{relation.name, fmt.Sprintf("吊点 %s 的关系形成闭环", start.Label)})
					break
				}
				seen[current.ID] = true
				nextID := relation.next(current)
				if nextID == "" {
					break
				}
				next, ok := findPoint(p, nextID)
				if !ok {
					break
				}
				current = *next
			}
		}
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

func validateTest(p *RigPlan, r RecordTestRequest) error {
	var fields []FieldError
	point, ok := findPoint(p, r.PointID)
	if !ok {
		fields = append(fields, FieldError{"pointId", "吊点不存在"})
	}
	if r.TestKind != TestInitial && r.TestKind != TestRetest {
		fields = append(fields, FieldError{"testKind", "试验类型必须为 initial 或 retest"})
	}
	if r.TargetLoadKg <= 0 {
		fields = append(fields, FieldError{"targetLoadKg", "目标载荷必须大于 0 kg"})
	}
	if r.MeasuredLoadKg <= 0 {
		fields = append(fields, FieldError{"measuredLoadKg", "实测载荷必须大于 0 kg"})
	}
	if r.HoldSeconds <= 0 {
		fields = append(fields, FieldError{"holdSeconds", "保持时长必须大于 0 秒"})
	}
	if r.DeformationMm < 0 {
		fields = append(fields, FieldError{"deformationMm", "变形量不能为负数"})
	}
	if r.Outcome != OutcomePass && r.Outcome != OutcomeFail {
		fields = append(fields, FieldError{"outcome", "结论必须为 pass 或 fail"})
	}
	if clean(r.EvidenceDigest) == "" {
		fields = append(fields, FieldError{"evidenceDigest", "证据摘要不能为空"})
	}
	if clean(r.PerformedBy) == "" {
		fields = append(fields, FieldError{"performedBy", "试验员不能为空"})
	}
	if ok && r.TargetLoadKg < point.PlannedLoadKg {
		fields = append(fields, FieldError{"targetLoadKg", fmt.Sprintf("目标载荷不得低于计划载荷 %.2f kg", point.PlannedLoadKg)})
	}
	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}
	return nil
}

func certificateExpired(value, performance string) bool {
	exp, err1 := time.Parse("2006-01-02", value)
	show, err2 := time.Parse("2006-01-02", performance)
	return err1 != nil || err2 != nil || exp.Before(show)
}
