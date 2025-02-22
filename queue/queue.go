package queue

import (
	"sync"
)

type Song struct {
	URL      string
	Filename string
}

type MusicQueue struct {
	mu          sync.Mutex
	songs       []Song
	playing     bool
	paused      bool
	Done        chan bool
	currentSong *Song // Add this field
}

func NewMusicQueue() *MusicQueue {
	return &MusicQueue{
		songs:       make([]Song, 0),
		Done:        make(chan bool, 1),
		playing:     false,
		paused:      false,
		currentSong: nil,
	}
}

func (q *MusicQueue) Add(song Song) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.songs = append(q.songs, song)
}

func (q *MusicQueue) Next() (Song, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.songs) == 0 {
		return Song{}, false
	}
	song := q.songs[0]
	q.songs = q.songs[1:]
	return song, true
}

func (q *MusicQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.songs = make([]Song, 0)
}

func (q *MusicQueue) List() []Song {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]Song(nil), q.songs...)
}

func (q *MusicQueue) Stop() {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.playing {
		q.paused = true
		q.Done <- true
	}
}

func (q *MusicQueue) Resume() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.paused = false
	q.playing = false // Reset playing state to allow playback to restart
}

func (q *MusicQueue) SetCurrentSong(song *Song) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.currentSong = song
}

func (q *MusicQueue) GetCurrentSong() *Song {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.currentSong
}

func (q *MusicQueue) IsPlaying() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.playing
}

func (q *MusicQueue) IsPaused() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.paused
}

func (q *MusicQueue) SetPlaying(playing bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.playing = playing
}
