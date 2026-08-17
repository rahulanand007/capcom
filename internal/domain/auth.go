package domain

import "time"

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type Organization struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type Principal struct {
	UserID         string
	Email          string
	OrganizationID string
	Organization   Organization
	Role           string
	SessionID      string
	CSRFHash       []byte
	PlatformAdmin  bool
}

type AuthSession struct {
	User           User         `json:"user"`
	Organization   Organization `json:"organization"`
	Role           string       `json:"role"`
	SessionToken   string       `json:"-"`
	CSRFToken      string       `json:"-"`
	AbsoluteExpiry time.Time    `json:"-"`
	IdleExpiry     time.Time    `json:"-"`
}
