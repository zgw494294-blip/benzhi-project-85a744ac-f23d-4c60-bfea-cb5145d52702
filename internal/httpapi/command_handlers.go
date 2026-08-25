package httpapi

import (
	"net/http"

	"stage-rig-clearance/internal/rigging"
)

type addPointBody struct {
	IdempotencyKey       string  `json:"idempotencyKey"`
	ExpectedVersion      int64   `json:"expectedVersion"`
	Label                string  `json:"label"`
	RatedLoadKg          float64 `json:"ratedLoadKg"`
	PlannedLoadKg        float64 `json:"plannedLoadKg"`
	DeviceModel          string  `json:"deviceModel"`
	CableSpec            string  `json:"cableSpec"`
	PrimaryPointID       string  `json:"primaryPointId"`
	RedundantPointID     string  `json:"redundantPointId"`
	CertificateExpiresOn string  `json:"certificateExpiresOn"`
}

func (s *Server) HandleAddPoint(w http.ResponseWriter, r *http.Request) {
	var b addPointBody
	if !s.commandBody(w, r, &b, &b.IdempotencyKey) {
		return
	}
	p, err := s.service.AddPoint(r.Context(), r.PathValue("planID"), b.ExpectedVersion, rigging.AddPointRequest{Label: b.Label, RatedLoadKg: b.RatedLoadKg, PlannedLoadKg: b.PlannedLoadKg, DeviceModel: b.DeviceModel, CableSpec: b.CableSpec, PrimaryPointID: b.PrimaryPointID, RedundantPointID: b.RedundantPointID, CertificateExpiresOn: b.CertificateExpiresOn}, b.IdempotencyKey)
	s.commandResult(w, p, err)
}

func (s *Server) HandleUpdatePoint(w http.ResponseWriter, r *http.Request) {
	var b addPointBody
	if !s.commandBody(w, r, &b, &b.IdempotencyKey) {
		return
	}
	p, err := s.service.UpdatePoint(r.Context(), r.PathValue("planID"), r.PathValue("pointID"), b.ExpectedVersion, rigging.UpdatePointRequest{Label: b.Label, RatedLoadKg: b.RatedLoadKg, PlannedLoadKg: b.PlannedLoadKg, DeviceModel: b.DeviceModel, CableSpec: b.CableSpec, PrimaryPointID: b.PrimaryPointID, RedundantPointID: b.RedundantPointID, CertificateExpiresOn: b.CertificateExpiresOn}, b.IdempotencyKey)
	s.commandResult(w, p, err)
}

