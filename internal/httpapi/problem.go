package httpapi

import (
	"encoding/json"
	"net/http"
)

// Problem is an RFC 9457 problem; returning one as an error renders it.
type Problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

func (p *Problem) Error() string { return p.Title + ": " + p.Detail }

func writeProblem(w http.ResponseWriter, p *Problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

func badRequest(detail string) *Problem {
	return &Problem{Type: "/problems/bad-request", Title: "Bad request", Status: http.StatusBadRequest, Detail: detail}
}

func invalid(detail string) *Problem {
	return &Problem{Type: "/problems/invalid", Title: "Invalid input", Status: http.StatusUnprocessableEntity, Detail: detail}
}

func unauthenticated() *Problem {
	return &Problem{Type: "/problems/unauthenticated", Title: "Sign in required", Status: http.StatusUnauthorized,
		Detail: "No valid session for this browser and IP address."}
}

func forbidden(detail string) *Problem {
	return &Problem{Type: "/problems/forbidden", Title: "Not allowed", Status: http.StatusForbidden, Detail: detail}
}

func reauthRequired() *Problem {
	return &Problem{Type: "/problems/reauth-required", Title: "Confirm with your passkey", Status: http.StatusForbidden,
		Detail: "This action needs a passkey confirmation from the last five minutes."}
}

func conflict(detail string) *Problem {
	return &Problem{Type: "/problems/conflict", Title: "Not possible right now", Status: http.StatusConflict, Detail: detail}
}

func notFound(detail string) *Problem {
	return &Problem{Type: "/problems/not-found", Title: "Not found", Status: http.StatusNotFound, Detail: detail}
}
