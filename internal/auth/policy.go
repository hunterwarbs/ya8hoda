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

	// Regular users can only use specific tools
	switch toolName {
	case "milvus.search":
		// Everyone can search
		return true
	case "milvus.store_document":
		// Only admins can store documents
		return p.IsAdmin(userID)
	case "openrouter.generate_image":
		// Everyone can generate images
		return true
	default:
		// Unknown tools are not allowed
		return false
	}
}
