package audit

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const maxFrameSize = 4 << 20

func writeFrame(w io.Writer, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(b) == 0 || len(b) > maxFrameSize {
		return fmt.Errorf("invalid audit frame size %d", len(b))
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(b)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func readFrames(r io.Reader, visit func([]byte) error) error {
	var header [4]byte
	for {
		_, err := io.ReadFull(r, header[:])
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read frame length: %w", err)
		}
		length := binary.BigEndian.Uint32(header[:])
		if length == 0 || length > maxFrameSize {
			return fmt.Errorf("invalid frame length %d", length)
		}
		payload := make([]byte, int(length))
		if _, err := io.ReadFull(r, payload); err != nil {
			return fmt.Errorf("read frame payload: %w", err)
		}
		if err := visit(payload); err != nil {
			return err
		}
	}
}
