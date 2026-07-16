package protocol

import (
	"encoding/json"
	"time"
)

// NowMillis returns the current unix time in milliseconds.
func NowMillis() int64 { return time.Now().UnixMilli() }

// Encode marshals a payload into an Envelope of the given type.
func Encode(msgType string, payload any) ([]byte, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return json.Marshal(Envelope{
		V:       Version,
		Type:    msgType,
		TS:      NowMillis(),
		Payload: raw,
	})
}

// Decode unmarshals raw bytes into an Envelope.
func Decode(data []byte) (*Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}
	return &env, nil
}

// As unmarshals an envelope payload into T.
func As[T any](env *Envelope) (T, error) {
	var v T
	err := json.Unmarshal(env.Payload, &v)
	return v, err
}
