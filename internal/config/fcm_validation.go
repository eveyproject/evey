package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// ValidateFCMServiceAccount checks that path points to a readable, valid Firebase service account JSON.
// Empty path is valid (FCM disabled).
func ValidateFCMServiceAccount(path string) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read FCM service account: %w", err)
	}
	var sa struct {
		Type        string `json:"type"`
		ProjectID   string `json:"project_id"`
		PrivateKey  string `json:"private_key"`
		ClientEmail string `json:"client_email"`
	}
	if err := json.Unmarshal(data, &sa); err != nil {
		return fmt.Errorf("parse FCM service account JSON: %w", err)
	}
	if sa.Type != "service_account" {
		return fmt.Errorf("FCM service account: expected type=service_account, got %q", sa.Type)
	}
	missing := []string{}
	if sa.ProjectID == "" {
		missing = append(missing, "project_id")
	}
	if sa.PrivateKey == "" {
		missing = append(missing, "private_key")
	}
	if sa.ClientEmail == "" {
		missing = append(missing, "client_email")
	}
	if len(missing) > 0 {
		return fmt.Errorf("FCM service account missing fields: %v", missing)
	}
	return nil
}
