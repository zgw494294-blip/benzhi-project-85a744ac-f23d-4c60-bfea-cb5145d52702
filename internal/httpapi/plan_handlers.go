package httpapi

import (
	"net/http"

	"stage-rig-clearance/internal/rigging"
)

type createPlanBody struct {
	IdempotencyKey   string  `json:"idempotencyKey"`
	VenueName        string  `json:"venueName"`
	PerformanceDate  string  `json:"performanceDate"`
	RatedTotalLoadKg float64 `json:"ratedTotalLoadKg"`
	OwnerName        string  `json:"ownerName"`
}

type updatePlanBody struct {
	IdempotencyKey   string  `json:"idempotencyKey"`
	ExpectedVersion  int64   `json:"expectedVersion"`
	VenueName        string  `json:"venueName"`
	PerformanceDate  string  `json:"performanceDate"`
	RatedTotalLoadKg float64 `json:"ratedTotalLoadKg"`
	OwnerName        string  `json:"ownerName"`
}

func (s *Server) HandleCreatePlan(w http.ResponseWriter, r *http.Request) {
	var body createPlanBody
	if err := decodeJSON(w, r, &body); err != nil {
		writeProblem(w, rigging.Invalid("body", err.Error()))
		return
	}
	if !validIdempotencyKey(body.IdempotencyKey) {
		writeProblem(w, rigging.Invalid("idempotencyKey", "幂等键需为 8-128 位字母、数字或 -_."))
		return
	}
	p, err := s.service.CreatePlan(r.Context(), rigging.CreatePlanRequest{VenueName: body.VenueName, PerformanceDate: body.PerformanceDate, RatedTotalLoadKg: body.RatedTotalLoadKg, OwnerName: body.OwnerName}, body.IdempotencyKey)
	if err != nil {
		writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) HandleGetPlan(w http.ResponseWriter, r *http.Request) {
	p, err := s.service.GetPlan(r.Context(), r.PathValue("planID"))
	if err != nil {
		writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) HandleUpdatePlan(w http.ResponseWriter, r *http.Request) {
	var body updatePlanBody
	if !s.commandBody(w, r, &body, &body.IdempotencyKey) {
		return
	}
	p, err := s.service.UpdatePlan(r.Context(), r.PathValue("planID"), body.ExpectedVersion, rigging.UpdatePlanRequest{VenueName: body.VenueName, PerformanceDate: body.PerformanceDate, RatedTotalLoadKg: body.RatedTotalLoadKg, OwnerName: body.OwnerName}, body.IdempotencyKey)
	s.commandResult(w, p, err)
}

func (s *Server) HandleListPlans(w http.ResponseWriter, r *http.Request) {
	plans, err := s.service.ListPlans(r.Context())
	if err != nil {
		writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plans": plans})
}