type removePointBody struct {
	IdempotencyKey  string `json:"idempotencyKey"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

func (s *Server) HandleRemovePoint(w http.ResponseWriter, r *http.Request) {
	var b removePointBody
	if !s.commandBody(w, r, &b, &b.IdempotencyKey) {
		return
	}
	p, err := s.service.RemovePoint(r.Context(), r.PathValue("planID"), r.PathValue("pointID"), b.ExpectedVersion, b.IdempotencyKey)
	s.commandResult(w, p, err)
}

type testBody struct {
	IdempotencyKey    string              `json:"idempotencyKey"`
	ExpectedVersion   int64               `json:"expectedVersion"`
	PointID           string              `json:"pointId"`
	TestKind          rigging.TestKind    `json:"testKind"`
	TargetLoadKg      float64             `json:"targetLoadKg"`
	MeasuredLoadKg    float64             `json:"measuredLoadKg"`
	HoldSeconds       int                 `json:"holdSeconds"`
	DeformationMm     float64             `json:"deformationMm"`
	Outcome           rigging.TestOutcome `json:"outcome"`
	EvidenceDigest    string              `json:"evidenceDigest"`
	PerformedBy       string              `json:"performedBy"`
	PointConfigDigest string              `json:"pointConfigDigest,omitempty"`
	RetestTaskID      string              `json:"retestTaskId,omitempty"`
}

func (b testBody) request() rigging.RecordTestRequest {
	return rigging.RecordTestRequest{PointID: b.PointID, TestKind: b.TestKind, TargetLoadKg: b.TargetLoadKg, MeasuredLoadKg: b.MeasuredLoadKg, HoldSeconds: b.HoldSeconds, DeformationMm: b.DeformationMm, Outcome: b.Outcome, EvidenceDigest: b.EvidenceDigest, PerformedBy: b.PerformedBy, PointConfigDigest: b.PointConfigDigest, RetestTaskID: b.RetestTaskID}
}

func (s *Server) HandleRecordTest(w http.ResponseWriter, r *http.Request) {
	var b testBody
	if !s.commandBody(w, r, &b, &b.IdempotencyKey) {
		return
	}
	if b.TestKind == rigging.TestRetest && b.RetestTaskID == "" {
		writeProblem(w, rigging.Invalid("retestTaskId", "整改复测必须指定待复测任务"))
		return
	}
	p, err := s.service.RecordTest(r.Context(), r.PathValue("planID"), b.ExpectedVersion, b.request(), b.IdempotencyKey)
	s.commandResult(w, p, err)
}

type testRowBody struct {
	PointID           string              `json:"pointId"`
	TestKind          rigging.TestKind    `json:"testKind"`
	TargetLoadKg      float64             `json:"targetLoadKg"`
	MeasuredLoadKg    float64             `json:"measuredLoadKg"`
	HoldSeconds       int                 `json:"holdSeconds"`
	DeformationMm     float64             `json:"deformationMm"`
	Outcome           rigging.TestOutcome `json:"outcome"`
	EvidenceDigest    string              `json:"evidenceDigest"`
	PerformedBy       string              `json:"performedBy"`
	PointConfigDigest string              `json:"pointConfigDigest"`
}

func (b testRowBody) request() rigging.RecordTestRequest {
	return rigging.RecordTestRequest{PointID: b.PointID, TestKind: b.TestKind, TargetLoadKg: b.TargetLoadKg, MeasuredLoadKg: b.MeasuredLoadKg, HoldSeconds: b.HoldSeconds, DeformationMm: b.DeformationMm, Outcome: b.Outcome, EvidenceDigest: b.EvidenceDigest, PerformedBy: b.PerformedBy, PointConfigDigest: b.PointConfigDigest}
}

type testBatchBody struct {
	IdempotencyKey  string        `json:"idempotencyKey"`
	ExpectedVersion int64         `json:"expectedVersion"`
	Tests           []testRowBody `json:"tests"`
}

func (s *Server) HandleRecordTestBatch(w http.ResponseWriter, r *http.Request) {
	var b testBatchBody
	if !s.commandBody(w, r, &b, &b.IdempotencyKey) {
		return
	}
	request := rigging.RecordTestBatchRequest{Tests: make([]rigging.RecordTestRequest, 0, len(b.Tests))}
	for _, row := range b.Tests {
		request.Tests = append(request.Tests, row.request())
	}
	result, err := s.service.RecordTestBatch(r.Context(), r.PathValue("planID"), b.ExpectedVersion, request, b.IdempotencyKey)
	if err != nil {
		writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type remediationBody struct {
	IdempotencyKey       string   `json:"idempotencyKey"`
	ExpectedVersion      int64    `json:"expectedVersion"`
	IssueID              string   `json:"issueId"`
	PointID              string   `json:"pointId,omitempty"`
	Note                 string   `json:"note"`
	RevisedBy            string   `json:"revisedBy"`
	PlannedLoadKg        *float64 `json:"plannedLoadKg,omitempty"`
	RedundantPointID     *string  `json:"redundantPointId,omitempty"`
	CertificateExpiresOn *string  `json:"certificateExpiresOn,omitempty"`
}

func (s *Server) HandleRemediate(w http.ResponseWriter, r *http.Request) {
	var b remediationBody
	if !s.commandBody(w, r, &b, &b.IdempotencyKey) {
		return
	}
	p, err := s.service.Remediate(r.Context(), r.PathValue("planID"), b.ExpectedVersion, rigging.RemediateRequest{IssueID: b.IssueID, PointID: b.PointID, Note: b.Note, RevisedBy: b.RevisedBy, PlannedLoadKg: b.PlannedLoadKg, RedundantPointID: b.RedundantPointID, CertificateExpiresOn: b.CertificateExpiresOn}, b.IdempotencyKey)
	s.commandResult(w, p, err)
}

type reviewBody struct {
	IdempotencyKey  string `json:"idempotencyKey"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Decision        string `json:"decision"`
	Reviewer        string `json:"reviewer"`
	Note            string `json:"note"`
}

func (s *Server) HandleReview(w http.ResponseWriter, r *http.Request) {
	var b reviewBody
	if !s.commandBody(w, r, &b, &b.IdempotencyKey) {
		return
	}
	p, err := s.service.Review(r.Context(), r.PathValue("planID"), b.ExpectedVersion, rigging.ReviewRequest{Decision: b.Decision, Reviewer: b.Reviewer, Note: b.Note}, b.IdempotencyKey)
	s.commandResult(w, p, err)
}

type credentialBody struct {
	IdempotencyKey  string `json:"idempotencyKey"`
	ExpectedVersion int64  `json:"expectedVersion"`
	IssuedBy        string `json:"issuedBy"`
}

func (s *Server) HandleIssueCredential(w http.ResponseWriter, r *http.Request) {
	var b credentialBody
	if !s.commandBody(w, r, &b, &b.IdempotencyKey) {
		return
	}
	p, err := s.service.IssueCredential(r.Context(), r.PathValue("planID"), b.ExpectedVersion, b.IssuedBy, b.IdempotencyKey)
	s.commandResult(w, p, err)
}

func (s *Server) commandBody(w http.ResponseWriter, r *http.Request, destination any, key *string) bool {
	if err := decodeJSON(w, r, destination); err != nil {
		writeProblem(w, rigging.Invalid("body", err.Error()))
		return false
	}
	if !validIdempotencyKey(*key) {
		writeProblem(w, rigging.Invalid("idempotencyKey", "幂等键需为 8-128 位字母、数字或 -_."))
		return false
	}
	return true
}

func (s *Server) commandResult(w http.ResponseWriter, p *rigging.RigPlan, err error) {
	if err != nil {
		writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}
