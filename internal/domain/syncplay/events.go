package syncplay

import "time"

type RoomCreated struct {
	RoomID     string
	HostID     string
	MediaID    string
	OccurredAt time.Time
}

func (e RoomCreated) EventName() string { return "syncplay.room.created" }

type RoomClosed struct {
	RoomID     string
	OccurredAt time.Time
}

func (e RoomClosed) EventName() string { return "syncplay.room.closed" }

type MemberJoined struct {
	RoomID     string
	UserID     string
	OccurredAt time.Time
}

func (e MemberJoined) EventName() string { return "syncplay.member.joined" }

type MemberLeft struct {
	RoomID     string
	UserID     string
	OccurredAt time.Time
}

func (e MemberLeft) EventName() string { return "syncplay.member.left" }
