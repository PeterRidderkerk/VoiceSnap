package services

import (
	"voicesnap/internal/engine"
	"voicesnap/internal/model"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// EngineService provides engine status and model management to the frontend.
type EngineService struct {
	app          *application.App
	initCallback func()
}

func NewEngineService() *EngineService {
	return &EngineService{}
}

// ModelExists returns true if the ASR model files are present.
func (s *EngineService) ModelExists() bool {
	return engine.ModelExists()
}

// DownloadModel downloads the ASR model with progress events.
func (s *EngineService) DownloadModel(primaryURL, fallbackURL string) error {
	modelsDir := engine.ModelDir()
	// Go up one level from sensevoice dir
	modelsDir = modelsDir[:len(modelsDir)-len("/sensevoice")]

	err := model.Download(primaryURL, fallbackURL, modelsDir, func(percent float64, downloaded, total int64) {
		if s.app != nil {
			s.app.Event.Emit("model:download-progress", map[string]interface{}{
				"percent":    percent,
				"downloaded": downloaded,
				"total":      total,
			})
		}
	})
	if err != nil {
		return err
	}

	if s.initCallback != nil {
		go s.initCallback()
	}
	return nil
}

// SetInitCallback sets the callback to re-initialize the engine after model download.
func (s *EngineService) SetInitCallback(cb func()) {
	s.initCallback = cb
}

// SetApp sets the Wails app reference for event emission.
func (s *EngineService) SetApp(app *application.App) {
	s.app = app
}
