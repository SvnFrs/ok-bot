package youtube

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Song struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
}

type SongLibrary struct {
	Songs map[string]Song `json:"songs"`
}

func GetVideoID(url string) string {
	// Handle different URL formats
	if strings.Contains(url, "youtube.com/watch?v=") {
		parts := strings.Split(url, "v=")
		if len(parts) > 1 {
			return strings.Split(parts[1], "&")[0]
		}
	} else if strings.Contains(url, "youtu.be/") {
		parts := strings.Split(url, "youtu.be/")
		if len(parts) > 1 {
			return strings.Split(parts[1], "?")[0]
		}
	}
	return ""
}

func DownloadAudio(url string) (string, error) {
	videoID := GetVideoID(url)
	if videoID == "" {
		return "", fmt.Errorf("invalid YouTube URL")
	}

	// Check if song already exists in library
	library, err := loadSongLibrary()
	if err != nil {
		return "", err
	}

	if song, exists := library.Songs[videoID]; exists {
		return song.Filename, nil
	}

	// Download the audio
	cmd := exec.Command("./yt-dlp_linux",
		"-x",                        // Extract audio
		"--audio-format", "opus",    // Convert to opus format
		"-o", "songs/%(id)s.%(ext)s", // Output format
		url)

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to download audio: %v", err)
	}

	filename := fmt.Sprintf("songs/%s.opus", videoID)

	// Add to library
	if library.Songs == nil {
		library.Songs = make(map[string]Song)
	}
	library.Songs[videoID] = Song{
		ID:       videoID,
		Filename: filename,
	}

	// Save updated library
	if err := saveSongLibrary(library); err != nil {
		return "", err
	}

	return filename, nil
}

func loadSongLibrary() (SongLibrary, error) {
	var library SongLibrary
	file, err := os.ReadFile("songs.json")
	if err != nil {
		if os.IsNotExist(err) {
			return SongLibrary{Songs: make(map[string]Song)}, nil
		}
		return library, err
	}

	err = json.Unmarshal(file, &library)
	return library, err
}

func saveSongLibrary(library SongLibrary) error {
	data, err := json.MarshalIndent(library, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile("songs.json", data, 0644)
}
