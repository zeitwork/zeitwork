package api

import (
	"encoding/json"
	"net/http"
	"regexp"
)

type WaitlistSignupRequest struct {
	Email string `json:"email"`
}

type WaitlistSignupResponse struct {
	Message string `json:"message"`
	Email   string `json:"email"`
}

// handleWaitlistSignup handles waitlist signup requests
func (s *Service) handleWaitlistSignup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req WaitlistSignupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate email
	if req.Email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	// Basic email validation
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(req.Email) {
		http.Error(w, "Invalid email format", http.StatusBadRequest)
		return
	}

	// Check if already on waitlist
	existing, err := s.db.Queries().WaitlistFindByEmail(ctx, req.Email)
	if err == nil && existing != nil {
		// Already on waitlist
		response := WaitlistSignupResponse{
			Message: "You're already on the waitlist! We'll contact you soon.",
			Email:   req.Email,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// Add to waitlist
	waitlist, err := s.db.Queries().WaitlistCreate(ctx, req.Email)
	if err != nil {
		s.logger.Error("Failed to add to waitlist", "error", err, "email", req.Email)
		http.Error(w, "Failed to add to waitlist", http.StatusInternalServerError)
		return
	}

	s.logger.Info("Added to waitlist", "email", waitlist.Email)

	// Return success response
	response := WaitlistSignupResponse{
		Message: "Thank you! You've been added to our waitlist. We'll contact you soon.",
		Email:   waitlist.Email,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}
