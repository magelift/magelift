package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const MaxResponseSize int64 = 1 << 20

func EncodeRequest(request Request) ([]byte, error) {
	if err := request.Validate(); err != nil {
		return nil, fmt.Errorf("invalid build runner request: %w", err)
	}
	return json.Marshal(canonicalRequest(request))
}

func EncodeResponse(response Response) ([]byte, error) {
	if err := response.Validate(); err != nil {
		return nil, fmt.Errorf("invalid build runner response: %w", err)
	}
	return json.Marshal(canonicalResponse(response))
}

func DecodeRequest(reader io.Reader) (Request, error) {
	var request Request
	if err := decodeStrict(reader, MaxResponseSize, &request); err != nil {
		return Request{}, fmt.Errorf("decode build runner request: %w", err)
	}
	if err := request.Validate(); err != nil {
		return Request{}, fmt.Errorf("invalid build runner request: %w", err)
	}
	return canonicalRequest(request), nil
}

func DecodeResponse(reader io.Reader) (Response, error) {
	var response Response
	if err := decodeStrict(reader, MaxResponseSize, &response); err != nil {
		return Response{}, fmt.Errorf("decode build runner response: %w", err)
	}
	if err := response.Validate(); err != nil {
		return Response{}, fmt.Errorf("invalid build runner response: %w", err)
	}
	return canonicalResponse(response), nil
}

func decodeStrict(reader io.Reader, maximum int64, destination any) error {
	data, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > maximum {
		return fmt.Errorf("message exceeds %d-byte limit", maximum)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("message contains trailing JSON data")
		}
		return fmt.Errorf("read trailing JSON data: %w", err)
	}
	return nil
}
