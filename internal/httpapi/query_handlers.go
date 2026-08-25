package httpapi

import (
	"encoding/json"
	"net/http"

	"stage-rig-clearance/internal/rigging"
)

func (s *Server) HandleTimeline(w http.ResponseWriter, r *http.Request) {
	records, verification, err := s.service.Timeline(r.Context(), r.PathValue("planID"))
	if err != nil {
		writeProblem(w, err)
		return
	}
	s.timelineBuffer.Reset()
	if err := json.NewEncoder(&s.timelineBuffer).Encode(map[string]any{"records": records, "verification": verification}); err != nil {
		writeProblem(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(s.timelineBuffer.Bytes())
}

func (s *Server) HandleVerifyCredential(w http.ResponseWriter, r *http.Request) {
	credential, verification, err := s.service.VerifyCredential(r.Context(), r.PathValue("planID"), r.PathValue("credentialID"))
	if err != nil {
		writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"credential": credential, "verification": verification})
}

func (s *Server) HandleVerifyCredentialGlobally(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	digests, ok := query["digest"]
	if !ok || len(query) != 1 || len(digests) != 1 {
		writeProblem(w, rigging.Invalid("digest", "必须且只能提供一个完整凭据摘要参数"))
		return
	}
	verification, err := s.service.VerifyCredentialGlobally(r.Context(), r.PathValue("credentialID"), digests[0])
	if err != nil {
		writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"verification": verification})
}

func (s *Server) HandleCheckPointRemoval(w http.ResponseWriter, r *http.Request) {
	check, err := s.service.CheckPointRemoval(r.Context(), r.PathValue("planID"), r.PathValue("pointID"))
	if err != nil {
		writeProblem(w, err)
		return
	}
	writeJSON(w, http.StatusOK, check)
}
