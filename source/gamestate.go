package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// What the updater saw the last time it patched, so the next start can tell when the game's files
// changed (a game update, a repair/verify, or another tool) and tell the player to patch again.
type gameState struct {
	Time      string `json:"time"`
	SourceSHA string `json:"source_sha256"` // the game's data file as it was read
	OutputSHA string `json:"output_sha256"` // the patched file this updater produced
	DllSHA    string `json:"dll_sha256"`    // SoulWorker64.dll
}

func (a *App) statePath() string { return filepath.Join(a.dir, "cache", "last_patch.json") }

func (a *App) gameDataFile() string {
	return filepath.Join(a.cfg.SWPath, "datas", a.cfg.DataSource+".v")
}

func (a *App) saveGameState(sourceSHA, outputSHA string) {
	dll, _ := hashFile(filepath.Join(a.cfg.SWPath, "SoulWorker64.dll"))
	b, _ := json.MarshalIndent(gameState{Time: time.Now().Format(time.RFC3339), SourceSHA: sourceSHA, OutputSHA: outputSHA, DllSHA: dll}, "", "  ")
	if os.MkdirAll(filepath.Dir(a.statePath()), 0o755) == nil {
		_ = os.WriteFile(a.statePath(), b, 0o644)
	}
}

// gameChangedNotice returns a message when the game's data file or SoulWorker64.dll is not what the
// updater last saw, or "" when nothing changed, nothing was recorded yet, or the files cannot be read.
func (a *App) gameChangedNotice() string {
	b, err := os.ReadFile(a.statePath())
	if err != nil {
		return ""
	}
	var st gameState
	if json.Unmarshal(b, &st) != nil || st.SourceSHA == "" {
		return ""
	}
	data := a.gameDataFile()
	if !exists(data) {
		return ""
	}
	cur, err := hashFile(data)
	if err != nil {
		return ""
	}
	dll, _ := hashFile(filepath.Join(a.cfg.SWPath, "SoulWorker64.dll"))
	dataSame := cur == st.SourceSHA || cur == st.OutputSHA
	dllSame := dll == "" || st.DllSHA == "" || dll == st.DllSHA
	switch {
	case dataSame && dllSame:
		return ""
	case !dllSame:
		return "The game was updated since your last patch (its files changed), so the translation may be gone.\n" +
			"  Choose 1) Patch now. Parts that are not translated yet stay in the game's own language.\n" +
			"  If the game does not start or shows wrong text, choose 4) Restore a backup."
	default:
		return fmt.Sprintf("%s.v in the game folder is not the file this updater saw last time\n"+
			"  (a game repair or verify, or another tool replaced it). Choose 1) Patch now to apply the translation again.\n"+
			"  If something looks wrong in the game, choose 4) Restore a backup.", a.cfg.DataSource)
	}
}
