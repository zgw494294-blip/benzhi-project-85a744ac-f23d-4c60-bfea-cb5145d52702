package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"stage-rig-clearance/internal/rigging"
)

func auditDigest(record rigging.AuditRecord) (string, error) {
	record.Digest = ""
	b, err := json.Marshal(record)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func credentialDigest(credential rigging.ClearanceCredential) (string, error) {
	credential.CredentialDigest = ""
	b, err := json.Marshal(credential)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
