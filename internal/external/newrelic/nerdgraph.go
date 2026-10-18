package newrelic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type graphQLError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// executeNerdGraph is the shared provider-owned transport boundary for
// credential lifecycle and query verification. Callers pass only an API key
// for the duration of this request; the key never enters a result, error, or
// query document.
func executeNerdGraph(ctx context.Context, httpClient HTTPDoer, endpoint string, apiKey []byte, query string, variables map[string]any, output any) error {
	if ctx == nil {
		return errors.New("New Relic NerdGraph context is required")
	}
	if httpClient == nil {
		return errors.New("New Relic NerdGraph HTTP client is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	body, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return errors.New("marshal New Relic NerdGraph request failed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return errors.New("build New Relic NerdGraph request failed")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("API-Key", string(apiKey))
	response, err := httpClient.Do(req)
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		return errors.New("New Relic NerdGraph request failed")
	}
	if response == nil {
		return errors.New("New Relic NerdGraph request returned no response")
	}
	if response.Body == nil {
		return errors.New("New Relic NerdGraph response had no body")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("New Relic NerdGraph request returned status %d", response.StatusCode)
	}
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []graphQLError  `json:"errors"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&envelope); err != nil {
		return errors.New("decode New Relic NerdGraph response failed")
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return errors.New("New Relic NerdGraph request returned GraphQL errors")
	}
	if output == nil {
		if len(envelope.Errors) > 0 {
			return errors.New("New Relic NerdGraph request returned GraphQL errors")
		}
		return nil
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return errors.New("decode New Relic NerdGraph data failed")
	}
	if len(envelope.Errors) > 0 {
		// Decode partial data before returning the GraphQL error. New Relic
		// mutations can commit an object while also reporting a non-fatal
		// top-level error; create callers use the returned identity to make
		// rollback ownership-safe.
		return errors.New("New Relic NerdGraph request returned GraphQL errors")
	}
	return nil
}
