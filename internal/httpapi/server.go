package httpapi

import (
	"bytes"
	"embed"
	"net/http"

	"stage-rig-clearance/internal/rigging"
)

//go:embed web/*
var webFiles embed.FS

type Server struct {
	service        *rigging.Service
	mux            *http.ServeMux
	timelineBuffer bytes.Buffer
}

func New(service *rigging.Service) *Server {
	s := &Server{service: service, mux: http.NewServeMux()}
	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return securityHeaders(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /", s.HandleIndex)
	s.mux.HandleFunc("GET /app.css", s.HandleCSS)
	s.mux.HandleFunc("GET /app.js", s.HandleJS)
	s.mux.HandleFunc("GET /api/plans", s.HandleListPlans)
	s.mux.HandleFunc("POST /api/plans", s.HandleCreatePlan)
	s.mux.HandleFunc("GET /api/plans/{planID}", s.HandleGetPlan)
	s.mux.HandleFunc("PATCH /api/plans/{planID}", s.HandleUpdatePlan)
	s.mux.HandleFunc("POST /api/plans/{planID}/points", s.HandleAddPoint)
	s.mux.HandleFunc("PATCH /api/plans/{planID}/points/{pointID}", s.HandleUpdatePoint)
	s.mux.HandleFunc("GET /api/plans/{planID}/points/{pointID}/removal-check", s.HandleCheckPointRemoval)
	s.mux.HandleFunc("DELETE /api/plans/{planID}/points/{pointID}", s.HandleRemovePoint)
	s.mux.HandleFunc("POST /api/plans/{planID}/tests", s.HandleRecordTest)
	s.mux.HandleFunc("POST /api/plans/{planID}/tests/batch", s.HandleRecordTestBatch)
	s.mux.HandleFunc("POST /api/plans/{planID}/remediations", s.HandleRemediate)
	s.mux.HandleFunc("POST /api/plans/{planID}/review", s.HandleReview)
	s.mux.HandleFunc("POST /api/plans/{planID}/credentials", s.HandleIssueCredential)
	s.mux.HandleFunc("GET /api/plans/{planID}/audit", s.HandleTimeline)
	s.mux.HandleFunc("GET /api/plans/{planID}/credentials/{credentialID}/verify", s.HandleVerifyCredential)
	s.mux.HandleFunc("GET /api/credentials/{credentialID}/verify", s.HandleVerifyCredentialGlobally)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}
