package rigging

import "context"

type DomainEvent struct {
	Type    string         `json:"type"`
	Actor   string         `json:"actor"`
	Payload map[string]any `json:"payload"`
}

type CommandReceipt struct {
	PlanID        string   `json:"planId"`
	Version       int64    `json:"version"`
	ResourceID    string   `json:"resourceId,omitempty"`
	Action        string   `json:"action"`
	RequestDigest string   `json:"requestDigest,omitempty"`
	ResourceIDs   []string `json:"resourceIds,omitempty"`
	BatchID       string   `json:"batchId,omitempty"`
}

type Repository interface {
	Get(context.Context, string) (*RigPlan, error)
	List(context.Context) ([]*RigPlan, error)
	LookupCommand(context.Context, string, string) (*CommandReceipt, error)
	Commit(context.Context, *RigPlan, int64, []DomainEvent, string, string, CommandReceipt) error
	FindCredential(context.Context, string) (*RigPlan, ClearanceCredential, error)
}

type Auditor interface {
	Append(context.Context, string, string, string, map[string]any) (AuditRecord, error)
	Timeline(context.Context, string) ([]AuditRecord, Verification, error)
	Prepare(context.Context, string, string, string) (ClearanceCredential, error)
	Seal(context.Context, ClearanceCredential) (AuditRecord, error)
	VerifyCredential(context.Context, ClearanceCredential) (Verification, error)
}
