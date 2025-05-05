package auth

import (
	"strconv"
	"strings"
)

// PolicyService manages user permissions for the bot.
type PolicyService struct {
	AdminUserIDs   map[int64]bool // map of admin user IDs
	AllowedUserIDs map[int64]bool // map of allowed user IDs (if empty, all users are allowed)
}

// NewPolicyService creates a new PolicyService.
func NewPolicyService(adminUserIDsStr, allowedUserIDsStr string) *PolicyService {
	adminUserIDs := make(map[int64]bool)
	allowedUserIDs := make(map[int64]bool)

	// Parse admin user IDs
	if adminUserIDsStr != "" {
		ids := strings.Split(adminUserIDsStr, ",")
		for _, idStr := range ids {
			id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err == nil {
				adminUserIDs[id] = true
			}
		}
	}

	// Parse allowed user IDs
	if allowedUserIDsStr != "" {
		ids := strings.Split(allowedUserIDsStr, ",")
		for _, idStr := range ids {
			id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err == nil {
				allowedUserIDs[id] = true
			}
		}
	}

	return &PolicyService{
		AdminUserIDs:   adminUserIDs,
		AllowedUserIDs: allowedUserIDs,
	}
}

// IsAdmin checks if a user is an admin.
func (p *PolicyService) IsAdmin(userID int64) bool {
	return p.AdminUserIDs[userID]
}

// IsAllowed checks if a user is allowed to use the bot.
func (p *PolicyService) IsAllowed(userID int64) bool {
	// If the allowed users list is empty, all users are allowed
	if len(p.AllowedUserIDs) == 0 {
		return true
	}

	// Admins are always allowed
	if p.IsAdmin(userID) {
		return true
	}

	// Check if the user is in the allowed list
	return p.AllowedUserIDs[userID]
}

// IsToolAllowed checks if a user is allowed to use a specific tool.
func (p *PolicyService) IsToolAllowed(userID int64, toolName string) bool {
	// Admins can use all tools
	if p.IsAdmin(userID) {
		return true
	}

	// Tool list for non-admins - only include tools defined in tools-spec
	allowedToolsMap := map[string]bool{
		"store_person_memory":      true,
		"store_self_memory":        true,
		"store_community_memory":   true,
		"remember_about_person":    true,
		"remember_about_self":      true,
		"remember_about_community": true,
	}

	return allowedToolsMap[toolName]
}

// GetAllowedTools returns a list of tool names that a user is allowed to use.
func (p *PolicyService) GetAllowedTools(userID int64) []string {
	// Define available tools (should match tools-spec directory)
	allTools := []string{
		"store_person_memory",
		"store_self_memory",
		"store_community_memory",
		"remember_about_person",
		"remember_about_self",
		"remember_about_community",
	}

	// If user is admin, they can use all defined tools
	if p.IsAdmin(userID) {
		return allTools
	}

	// For regular users, return the same list
	return allTools
}
