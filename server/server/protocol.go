package server

import "time"

// PlaybackStatus is the enum for stopped/playing/paused status
type PlaybackStatus int

// PlaybackStatus enum instances
const (
	PlaybackStatusStopped PlaybackStatus = 0
	PlaybackStatusPlaying PlaybackStatus = 1
	PlaybackStatusPaused  PlaybackStatus = 2
)

// PlaybackState describes the media playback state in a room
type PlaybackState struct {
	source      string
	status      PlaybackStatus
	position    float64
	speed       float64
	duration    float64
	lastUpdated time.Time
}
