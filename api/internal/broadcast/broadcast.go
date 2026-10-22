// Package broadcast is how the HTTP handlers tell a board's open WebSockets
// what they just saved. The realtime package delivers it.
package broadcast

import "context"

// ChangeIDHeader carries the id the app gave a write, so the broadcast of it
// can say whose it is and the sender can ignore its own.
const ChangeIDHeader = "X-Change-Id"

// A Message's Type is one of these.
const (
	// Change is a Board change that was saved.
	Change = "change"
	// BoardUpdated is a board's new name and description.
	BoardUpdated = "board"
	// BoardDeleted says the board is gone.
	BoardDeleted = "deleted"
	// PresenceChanged carries who has the board open now.
	PresenceChanged = "presence"
)

type Message struct {
	Type string `json:"type"`
	// ID is the sender's id for the write that caused this message, if any.
	ID     string  `json:"id,omitempty"`
	Change any     `json:"change,omitempty"`
	Board  *Detail `json:"board,omitempty"`
	// People is everyone with the board open, for PresenceChanged.
	People []Person `json:"people,omitempty"`
}

// Person is a member with a board open.
type Person struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// Detail is what a board's header shows.
type Detail struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Broadcaster pushes a message to the open connections of one board. It is
// best-effort: a failed push never fails the save it follows.
type Broadcaster interface {
	Broadcast(ctx context.Context, boardID string, message Message)
}

// Discard drops every message.
type Discard struct{}

func (Discard) Broadcast(context.Context, string, Message) {}

// Roster says who has a board open right now: one Person each, however many
// tabs they have it open in.
type Roster interface {
	Present(ctx context.Context, boardID string) []Person
}
