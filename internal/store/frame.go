package store

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"stage-rig-clearance/internal/rigging"
)

const schemaVersion = 1
const maxFrameBytes = 16 << 20

var frameChecksum = sha256.New()

type eventFrame struct {
	SchemaVersion   int                    `json:"schemaVersion"`
	Sequence        uint64                 `json:"sequence"`
	PlanID          string                 `json:"planId"`
	ExpectedVersion int64                  `json:"expectedVersion"`
	NewVersion      int64                  `json:"newVersion"`
	Events          []rigging.DomainEvent  `json:"events"`
	Aggregate       *rigging.RigPlan       `json:"aggregate"`
	IdempotencyKey  string                 `json:"idempotencyKey"`
	Action          string                 `json:"action"`
	Receipt         rigging.CommandReceipt `json:"receipt"`
	Checksum        string                 `json:"checksum"`
}

func checksumFrame(frame eventFrame) (string, error) {
	frame.Checksum = ""
	b, err := json.Marshal(frame)
	if err != nil {
		return "", err
	}
	frameChecksum.Reset()
	if _, err := frameChecksum.Write(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(frameChecksum.Sum(nil)), nil
}

func encodeFrame(frame eventFrame) ([]byte, error) {
	checksum, err := checksumFrame(frame)
	if err != nil {
		return nil, err
	}
	frame.Checksum = checksum
	payload, err := json.Marshal(frame)
	if err != nil {
		return nil, err
	}
	if len(payload) > maxFrameBytes {
		return nil, fmt.Errorf("event frame exceeds %d bytes", maxFrameBytes)
	}
	result := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(result[:4], uint32(len(payload)))
	copy(result[4:], payload)
	return result, nil
}

func decodeFrames(reader io.Reader, visit func(eventFrame) error) error {
	var header [4]byte
	for {
		_, err := io.ReadFull(reader, header[:])
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read event frame length: %w", err)
		}
		length := binary.BigEndian.Uint32(header[:])
		if length == 0 || length > maxFrameBytes {
			return fmt.Errorf("invalid event frame length %d", length)
		}
		payload := make([]byte, int(length))
		if _, err := io.ReadFull(reader, payload); err != nil {
			return fmt.Errorf("read event frame payload: %w", err)
		}
		var frame eventFrame
		if err := json.Unmarshal(payload, &frame); err != nil {
			return fmt.Errorf("decode event frame: %w", err)
		}
		if frame.SchemaVersion != schemaVersion {
			return fmt.Errorf("unsupported event schemaVersion %d", frame.SchemaVersion)
		}
		actual, err := checksumFrame(frame)
		if err != nil {
			return err
		}
		if actual != frame.Checksum {
			return fmt.Errorf("event frame %d checksum mismatch", frame.Sequence)
		}
		if err := visit(frame); err != nil {
			return err
		}
	}
}
