package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"stage-rig-clearance/internal/rigging"
)

type problem struct {
	Type   string               `json:"type"`
	Title  string               `json:"title"`
	Status int                  `json:"status"`
	Detail string               `json:"detail"`
	Fields []rigging.FieldError `json:"fields,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeProblem(w http.ResponseWriter, err error) {
	status, kind, title := http.StatusInternalServerError, "internal", "服务器内部错误"
	switch {
	case errors.Is(err, rigging.ErrValidation):
		status, kind, title = 422, "validation", "输入校验失败"
	case errors.Is(err, rigging.ErrNotFound):
		status, kind, title = 404, "not_found", "资源不存在"
	case errors.Is(err, rigging.ErrVersionConflict):
		status, kind, title = 409, "version_conflict", "版本冲突"
	case errors.Is(err, rigging.ErrStateConflict):
		status, kind, title = 409, "state_conflict", "状态不允许该操作"
	case errors.Is(err, rigging.ErrFrozen):
		status, kind, title = 409, "frozen", "方案已冻结"
	case errors.Is(err, rigging.ErrIdempotency):
		status, kind, title = 409, "idempotency_conflict", "幂等键冲突"
	}
	p := problem{Type: "https://stage-rig.local/problems/" + kind, Title: title, Status: status, Detail: err.Error()}
	var validation *rigging.ValidationError
	if errors.As(err, &validation) {
		p.Fields = validation.Fields
	}
	writeJSON(w, status, p)
}
