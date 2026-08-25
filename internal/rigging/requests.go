package rigging

type CreatePlanRequest struct {
	VenueName        string  `json:"venueName"`
	PerformanceDate  string  `json:"performanceDate"`
	RatedTotalLoadKg float64 `json:"ratedTotalLoadKg"`
	OwnerName        string  `json:"ownerName"`
}

type UpdatePlanRequest struct {
	VenueName        string  `json:"venueName"`
	PerformanceDate  string  `json:"performanceDate"`
	RatedTotalLoadKg float64 `json:"ratedTotalLoadKg"`
	OwnerName        string  `json:"ownerName"`
}

type AddPointRequest struct {
	Label                string  `json:"label"`
	RatedLoadKg          float64 `json:"ratedLoadKg"`
	PlannedLoadKg        float64 `json:"plannedLoadKg"`
	DeviceModel          string  `json:"deviceModel"`
	CableSpec            string  `json:"cableSpec"`
	PrimaryPointID       string  `json:"primaryPointId"`
	RedundantPointID     string  `json:"redundantPointId"`
	CertificateExpiresOn string  `json:"certificateExpiresOn"`
}

type UpdatePointRequest = AddPointRequest

type RecordTestRequest struct {
	PointID           string      `json:"pointId"`
	TestKind          TestKind    `json:"testKind"`
	TargetLoadKg      float64     `json:"targetLoadKg"`
	MeasuredLoadKg    float64     `json:"measuredLoadKg"`
	HoldSeconds       int         `json:"holdSeconds"`
	DeformationMm     float64     `json:"deformationMm"`
	Outcome           TestOutcome `json:"outcome"`
	EvidenceDigest    string      `json:"evidenceDigest"`
	PerformedBy       string      `json:"performedBy"`
	PointConfigDigest string      `json:"pointConfigDigest,omitempty"`
	RetestTaskID      string      `json:"retestTaskId,omitempty"`
}

type RecordTestBatchRequest struct {
	Tests []RecordTestRequest `json:"tests"`
}

type ArchivedTestResult struct {
	TestID   string   `json:"testId"`
	PointID  string   `json:"pointId"`
	Findings []string `json:"findings"`
}

type TestBatchResult struct {
	BatchID string               `json:"batchId"`
	Plan    *RigPlan             `json:"plan"`
	Tests   []ArchivedTestResult `json:"tests"`
}

type RemediateRequest struct {
	IssueID              string   `json:"issueId"`
	PointID              string   `json:"pointId,omitempty"`
	Note                 string   `json:"note"`
	RevisedBy            string   `json:"revisedBy"`
	PlannedLoadKg        *float64 `json:"plannedLoadKg,omitempty"`
	RedundantPointID     *string  `json:"redundantPointId,omitempty"`
	CertificateExpiresOn *string  `json:"certificateExpiresOn,omitempty"`
}

type ReviewRequest struct {
	Decision string `json:"decision"`
	Reviewer string `json:"reviewer"`
	Note     string `json:"note"`
}
